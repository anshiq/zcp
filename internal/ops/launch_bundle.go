package ops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zeropsio/zcp/internal/schema"
	"github.com/zeropsio/zcp/internal/topology"
)

// importModeHA is the platform scaling-mode value for HA managed services.
// Used in launch-bundle composition where we promote managed deps from
// NON_HA (export default) to HA for production.
const importModeHA = "HA"

// runtimeProductionMinContainers is the default minimum container count
// for runtime services in production — provides HA-via-replication for
// stateless apps. Caller may override via LaunchBundleInputs.MinContainers.
const runtimeProductionMinContainers = 2

// runtimeProductionCPUMode is the default CPU mode for production runtime
// services. DEDICATED removes the noisy-neighbor variance of SHARED at
// the cost of higher per-container price.
const runtimeProductionCPUMode = "DEDICATED"

// LaunchBundle is the output of BuildLaunchBundle. Same general shape as
// ExportBundle but specialized for the launch-production flow: prod-tier
// transforms applied to managed services + runtime, source snapshot
// hashes recorded for immutability guard, no Variant field (launch is
// always single-runtime production from one source half).
//
// SourceSnapshot is the immutability guard substrate: BuildLaunchBundle
// records a deterministic digest of source state at compose-time; the
// workflow handler re-computes before mutation and rejects on drift.
type LaunchBundle struct {
	// ImportYAML is the rendered zerops-project-import.yaml body to send
	// to PostClientProjectImport.
	ImportYAML string
	// TargetProjectName is the destination project name (echoed from inputs).
	TargetProjectName string
	// SourceProjectID identifies the dev/stage source. Recorded for
	// audit + immutability guard.
	SourceProjectID string
	// SourceSnapshot is the per-bundle integrity record. Workflow handler
	// re-computes before mutation and refuses to publish on drift.
	SourceSnapshot SourceSnapshot
	// Classifications echoes the per-env bucket map for the audit log.
	Classifications map[string]topology.SecretClassification
	// Warnings — non-fatal hints carried forward to the agent UI.
	Warnings []string
	// Errors — blocking schema-validation failures. When non-empty, bundle
	// MUST NOT publish.
	Errors []schema.ValidationError
}

// SourceSnapshot is a deterministic digest of source state at the moment
// BuildLaunchBundle composed the bundle. Used by the workflow handler's
// source-immutability guard: re-compute these hashes before invoking
// ProjectAdminClient.CreateAndImportProject; if any field changed,
// publish is rejected with a `source-drift` blocker.
type SourceSnapshot struct {
	// GitCommitSHA is the source repo's HEAD commit when the bundle was composed.
	GitCommitSHA string
	// ZeropsYAMLSHA256 is sha256 of the source zerops.yaml body (including
	// the appended setup: prod block as committed).
	ZeropsYAMLSHA256 string
	// ProjectEnvsDigest is sha256 over sorted (key=value) lines of the
	// source project envs. Captures the env shape that classification ran
	// against.
	ProjectEnvsDigest string
	// ServiceListDigest is sha256 over sorted "hostname:type" lines of
	// the source service list. Captures the topology shape.
	ServiceListDigest string
}

// LaunchBundleInputs feeds BuildLaunchBundle. Strict superset of the
// classification inputs from BundleInputs plus prod-specific knobs.
type LaunchBundleInputs struct {
	// SourceProjectID — recorded on the bundle for audit.
	SourceProjectID string
	// TargetProjectName — what the new prod project will be named.
	TargetProjectName string
	// TargetHostname — the runtime hostname in the launch yaml. Inherited
	// from the source's chosen half (dev half for ModeStandard pair, or
	// the single runtime for ModeSimple, etc.).
	TargetHostname string
	// ServiceType — runtime type tag (e.g. "nodejs@22"). Same convention
	// as BundleInputs.
	ServiceType string
	// SetupName — the zerops.yaml setup-block name the runtime resolves
	// at build. Caller is expected to write this block to source repo
	// during the source-control mutation phase (launch-write-prod-setup
	// atom). Default convention: "prod".
	SetupName string
	// RepoURL — buildFromGit URL pointing at the source repo. Same one
	// the dev/stage runtime uses; production builds from the same code,
	// just with the prod setup-block.
	RepoURL string
	// ZeropsYAMLBody — verbatim source zerops.yaml body (after the agent
	// has appended setup: prod and pushed). BuildLaunchBundle hashes it
	// into SourceSnapshot.ZeropsYAMLSHA256 and verifies SetupName exists.
	ZeropsYAMLBody string
	// GitCommitSHA — current HEAD of the source repo. Captured into
	// SourceSnapshot.GitCommitSHA.
	GitCommitSHA string
	// ProjectEnvs — source project-level env snapshot for classification.
	ProjectEnvs []ProjectEnvVar
	// ManagedServices — managed dep entries in source. Bundle promotes
	// each to HA unless its Hostname is in KeepNonHA.
	ManagedServices []ManagedServiceEntry
	// KeepNonHA — opt-out: managed service hostnames the user explicitly
	// wants to stay NON_HA in prod (cost / can't HA / per-service reasons).
	KeepNonHA []string
	// MinContainers — runtime min count. Default runtimeProductionMinContainers (2).
	MinContainers int
	// AdditionalTags — appended to ["env:prod", "source-project:<id>", "managed-by:zcp-launch"].
	AdditionalTags []string
}

// BuildLaunchBundle composes the production import yaml.
//
// Composition steps:
//  1. Verify SetupName exists in ZeropsYAMLBody (same gate as export).
//  2. Classify project envs via composeProjectEnvVariables (REUSED).
//  3. Compose services array:
//     - Runtime entry: mode NON_HA (platform constraint), minContainers
//     (default 2), cpuMode DEDICATED, no enableSubdomainAccess.
//     - Managed entries: mode HA unless in KeepNonHA, priority 10.
//  4. Compose project block: name, mode SERIOUS, tags merged, envVariables.
//  5. Marshal yaml + add preprocessor header (REUSED helper).
//  6. Schema-validate; surface errors on bundle.
//  7. Compute SourceSnapshot hashes (NEW).
func BuildLaunchBundle(
	inputs LaunchBundleInputs,
	classifications map[string]topology.SecretClassification,
) (*LaunchBundle, error) {
	// Validate required inputs
	if inputs.TargetProjectName == "" {
		return nil, fmt.Errorf("launch bundle: TargetProjectName required")
	}
	if inputs.TargetHostname == "" {
		return nil, fmt.Errorf("launch bundle: TargetHostname required")
	}
	if inputs.ServiceType == "" {
		return nil, fmt.Errorf("launch bundle: ServiceType required")
	}
	if inputs.SetupName == "" {
		inputs.SetupName = "prod"
	}
	if inputs.RepoURL == "" {
		return nil, fmt.Errorf("launch bundle: RepoURL required")
	}
	if inputs.ZeropsYAMLBody == "" {
		return nil, fmt.Errorf("launch bundle: ZeropsYAMLBody required")
	}
	if inputs.SourceProjectID == "" {
		return nil, fmt.Errorf("launch bundle: SourceProjectID required (audit + immutability guard)")
	}
	if err := verifyZeropsYAMLSetup(inputs.ZeropsYAMLBody, inputs.SetupName); err != nil {
		return nil, err
	}

	bundle := &LaunchBundle{
		TargetProjectName: inputs.TargetProjectName,
		SourceProjectID:   inputs.SourceProjectID,
		Classifications:   classifications,
	}

	// Step 2 — classify (REUSED helper from export_bundle.go)
	projectEnvs, envWarnings := composeProjectEnvVariables(inputs.ProjectEnvs, classifications)
	bundle.Warnings = append(bundle.Warnings, envWarnings...)

	// Step 2b — detect indirect refs (REUSED)
	zeropsRefs := extractZeropsYAMLRunEnvRefs(inputs.ZeropsYAMLBody)
	bundle.Warnings = append(bundle.Warnings, detectIndirectInfraReferences(inputs.ProjectEnvs, classifications, zeropsRefs)...)

	// Step 3 — compose services array with prod-tier transforms
	minContainers := inputs.MinContainers
	if minContainers <= 0 {
		minContainers = runtimeProductionMinContainers
	}
	runtimeEntry := map[string]any{
		"hostname":      inputs.TargetHostname,
		"type":          inputs.ServiceType,
		"mode":          importModeNonHA, // runtime always NON_HA at platform layer
		"buildFromGit":  inputs.RepoURL,
		"zeropsSetup":   inputs.SetupName,
		"minContainers": minContainers,
		"verticalAutoscaling": map[string]any{
			"cpuMode": runtimeProductionCPUMode,
		},
		// enableSubdomainAccess intentionally absent — prod uses custom domain
	}

	keepNonHASet := make(map[string]bool, len(inputs.KeepNonHA))
	for _, h := range inputs.KeepNonHA {
		keepNonHASet[h] = true
	}

	services := make([]any, 0, 1+len(inputs.ManagedServices))
	services = append(services, runtimeEntry)
	for _, m := range inputs.ManagedServices {
		entry := map[string]any{
			"hostname": m.Hostname,
			"type":     m.Type,
			"priority": 10,
		}
		// HA promotion: every managed service goes HA unless opted out.
		// Note: object-storage and shared-storage have their own mode
		// constraints — caller should put those in KeepNonHA if needed.
		if keepNonHASet[m.Hostname] {
			if m.Mode != "" {
				entry["mode"] = m.Mode
			} else {
				entry["mode"] = importModeNonHA
			}
		} else {
			entry["mode"] = importModeHA
		}
		services = append(services, entry)
	}

	// Step 4 — compose project block
	project := map[string]any{
		"name": inputs.TargetProjectName,
		"tags": composeLaunchTags(inputs.SourceProjectID, inputs.AdditionalTags),
	}
	if len(projectEnvs) > 0 {
		project["envVariables"] = projectEnvs
	}

	doc := map[string]any{
		"project":  project,
		"services": services,
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("launch bundle: marshal yaml: %w", err)
	}
	body := string(out)
	body = addPreprocessorHeader(body, projectEnvs)

	bundle.ImportYAML = body

	// Step 6 — schema-validate import yaml
	if errs := schema.ValidateImportYAML(body); len(errs) > 0 {
		bundle.Errors = errs
	}

	// Step 7 — compute source snapshot hashes
	bundle.SourceSnapshot = computeSourceSnapshot(inputs)

	return bundle, nil
}

// composeLaunchTags returns the canonical tag set for a launch bundle.
// "env:prod" + "source-project:<sourceID>" + "managed-by:zcp-launch" are
// always emitted; AdditionalTags are appended in order (deduplicated).
func composeLaunchTags(sourceProjectID string, additional []string) []string {
	tags := []string{
		"env:prod",
		"source-project:" + sourceProjectID,
		"managed-by:zcp-launch",
	}
	seen := map[string]bool{
		tags[0]: true,
		tags[1]: true,
		tags[2]: true,
	}
	for _, t := range additional {
		if t == "" {
			continue
		}
		if seen[t] {
			continue
		}
		tags = append(tags, t)
		seen[t] = true
	}
	return tags
}

// computeSourceSnapshot produces a deterministic digest of source state
// for the immutability guard.
func computeSourceSnapshot(inputs LaunchBundleInputs) SourceSnapshot {
	return SourceSnapshot{
		GitCommitSHA:      inputs.GitCommitSHA,
		ZeropsYAMLSHA256:  sha256Hex(inputs.ZeropsYAMLBody),
		ProjectEnvsDigest: hashProjectEnvs(inputs.ProjectEnvs),
		ServiceListDigest: hashServiceList(inputs.ManagedServices, inputs.TargetHostname, inputs.ServiceType),
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// hashProjectEnvs returns sha256 over sorted "key=value\n" lines.
// Stable across runs with identical inputs.
func hashProjectEnvs(envs []ProjectEnvVar) string {
	pairs := make([]string, 0, len(envs))
	for _, e := range envs {
		pairs = append(pairs, e.Key+"="+e.Value)
	}
	sort.Strings(pairs)
	return sha256Hex(strings.Join(pairs, "\n"))
}

// hashServiceList returns sha256 over sorted "hostname:type\n" lines
// including the runtime + all managed deps.
func hashServiceList(managed []ManagedServiceEntry, runtimeHost, runtimeType string) string {
	lines := make([]string, 0, 1+len(managed))
	lines = append(lines, runtimeHost+":"+runtimeType)
	for _, m := range managed {
		lines = append(lines, m.Hostname+":"+m.Type)
	}
	sort.Strings(lines)
	return sha256Hex(strings.Join(lines, "\n"))
}
