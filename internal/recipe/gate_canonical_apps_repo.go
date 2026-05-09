package recipe

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Run-34 Fix 2 — engine validator at codebase-content `complete-phase`
// for the canonical apps-repo. Refuses the un-scoped close when the
// canonical apps-repo's assembled README is missing
// "## Recipe features" (RF1) or "## Production vs. Development" (PD1).
//
// Brief teaches REQUIRED → engine enforces REQUIRED. Without this,
// the substrate-side teaching is the only defense against missing-
// section regression — and a single agent skip silently ships an
// incomplete apps-repo README (run-34 evidence: cc-content-api
// recorded the un-slotted IG fragment carrying both H2 sections
// with bodyBytes:1684 ok:true; published apidev README still shipped
// without RF1+PD1 because of the un-slotted IG stitch bug, and there
// was no engine gate to catch the regression after the stitch fix
// landed).
//
// "Canonical apps-repo" = the central app's codebase per
// briefs/refinement/derived_rules.md §RF1 ("for multi-codebase
// recipes, the api codebase owns this section"). Resolved via
// Plan.CanonicalAppsRepoCodebase: api-role first, then a single
// non-worker codebase. Recipes without an identifiable canonical
// apps-repo (worker-only plans, etc.) skip the gate.
//
// Pre-scaffold codebases (no SourceRoot) skip the gate — the README
// can't assemble without a SourceRoot. Codebase-content can't run
// without SourceRoot either, so this is a pure defense-in-depth case.

// rf1HeadingMarker matches the canonical "## Recipe features" H2.
// Case-insensitive prefix match keeps minor capitalization drift from
// silently bypassing the gate (e.g. "## Recipe Features").
const (
	rf1HeadingTitle = "## Recipe features"
	pd1HeadingTitle = "## Production vs. Development"
)

// gateRequireRF1PD1OnCanonicalAppsRepo wraps requireRF1PD1OnCanonicalAppsRepo
// for the GateContext signature. Registered in CodebaseContentGates +
// FinalizeGates so missing sections surface at the earliest close
// where they MUST be present (codebase-content owns content authoring;
// finalize re-runs codebase gates per spec §G).
func gateRequireRF1PD1OnCanonicalAppsRepo(ctx GateContext) []Violation {
	return requireRF1PD1OnCanonicalAppsRepo(ctx.Plan)
}

// requireRF1PD1OnCanonicalAppsRepo returns blocking violations when
// the canonical apps-repo's assembled README is missing RF1
// (`## Recipe features`) or PD1 (`## Production vs. Development`).
// No-op for plans without an identifiable canonical apps-repo or
// when that codebase is pre-scaffold (no SourceRoot).
func requireRF1PD1OnCanonicalAppsRepo(plan *Plan) []Violation {
	if plan == nil {
		return nil
	}
	cb, ok := plan.CanonicalAppsRepoCodebase()
	if !ok {
		return nil
	}
	if cb.SourceRoot == "" {
		return nil
	}
	body, _, err := AssembleCodebaseREADME(plan, cb.Hostname)
	if err != nil {
		// Assembly failure is surfaced by other gates; we don't
		// double-report here. Returning nil keeps this gate
		// orthogonal — RF1/PD1 presence is checked only when the
		// README assembles cleanly.
		return nil
	}
	// Path attribute uses the codebase's SourceRoot/README.md shape so
	// path-prefix filters used by scoped close (CompletePhaseScoped)
	// route this violation to the correct codebase scope. Pinned by
	// TestCompletePhaseScoped_VerdictEquivalentToFullPhaseSlice.
	readmePath := filepath.Join(cb.SourceRoot, "README.md")
	var out []Violation
	if !containsHeading(body, rf1HeadingTitle) {
		out = append(out, Violation{
			Code:     "canonical-apps-repo-missing-rf1",
			Path:     readmePath,
			Severity: SeverityBlocking,
			Message: fmt.Sprintf(
				"canonical apps-repo (%s) README missing required H2 %q. The api/central-app codebase owns the recipe-level `Recipe features` bullet list per briefs/refinement/derived_rules.md §RF1. Author the section as part of `codebase/%s/integration-guide` (un-slotted, append after the numbered IG steps) or as a sibling un-slotted fragment; rerun `complete-phase phase=codebase-content`.",
				cb.Hostname, rf1HeadingTitle, cb.Hostname,
			),
		})
	}
	if !containsHeading(body, pd1HeadingTitle) {
		out = append(out, Violation{
			Code:     "canonical-apps-repo-missing-pd1",
			Path:     readmePath,
			Severity: SeverityBlocking,
			Message: fmt.Sprintf(
				"canonical apps-repo (%s) README missing required H2 %q. The api/central-app codebase owns the recipe-level `Production vs. Development` 3-bullet HA-migration guide per briefs/refinement/derived_rules.md §PD1. Author the section as part of `codebase/%s/integration-guide` (un-slotted, append after the numbered IG steps); rerun `complete-phase phase=codebase-content`.",
				cb.Hostname, pd1HeadingTitle, cb.Hostname,
			),
		})
	}
	return out
}

// containsHeading reports whether body has a markdown H2 line whose
// title matches want (case-insensitive). Matches `^## <title>` only;
// does not match deeper headings or inline mentions.
func containsHeading(body, want string) bool {
	wantLower := strings.ToLower(want)
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.EqualFold(trimmed, want) {
			return true
		}
		// Allow trailing punctuation drift (e.g. "## Recipe features:")
		// without false-negatives, but only on exact prefix match.
		if lower := strings.ToLower(trimmed); strings.HasPrefix(lower, wantLower) {
			return true
		}
	}
	return false
}
