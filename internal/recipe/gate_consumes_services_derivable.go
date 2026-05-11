package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Run-40 ENG-5 — `consumesServices` ghost-dependency gate.
//
// `cb.ConsumesServices` is the per-codebase managed-service set the
// codebase-content composer + recipe-context renderer filter by. It
// is populated by `populateConsumesServicesFromYaml` at scaffold
// complete-phase: parse the bare yaml's `run.envVariables` for
// `${<host>_*}` references, intersect with `plan.Services`.
//
// Run-39 surfaced a stale-cache failure mode: `worker.ConsumesServices`
// declared `["broker","db"]` after scaffold-time analysis, but the
// feature phase rewrote worker's source to drop the Postgres client.
// The yaml still carried db env vars (themselves the byproduct of
// the scaffold-time decision); `cb.ConsumesServices` was never
// refreshed; plan.json shipped with a ghost dependency the source
// can't reach.
//
// gateConsumesServicesDerivable runs at feature complete-phase and
// at codebase-content complete-phase. It re-parses each codebase's
// on-disk zerops.yaml and flags hosts in `cb.ConsumesServices` that
// the current yaml doesn't reference. The agent receives a refusal
// pointing at the offending host(s) and the recovery path:
//   - drop the cross-service env-var alias from `run.envVariables`
//     (the yaml was wrong), OR
//   - re-introduce the consuming code path (the source was wrong),
//   - then re-run complete-phase so the derivation refreshes.
//
// Diagnosed in plans/run-40-evidence-grounded-plan.md §"S1-4" + §"ENG-5".

// gateConsumesServicesDerivable runs the ghost-dependency check
// across every codebase that has a populated ConsumesServices slice.
// Codebases with nil ConsumesServices (not yet analyzed) are skipped
// — the populate path will run at scaffold close and refresh them.
//
// Returns a Blocking violation per offending host per codebase so the
// agent sees every ghost at once rather than chasing one at a time.
func gateConsumesServicesDerivable(ctx GateContext) []Violation {
	if ctx.Plan == nil {
		return nil
	}
	var out []Violation
	for _, cb := range ctx.Plan.Codebases {
		if cb.ConsumesServices == nil {
			continue
		}
		if cb.SourceRoot == "" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(cb.SourceRoot, "zerops.yaml"))
		if err != nil {
			// Yaml not on disk: skip silently. The populate path
			// already documents this as a no-op (sim/back-compat
			// shape); the gate inherits the same conservative
			// behavior so it doesn't fail on environments where the
			// yaml is staged elsewhere.
			continue
		}
		derived := parseConsumedServicesFromYaml(string(body), ctx.Plan)
		derivedSet := make(map[string]struct{}, len(derived))
		for _, h := range derived {
			derivedSet[h] = struct{}{}
		}
		var ghosts []string
		for _, declared := range cb.ConsumesServices {
			if _, ok := derivedSet[declared]; !ok {
				ghosts = append(ghosts, declared)
			}
		}
		if len(ghosts) == 0 {
			continue
		}
		sort.Strings(ghosts)
		for _, ghost := range ghosts {
			out = append(out, Violation{
				Code:     "consumes-services-ghost-dependency",
				Path:     fmt.Sprintf("codebase/%s/consumesServices", cb.Hostname),
				Severity: SeverityBlocking,
				Message: fmt.Sprintf(
					"codebase %q declares dependency on managed service %q in plan.ConsumesServices, but the on-disk %s/zerops.yaml does not reference `${%s_*}` in run.envVariables. The declared dependency is a ghost — re-rendered briefs would tell the porter to wire a service the code can't reach. Recovery: either restore the consuming env-var alias in `run.envVariables` (if the code path was supposed to keep the dependency) or drop the stale dependency by re-running scaffold/feature complete-phase after the yaml change so the derivation refreshes.",
					cb.Hostname, ghost, cb.SourceRoot, ghost),
			})
		}
	}
	return out
}
