package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/zeropsio/zcp/internal/ops"
	"github.com/zeropsio/zcp/internal/platform"
	"github.com/zeropsio/zcp/internal/runtime"
	"github.com/zeropsio/zcp/internal/topology"
	"github.com/zeropsio/zcp/internal/workflow"
)

// handleLaunchProduction orchestrates the launch-production workflow per
// plans/production-lifecycle-2026-05-11.md §8.1. Stateless multi-call
// narrowing via per-request WorkflowInput fields:
//   - ProductionProjectName / Region / CustomDomain / KeepNonHA — scope
//   - EnvClassifications — classify-prompt outputs
//   - LaunchKey — one-shot account-wide token (mutation pipeline, Phase D.2)
//
// Six top-level statuses:
//
//	scope-prompt    → ProductionProjectName empty
//	classify-prompt → source envs present, classifications incomplete
//	ready-to-launch → scope + classifications complete, awaiting LaunchKey
//	launching       → LaunchKey supplied; mutation pipeline in flight (D.2)
//	failed          → mutation step failed (D.2)
//	launched        → terminal success (D.2)
//
// P-LP-1: input.LaunchKey is json:"-" so it never appears in any
// JSON-serialized response. Handler must not log it, write it to state,
// or include it in error messages.
//
// P-LP-2: this is the ONLY file in internal/tools/ that may construct
// platform.ProjectAdminClient. Pinned by
// internal/platform/project_admin_imports_test.go.
//
// projectAdminClientFactory constructs a ProjectAdminClient from a
// launch-window key. Indirected so unit tests can inject a mock without
// hitting the real Zerops API. Package-level var so the launch handler
// can call it; tests override via setProjectAdminClientFactory.
//
// Production default: platform.NewProjectAdminClient.
//
//nolint:gochecknoglobals // test-injection point for the cross-project surface
var projectAdminClientFactory = platform.NewProjectAdminClient

// setProjectAdminClientFactory swaps the factory for tests. Restore with
// the returned cleanup func via defer.
func setProjectAdminClientFactory(f func(launchKey, apiHost string) (platform.ProjectAdminClient, error)) func() {
	prev := projectAdminClientFactory
	projectAdminClientFactory = f
	return func() { projectAdminClientFactory = prev }
}

func handleLaunchProduction(
	ctx context.Context,
	projectID string,
	client platform.Client,
	input WorkflowInput,
	stateDir string,
	rt runtime.Info,
) (*mcp.CallToolResult, any, error) {
	_ = rt // reserved for environment-aware (container vs local) behavior in source-control verification

	if client == nil {
		return convertError(platform.NewPlatformError(
			platform.ErrInvalidParameter,
			"Platform client unavailable — launch-production requires API access for source-project discovery",
			"Ensure ZCP is configured with a Zerops API key (ZCP_API_KEY) before invoking launch-production.",
		), WithRecoveryStatus()), nil, nil
	}
	if projectID == "" {
		return convertError(platform.NewPlatformError(
			platform.ErrInvalidParameter,
			"Project ID unavailable — launch-production requires a configured source-project context",
			"Ensure ZCP is bound to a Zerops project (ZCP_PROJECT_ID or zcp config).",
		), WithRecoveryStatus()), nil, nil
	}

	corpus, err := workflow.LoadAtomCorpus()
	if err != nil {
		return convertError(err, WithRecoveryStatus()), nil, nil
	}

	// Status 1 — scope-prompt: required scope fields incomplete.
	if missing := missingScopeFields(input); len(missing) > 0 {
		return launchScopePromptResponse(corpus, input, missing), nil, nil
	}

	// Read source project envs (needed for both classify-prompt and
	// publish-time bundle composition).
	sourceEnvs, err := readProjectEnvs(ctx, client, projectID)
	if err != nil {
		return convertError(err, WithRecoveryStatus()), nil, nil
	}

	// Status 2 — classify-prompt: source envs present, not all bucketed.
	if needsClassifyPrompt(input.EnvClassifications, sourceEnvs) {
		classifications := convertClassificationsInput(input.EnvClassifications)
		return launchClassifyPromptResponse(corpus, sourceEnvs, classifications), nil, nil
	}

	classifications := convertClassificationsInput(input.EnvClassifications)

	// Status 3+ — ready-to-launch / mutation pipeline.
	// Check for existing launch state — if a prior publish already created
	// the target project, idempotent resume returns the current state
	// instead of re-importing.
	launchID := generateLaunchID(projectID, input.ProductionProjectName)
	existing, err := readLaunchState(stateDir, launchID)
	if err != nil && !errors.Is(err, ErrLaunchStateMissing) {
		return convertError(platform.NewPlatformError(
			platform.ErrAPIError,
			fmt.Sprintf("read launch state: %v", err),
			"Inspect .zcp/state/launch-production/ and clean up corrupted state files if needed.",
		), WithRecoveryStatus()), nil, nil
	}

	// If we already created the target project on a prior call, return
	// the current state regardless of launchKey. This is the recovery
	// primitive (action="status" semantics).
	if existing != nil && existing.TargetProjectID != "" {
		return launchResumeResponse(corpus, existing), nil, nil
	}

	if input.LaunchKey == "" {
		return launchReadyToLaunchResponse(corpus, input, sourceEnvs), nil, nil
	}

	// Mutation pipeline — LaunchKey supplied, no existing target.
	return executeLaunchMutation(ctx, projectID, input, sourceEnvs, classifications, corpus, stateDir, launchID)
}

// executeLaunchMutation runs the read-modify-write mutation pipeline:
//  1. Construct ProjectAdminClient from launchKey (validates key).
//  2. Build LaunchBundle from source state.
//  3. Call CreateAndImportProject.
//  4. Write state file with results.
//  5. Append audit log entry.
//  6. Return launching/launched/failed status.
//
// The launchKey lives inside admin's SDK handler — never copied to local
// vars. defer admin.Close() zeros it before return.
//
// P-LP-1: no field on the response or state file carries the key.
func executeLaunchMutation(
	ctx context.Context,
	sourceProjectID string,
	input WorkflowInput,
	sourceEnvs []ops.ProjectEnvVar,
	classifications map[string]topology.SecretClassification,
	corpus []workflow.KnowledgeAtom,
	stateDir string,
	launchID string,
) (*mcp.CallToolResult, any, error) {
	admin, err := projectAdminClientFactory(input.LaunchKey, "")
	if err != nil {
		// Don't leak the key value in the error — wrap via the typed error.
		return launchFailedAuthResponse(corpus, err), nil, nil
	}
	defer admin.Close()

	// Compose bundle. For D.2 MVP we synthesize a minimal bundle from
	// the source-project's envs + a fabricated zerops.yaml body that
	// carries the expected setup: prod marker. Real source-yaml read
	// via SSH lives in the next iteration; the bundle composition path
	// is structurally correct and exercised by tests with mock inputs.
	bundleInputs := ops.LaunchBundleInputs{
		SourceProjectID:   sourceProjectID,
		TargetProjectName: input.ProductionProjectName,
		TargetHostname:    input.TargetService,
		// ServiceType / RepoURL / ZeropsYAMLBody / GitCommitSHA / ManagedServices
		// are sourced by full integration in a follow-up — D.2 MVP returns a
		// structured "source-control-required" blocker so the agent knows
		// to provide them via prior phase outputs.
		ProjectEnvs: sourceEnvs,
		KeepNonHA:   input.KeepNonHA,
	}

	// Structural validation: TargetHostname required for bundle.
	if bundleInputs.TargetHostname == "" {
		_ = appendAuditLog(stateDir, launchAuditEntry{
			LaunchID:          launchID,
			Action:            "publish-rejected",
			SourceProjectID:   sourceProjectID,
			TargetProjectName: input.ProductionProjectName,
			Result:            "failure",
			ErrorMessage:      "TargetService (runtime hostname) required for launch publish",
		})
		return launchSourceControlBlockerResponse(corpus,
			"TargetService input required — launch needs the source-runtime hostname to compose the bundle. Pass targetService=<hostname> from the source project.",
		), nil, nil
	}

	// MVP gate: source-control fields not supplied yet. Surface a
	// source-control blocker that names the missing artifacts so the
	// agent can populate them via the read-side narrowing path. Future
	// iteration will read these via SSH in the same handler call.
	if missingSrc := missingSourceControlInputs(input, bundleInputs); len(missingSrc) > 0 {
		_ = appendAuditLog(stateDir, launchAuditEntry{
			LaunchID:          launchID,
			Action:            "publish-rejected",
			SourceProjectID:   sourceProjectID,
			TargetProjectName: input.ProductionProjectName,
			Result:            "failure",
			ErrorMessage:      "source-control inputs missing: " + fmt.Sprintf("%v", missingSrc),
		})
		return launchSourceControlBlockerResponse(corpus,
			fmt.Sprintf("Source-control inputs missing: %v. Provide setupName + serviceType + repoURL + zeropsYAMLBody via the launch-production scope path before publish.", missingSrc),
		), nil, nil
	}

	// Bundle composition — uses ops.BuildLaunchBundle (Phase C).
	bundle, err := ops.BuildLaunchBundle(bundleInputs, classifications)
	if err != nil {
		_ = appendAuditLog(stateDir, launchAuditEntry{
			LaunchID:          launchID,
			Action:            "publish-rejected",
			SourceProjectID:   sourceProjectID,
			TargetProjectName: input.ProductionProjectName,
			Result:            "failure",
			ErrorMessage:      "bundle compose: " + err.Error(),
		})
		return launchFailedResponse(corpus, topology.BlockerCategoryOther,
			"bundle-compose-failed",
			"Launch bundle composition failed: "+err.Error()), nil, nil
	}
	if len(bundle.Errors) > 0 {
		_ = appendAuditLog(stateDir, launchAuditEntry{
			LaunchID:          launchID,
			Action:            "publish-rejected",
			SourceProjectID:   sourceProjectID,
			TargetProjectName: input.ProductionProjectName,
			Result:            "failure",
			ErrorMessage:      "schema validation failed",
		})
		return launchFailedResponse(corpus, topology.BlockerCategorySchema,
			"schema-validation-failed",
			fmt.Sprintf("Import yaml schema validation failed: %v", bundle.Errors)), nil, nil
	}

	// Persist initial state pre-mutation — if CreateAndImport panics or
	// the process dies before completion, the state file shows the
	// attempt and the source-snapshot for forensics.
	state := &launchState{
		LaunchID:          launchID,
		SourceProjectID:   sourceProjectID,
		TargetProjectName: input.ProductionProjectName,
		SourceSnapshot:    bundle.SourceSnapshot,
		Classifications:   classifications,
		Status:            topology.LaunchStatusLaunching,
	}
	if err := writeLaunchState(stateDir, state); err != nil {
		// Non-fatal — proceed with the mutation, but warn.
		bundle.Warnings = append(bundle.Warnings,
			fmt.Sprintf("write launch state: %v (proceeding; resume after restart may not work)", err))
	}

	// Mutation: CreateAndImportProject. This is the irreversible step.
	result, err := admin.CreateAndImportProject(ctx, bundle.ImportYAML, platform.CreateOpts{
		Location: input.Region,
		Tags:     []string{"env:prod", "managed-by:zcp-launch"},
	})
	if err != nil {
		state.Status = topology.LaunchStatusFailed
		state.LastError = err.Error()
		_ = writeLaunchState(stateDir, state)
		_ = appendAuditLog(stateDir, launchAuditEntry{
			LaunchID:          launchID,
			Action:            "create-and-import",
			SourceProjectID:   sourceProjectID,
			TargetProjectName: input.ProductionProjectName,
			SourceCommitSHA:   bundle.SourceSnapshot.GitCommitSHA,
			SourceYAMLSHA256:  bundle.SourceSnapshot.ZeropsYAMLSHA256,
			Classifications:   classifications,
			HAOptOut:          input.KeepNonHA,
			Result:            "failure",
			ErrorMessage:      err.Error(),
		})
		return launchFailedResponse(corpus, topology.BlockerCategoryAuth,
			"create-import-failed",
			"CreateAndImportProject failed: "+err.Error()), nil, nil
	}

	// Success — record imported services in state.
	state.TargetProjectID = result.ProjectID
	state.ImportedServices = make([]importedServiceEntry, 0, len(result.ServiceStacks))
	hasPerServiceError := false
	for _, s := range result.ServiceStacks {
		entry := importedServiceEntry{
			ID:   s.ID,
			Name: s.Name,
		}
		for _, p := range s.Processes {
			entry.ProcessIDs = append(entry.ProcessIDs, p.ID)
		}
		if s.Error != nil {
			entry.ImportError = s.Error.Code + ": " + s.Error.Message
			hasPerServiceError = true
		}
		state.ImportedServices = append(state.ImportedServices, entry)
	}
	if hasPerServiceError {
		state.Status = topology.LaunchStatusFailed
		state.LastError = "one or more service stacks reported import errors"
	} else {
		state.Status = topology.LaunchStatusLaunched
	}
	_ = writeLaunchState(stateDir, state)

	_ = appendAuditLog(stateDir, launchAuditEntry{
		LaunchID:          launchID,
		Action:            "create-and-import",
		SourceProjectID:   sourceProjectID,
		TargetProjectID:   result.ProjectID,
		TargetProjectName: result.ProjectName,
		SourceCommitSHA:   bundle.SourceSnapshot.GitCommitSHA,
		SourceYAMLSHA256:  bundle.SourceSnapshot.ZeropsYAMLSHA256,
		Classifications:   classifications,
		HAOptOut:          input.KeepNonHA,
		Result:            boolStr(!hasPerServiceError, "success", "failure"),
		ErrorMessage:      stringIf(hasPerServiceError, "one or more service stacks reported import errors"),
	})

	if hasPerServiceError {
		return launchFailedResponse(corpus, topology.BlockerCategoryOrphan,
			"orphan-project",
			fmt.Sprintf("Target project %s created but one or more services had import errors. Inspect imported-services in state file; delete via Zerops dashboard or retry with corrected inputs.", result.ProjectID),
		), nil, nil
	}

	return launchLaunchedResponse(corpus, state), nil, nil
}

// boolStr returns t when cond, f otherwise.
func boolStr(cond bool, t, f string) string {
	if cond {
		return t
	}
	return f
}

// stringIf returns s when cond, "" otherwise.
func stringIf(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

// missingSourceControlInputs reports source-control fields that the
// agent must populate before the bundle compose can produce a valid
// import yaml. Caller responds with a source-control blocker pointing
// at the launch-write-prod-setup atom.
func missingSourceControlInputs(input WorkflowInput, b ops.LaunchBundleInputs) []string {
	var missing []string
	if input.TargetService == "" {
		missing = append(missing, "targetService")
	}
	if b.ServiceType == "" {
		missing = append(missing, "serviceType")
	}
	if b.RepoURL == "" {
		missing = append(missing, "repoURL")
	}
	if b.ZeropsYAMLBody == "" {
		missing = append(missing, "zeropsYAMLBody (write setup: prod block and commit)")
	}
	return missing
}

// launchProductionResponse is the wire shape returned by every status of
// the launch-production workflow.
//
// Top-level status drives agent dispatch; blockers[] + checks[] carry
// structured per-status detail per plan v2 §8.1 design.
//
// LaunchKey intentionally absent — P-LP-1 invariant. No field on this
// struct or its members carries the key.
type launchProductionResponse struct {
	Workflow string                          `json:"workflow"`
	Status   topology.LaunchProductionStatus `json:"status"`
	Phase    workflow.Phase                  `json:"phase"`
	Guidance string                          `json:"guidance"`
	Blockers []topology.Blocker              `json:"blockers,omitempty"`
	Inputs   *launchInputsEcho               `json:"inputs,omitempty"`
	// Classifications is the classify-prompt review table — emitted when
	// status is classify-prompt. Per-env rows omit values per P-LP-5.
	Classifications []launchClassifyRow `json:"classifications,omitempty"`
}

// launchInputsEcho echoes the scope inputs the workflow saw on the call,
// for agent forensics. Excludes LaunchKey unconditionally.
type launchInputsEcho struct {
	ProductionProjectName string   `json:"productionProjectName,omitempty"`
	Region                string   `json:"region,omitempty"`
	CustomDomain          string   `json:"customDomain,omitempty"`
	KeepNonHA             []string `json:"keepNonHa,omitempty"`
}

// launchClassifyRow is one row of the classify-prompt review table.
// Mirrors export's classify-prompt row shape but explicitly omits raw
// values — agent fetches them separately via zerops_discover and
// re-calls with the populated EnvClassifications.
type launchClassifyRow struct {
	Key           string                        `json:"key"`
	CurrentBucket topology.SecretClassification `json:"currentBucket"`
}

// missingScopeFields returns the names of scope fields that are still
// missing. Empty result = scope complete enough to advance.
func missingScopeFields(input WorkflowInput) []string {
	var missing []string
	if input.ProductionProjectName == "" {
		missing = append(missing, "productionProjectName")
	}
	// Region defaults to eu-central at compose-time if empty (per spec-
	// launch-production-platform-spike A.4) — don't require it from
	// the agent.
	return missing
}

// launchScopePromptResponse builds the scope-prompt response.
func launchScopePromptResponse(corpus []workflow.KnowledgeAtom, input WorkflowInput, missing []string) *mcp.CallToolResult {
	guidance := atomBody(corpus, "launch-scope-prompt")
	if guidance == "" {
		// Fallback when corpus load left the atom out — shouldn't happen
		// in practice, but better than a silent empty response.
		guidance = "Provide productionProjectName, region (optional, defaults to eu-central), and custom-domain options."
	}

	blockers := make([]topology.Blocker, 0, len(missing))
	for _, name := range missing {
		blockers = append(blockers, topology.Blocker{
			ID:       "scope-missing-" + name,
			Severity: topology.BlockerSeverityBlock,
			Category: topology.BlockerCategoryScope,
			Message:  fmt.Sprintf("workflow input %q required to advance to classify-prompt", name),
		})
	}

	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusScopePrompt,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: guidance,
		Blockers: blockers,
		Inputs:   echoInputs(input),
	})
}

// launchClassifyPromptResponse builds the classify-prompt response.
func launchClassifyPromptResponse(
	corpus []workflow.KnowledgeAtom,
	sourceEnvs []ops.ProjectEnvVar,
	classifications map[string]topology.SecretClassification,
) *mcp.CallToolResult {
	guidance := atomBody(corpus, "launch-classify-prompt")
	if guidance == "" {
		// Reuse export's classify-prompt guidance shape — same protocol,
		// same buckets. Atom alias planned for Phase E.
		guidance = atomBody(corpus, "export-classify-envs")
	}
	if guidance == "" {
		guidance = "Classify each source env into infrastructure / auto-secret / external-secret / plain-config buckets."
	}

	rows := make([]launchClassifyRow, 0, len(sourceEnvs))
	for _, env := range sourceEnvs {
		rows = append(rows, launchClassifyRow{
			Key:           env.Key,
			CurrentBucket: classifications[env.Key],
		})
	}

	return jsonResult(launchProductionResponse{
		Workflow:        workflowLaunchProduction,
		Status:          topology.LaunchStatusClassifyPrompt,
		Phase:           workflow.PhaseLaunchProductionActive,
		Guidance:        guidance,
		Classifications: rows,
	})
}

// launchReadyToLaunchResponse builds the ready-to-launch preview. Phase D.1
// emits a minimal preview that echoes inputs + classified-env summary +
// directs the agent to obtain the one-shot launch key. Phase D.2 will
// extend this with the LaunchBundle preview, source-snapshot hashes, and
// cost estimate.
func launchReadyToLaunchResponse(
	corpus []workflow.KnowledgeAtom,
	input WorkflowInput,
	sourceEnvs []ops.ProjectEnvVar,
) *mcp.CallToolResult {
	guidance := atomBody(corpus, "launch-mutation-key-required")
	if guidance == "" {
		guidance = "Scope and classifications complete. Generate a one-shot Zerops API key (account-wide) and re-call with launchKey set to advance to publish."
	}
	_ = sourceEnvs // Phase D.2 will surface classified-env summary

	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusReadyToLaunch,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: guidance,
		Inputs:   echoInputs(input),
	})
}

// launchResumeResponse returns the current state of a launch that has
// already created the target project (idempotent resume).
func launchResumeResponse(corpus []workflow.KnowledgeAtom, state *launchState) *mcp.CallToolResult {
	resp := launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   state.Status,
		Phase:    workflow.PhaseLaunchProductionActive,
	}
	switch state.Status {
	case topology.LaunchStatusLaunched:
		resp.Guidance = atomBody(corpus, "launch-post-checklist")
		if resp.Guidance == "" {
			resp.Guidance = fmt.Sprintf("Production project %s launched. Delete the launch-window key + set external secrets in Zerops UI.", state.TargetProjectID)
		}
	case topology.LaunchStatusFailed:
		resp.Guidance = "Prior launch reached failed status. Inspect lastError in state file; retry by clearing the state file and re-calling publish."
		resp.Blockers = []topology.Blocker{{
			ID:       "prior-launch-failed",
			Severity: topology.BlockerSeverityBlock,
			Category: topology.BlockerCategoryOther,
			Message:  "previous launch failed: " + state.LastError,
		}}
	case topology.LaunchStatusUnset,
		topology.LaunchStatusScopePrompt,
		topology.LaunchStatusClassifyPrompt,
		topology.LaunchStatusReadyToLaunch,
		topology.LaunchStatusLaunching:
		// Resume found state with a pre-terminal status — surface
		// in-progress guidance; agent re-polls via action="status".
		resp.Guidance = "Launch in progress. State file shows targetProjectID " + state.TargetProjectID + "."
	}
	return jsonResult(resp)
}

// launchFailedAuthResponse handles the case where NewProjectAdminClient
// fails to authenticate. The error from the constructor never contains
// the key value (per P-LP-1) — we just wrap its message.
func launchFailedAuthResponse(corpus []workflow.KnowledgeAtom, err error) *mcp.CallToolResult {
	_ = corpus
	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusFailed,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: "Launch-window key validation failed. Generate a new account-wide token in Zerops dashboard and retry.",
		Blockers: []topology.Blocker{{
			ID:       "launch-key-invalid",
			Severity: topology.BlockerSeverityBlock,
			Category: topology.BlockerCategoryAuth,
			Message:  "ProjectAdminClient construction failed: " + err.Error(),
		}},
	})
}

// launchSourceControlBlockerResponse fires when source-control fields
// (zerops.yaml body, setup name, target hostname) are not supplied to
// the handler.
func launchSourceControlBlockerResponse(corpus []workflow.KnowledgeAtom, msg string) *mcp.CallToolResult {
	guidance := atomBody(corpus, "launch-write-prod-setup")
	if guidance == "" {
		guidance = "Append setup: prod block to source zerops.yaml, commit, and push before publish."
	}
	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusFailed,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: guidance,
		Blockers: []topology.Blocker{{
			ID:       "source-control-required",
			Severity: topology.BlockerSeverityBlock,
			Category: topology.BlockerCategorySourceControl,
			Message:  msg,
		}},
	})
}

// launchFailedResponse builds a generic failed-status response with a
// single blocker.
func launchFailedResponse(corpus []workflow.KnowledgeAtom, category topology.BlockerCategory, id, msg string) *mcp.CallToolResult {
	_ = corpus
	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusFailed,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: msg,
		Blockers: []topology.Blocker{{
			ID:       id,
			Severity: topology.BlockerSeverityBlock,
			Category: category,
			Message:  msg,
		}},
	})
}

// launchLaunchedResponse builds the terminal-success response with the
// mandatory delete-key atom (P-LP-4 invariant) + post-launch checklist.
func launchLaunchedResponse(corpus []workflow.KnowledgeAtom, state *launchState) *mcp.CallToolResult {
	// Concatenate the mandatory delete-key atom + post-checklist for the
	// composite "what you do next" surface.
	deleteAtom := atomBody(corpus, "launch-delete-key")
	checklistAtom := atomBody(corpus, "launch-post-checklist")
	guidance := deleteAtom
	if checklistAtom != "" {
		if guidance != "" {
			guidance += "\n\n"
		}
		guidance += checklistAtom
	}
	if guidance == "" {
		// Fallback so the mandatory step is never silently dropped.
		guidance = "Production project " + state.TargetProjectID + " launched. DELETE THE LAUNCH-WINDOW KEY NOW in Zerops dashboard → Access Tokens Management."
	}
	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusLaunched,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: guidance,
		Inputs: &launchInputsEcho{
			ProductionProjectName: state.TargetProjectName,
		},
	})
}

// echoInputs returns a sanitized snapshot of scope inputs — never
// includes LaunchKey.
func echoInputs(input WorkflowInput) *launchInputsEcho {
	echo := &launchInputsEcho{
		ProductionProjectName: input.ProductionProjectName,
		Region:                input.Region,
		CustomDomain:          input.CustomDomain,
	}
	if len(input.KeepNonHA) > 0 {
		echo.KeepNonHA = append(echo.KeepNonHA, input.KeepNonHA...)
	}
	return echo
}

// atomBody returns the body string for atom with the given ID. Empty
// when not found (caller falls back to inline guidance). Thin wrapper
// over workflow.LookupAtomBody so the discipline test
// (TestNoProductionAtomBodyReads) keeps the direct Body access at the
// parser boundary.
func atomBody(corpus []workflow.KnowledgeAtom, id string) string {
	return workflow.LookupAtomBody(corpus, id)
}
