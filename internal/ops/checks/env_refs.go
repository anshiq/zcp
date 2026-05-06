package checks

import (
	"context"
	"fmt"
	"strings"

	"github.com/zeropsio/zcp/internal/ops"
	"github.com/zeropsio/zcp/internal/workflow"
)

// CheckEnvRefs validates cross-service env-variable references in a single
// zerops.yaml entry. Resolves each `${...}` reference against the map of
// platform-discovered env vars per hostname and the live-hostname list.
// A reference whose hostname prefix matches a live service but names an
// unknown var on that service fails — that's a typo or schema drift the
// agent can fix.
//
// Inputs:
//   - hostname: the codebase/runtime hostname the entry belongs to. Used
//     as the StepCheck.Name prefix so accumulators across targets can
//     disambiguate.
//   - entry: the parsed setup entry containing `envVariables`. Caller is
//     responsible for parsing zerops.yaml; the predicate only consumes
//     the already-parsed shape.
//   - discoveredEnvVars: map of hostname -> list of env var keys the
//     platform has provisioned. Usually built from BootstrapState; in
//     shim mode the CLI fills it from a locally-cached snapshot or
//     passes an empty map (graceful skip).
//   - liveHostnames: every hostname visible to the recipe (runtime
//     dev/stage targets + managed-service hostnames). A `${...}` body
//     whose prefix doesn't match any live hostname is treated as a lone
//     ref (project-level var / runtime placeholder the platform resolves
//     at deploy time) and skipped — empty liveHostnames therefore
//     gracefully skips all refs in shim mode.
//
// Returns nil when the entry declares no env vars — no surface to check.
// Returns one StepCheck per call (pass or fail). The server-side
// tool-layer caller may decorate with ReadSurface / CoupledWith; the
// predicate itself sticks to Name / Status / Detail so the same shape
// works from the CLI shim.
func CheckEnvRefs(_ context.Context, hostname string, entry *ops.ZeropsYmlEntry, discoveredEnvVars map[string][]string, liveHostnames []string) []workflow.StepCheck {
	if entry == nil || len(entry.Run.EnvVariables) == 0 {
		return nil
	}
	envErrs := ops.ValidateEnvReferences(entry.Run.EnvVariables, discoveredEnvVars, liveHostnames)
	if len(envErrs) == 0 {
		return []workflow.StepCheck{{
			Name:   hostname + "_env_refs",
			Status: StatusPass,
		}}
	}
	details := make([]string, len(envErrs))
	for i, e := range envErrs {
		details[i] = fmt.Sprintf("%s: %s", e.Reference, e.Reason)
	}
	return []workflow.StepCheck{{
		Name:   hostname + "_env_refs",
		Status: StatusFail,
		Detail: strings.Join(details, "; "),
	}}
}
