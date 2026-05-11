package tools

import (
	"context"
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
// rt + stateDir parameters reserved for Phase D.2's idempotent resume
// + audit log writer; D.1 does not yet use them.
func handleLaunchProduction(
	ctx context.Context,
	projectID string,
	client platform.Client,
	input WorkflowInput,
	stateDir string,
	rt runtime.Info,
) (*mcp.CallToolResult, any, error) {
	_ = stateDir // reserved for Phase D.2 state persistence
	_ = rt       // reserved for Phase D.2 environment-aware behavior

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
	// the ready-to-launch bundle composition once Phase D.2 lands).
	sourceEnvs, err := readProjectEnvs(ctx, client, projectID)
	if err != nil {
		return convertError(err, WithRecoveryStatus()), nil, nil
	}

	// Status 2 — classify-prompt: source envs present, not all bucketed.
	if needsClassifyPrompt(input.EnvClassifications, sourceEnvs) {
		classifications := convertClassificationsInput(input.EnvClassifications)
		return launchClassifyPromptResponse(corpus, sourceEnvs, classifications), nil, nil
	}

	// Status 3 — ready-to-launch (Phase D.1 stops here).
	// Phase D.2 will:
	//   - Read source zerops.yaml via SSH + verify setup: prod block exists
	//     (chain to launch-write-prod-setup atom otherwise).
	//   - Read git remote + git SHA.
	//   - Build the LaunchBundle via ops.BuildLaunchBundle.
	//   - Run source-immutability hashing and surface bundle preview.
	//   - On LaunchKey supplied → mutation pipeline.
	//
	// D.1 returns a ready-to-launch envelope with the inputs echoed +
	// the awaiting-key atom so the agent knows the next step.
	if input.LaunchKey == "" {
		return launchReadyToLaunchResponse(corpus, input, sourceEnvs), nil, nil
	}

	// LaunchKey present but Phase D.2 mutation pipeline isn't here yet.
	// Return a structured "not implemented" response so callers don't
	// accidentally proceed assuming the mutation completed.
	return launchMutationDeferredResponse(corpus), nil, nil
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
	KeepNonHA             []string `json:"keepNonHA,omitempty"`
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

// launchMutationDeferredResponse is the Phase D.1 placeholder for when
// LaunchKey is supplied but the D.2 mutation pipeline hasn't shipped yet.
// Returns a structured failed-status response that explicitly names the
// missing implementation — better than silently dropping the key.
func launchMutationDeferredResponse(corpus []workflow.KnowledgeAtom) *mcp.CallToolResult {
	guidance := "launch-production mutation pipeline ships in Phase D.2 — see plans/production-lifecycle-2026-05-11.md §11. Re-call without launchKey to inspect the ready-to-launch preview."
	_ = corpus

	return jsonResult(launchProductionResponse{
		Workflow: workflowLaunchProduction,
		Status:   topology.LaunchStatusFailed,
		Phase:    workflow.PhaseLaunchProductionActive,
		Guidance: guidance,
		Blockers: []topology.Blocker{{
			ID:       "phase-d2-pending",
			Severity: topology.BlockerSeverityBlock,
			Category: topology.BlockerCategoryOther,
			Message:  "Mutation pipeline (Phase D.2) not yet implemented; re-call without launchKey for ready-to-launch preview",
		}},
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
