package recipe

import (
	"strings"
	"testing"
)

// Run-34 Fix 2 — un-slotted IG `codebase/<host>/integration-guide`
// MUST always be appended into the assembled README, regardless of
// whether slotted IG slots (`/integration-guide/<n>`) also exist or
// the order they were recorded in.
//
// Pre-fix: mergeSlottedIGFragments OVERWROTE the un-slotted key with
// the concatenated slot bodies whenever any slotted slot existed,
// silently dropping un-slotted content. cc-content-api recorded
// `codebase/api/integration-guide` (un-slotted) carrying RF1+PD1+
// "Understand Zerops Core Concepts" H2 sections with bodyBytes:1684
// ok:true; engine acked but the published apidev README shipped
// without those sections.
//
// Diagnosed in plans/run-34-validation.md §"Top 5 surprises" #2.

func TestStitchUnslottedIG_AlwaysAppends(t *testing.T) {
	t.Parallel()

	plan := syntheticShowcasePlan()
	for i, cb := range plan.Codebases {
		plan.Codebases[i].SourceRoot = "/var/www/" + cb.Hostname + "dev"
	}

	const unslottedBody = "## Recipe features\n\n- Feature one\n- Feature two\n"
	cases := []struct {
		name      string
		fragments map[string]string
	}{
		{
			name: "unslotted only — back-compat path",
			fragments: map[string]string{
				"codebase/api/integration-guide": unslottedBody,
			},
		},
		{
			name: "slotted then unslotted — agent recorded slot first",
			fragments: map[string]string{
				"codebase/api/integration-guide/2": "### 2. Slotted item two\n",
				"codebase/api/integration-guide/3": "### 3. Slotted item three\n",
				"codebase/api/integration-guide":   unslottedBody,
			},
		},
		{
			name: "unslotted then slotted — agent recorded unslotted first",
			fragments: map[string]string{
				"codebase/api/integration-guide":   unslottedBody,
				"codebase/api/integration-guide/2": "### 2. Slotted item two\n",
				"codebase/api/integration-guide/3": "### 3. Slotted item three\n",
			},
		},
		{
			name: "slot 5 only + unslotted — sparse slot space",
			fragments: map[string]string{
				"codebase/api/integration-guide":   unslottedBody,
				"codebase/api/integration-guide/5": "### 5. Slotted item five\n",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			merged := mergeSlottedIGFragments(tc.fragments, "api")
			got := merged["codebase/api/integration-guide"]
			if !strings.Contains(got, "## Recipe features") {
				t.Errorf("un-slotted IG body dropped from merge result.\nFragments: %v\nMerged: %q",
					tc.fragments, got)
			}
		})
	}
}

// TestStitchUnslottedIG_SiblingHostsUnaffected — merging slotted IG
// for host A must not touch host B's un-slotted body.
func TestStitchUnslottedIG_SiblingHostsUnaffected(t *testing.T) {
	t.Parallel()

	frags := map[string]string{
		"codebase/api/integration-guide/2": "### 2. api slot two\n",
		"codebase/api/integration-guide":   "## Recipe features\n\nApi unslotted\n",
		"codebase/app/integration-guide":   "## Recipe features\n\nApp unslotted\n",
	}
	merged := mergeSlottedIGFragments(frags, "api")
	if got := merged["codebase/app/integration-guide"]; !strings.Contains(got, "App unslotted") {
		t.Errorf("merging api slots clobbered app's un-slotted IG body; got %q", got)
	}
}

// Run-34 Fix 2 (engine validator side) — the un-scoped main-agent
// `complete-phase phase=codebase-content` MUST refuse when the
// canonical apps-repo's assembled README is missing
// "## Recipe features" (RF1) or "## Production vs. Development" (PD1).
// Brief teaches REQUIRED → engine enforces REQUIRED, so substrate
// teaching is not the only defense against missing-section regression.
//
// Diagnosed in plans/run-34-validation.md: apidev shipped without
// RF1+PD1 even after the cc-content-api agent recorded the un-slotted
// IG fragment carrying both sections (bodyBytes:1684, ok:true).

func TestRequireRF1PD1OnCanonicalAppsRepo_MissingBoth_Blocks(t *testing.T) {
	t.Parallel()
	plan := canonicalAppsRepoTestPlan(t, "")

	violations := requireRF1PD1OnCanonicalAppsRepo(plan)
	if len(violations) == 0 {
		t.Fatal("expected blocking violation when canonical apps-repo (api) is missing both RF1 + PD1")
	}
	var sawRF1, sawPD1 bool
	for _, v := range violations {
		if strings.Contains(v.Message, "Recipe features") {
			sawRF1 = true
		}
		if strings.Contains(v.Message, "Production vs. Development") {
			sawPD1 = true
		}
		if v.Severity != SeverityBlocking {
			t.Errorf("violation %q must be blocking, got severity %v", v.Code, v.Severity)
		}
	}
	if !sawRF1 {
		t.Errorf("violations did not name RF1 (Recipe features); got %+v", violations)
	}
	if !sawPD1 {
		t.Errorf("violations did not name PD1 (Production vs. Development); got %+v", violations)
	}
}

func TestRequireRF1PD1OnCanonicalAppsRepo_HasBoth_Passes(t *testing.T) {
	t.Parallel()
	plan := canonicalAppsRepoTestPlan(t,
		"## Recipe features\n\n- Foo\n- Bar\n\n## Production vs. Development\n\n- Use HA mode\n",
	)
	if violations := requireRF1PD1OnCanonicalAppsRepo(plan); len(violations) > 0 {
		t.Errorf("expected no violation when both RF1 + PD1 present; got %+v", violations)
	}
}

func TestRequireRF1PD1OnCanonicalAppsRepo_MissingPD1_Blocks(t *testing.T) {
	t.Parallel()
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- Foo\n")
	violations := requireRF1PD1OnCanonicalAppsRepo(plan)
	if len(violations) != 1 {
		t.Fatalf("expected exactly one violation (PD1 missing); got %+v", violations)
	}
	if !strings.Contains(violations[0].Message, "Production vs. Development") {
		t.Errorf("expected PD1 violation; got %+v", violations[0])
	}
}

func TestRequireRF1PD1OnCanonicalAppsRepo_NoCanonicalAppsRepo_NoOp(t *testing.T) {
	t.Parallel()
	// Worker-only plan has no canonical apps-repo. Validator must no-op
	// rather than pick an arbitrary codebase.
	plan := &Plan{
		Slug:      "worker-only",
		Framework: "synth",
		Codebases: []Codebase{
			{Hostname: "worker", Role: RoleWorker, BaseRuntime: "nodejs@22", IsWorker: true, SourceRoot: "/var/www/workerdev"},
		},
	}
	if violations := requireRF1PD1OnCanonicalAppsRepo(plan); len(violations) > 0 {
		t.Errorf("worker-only plan should produce no RF1/PD1 violation; got %+v", violations)
	}
}

func TestRequireRF1PD1OnCanonicalAppsRepo_PreScaffold_NoOp(t *testing.T) {
	t.Parallel()
	// Pre-scaffold (no SourceRoot on the api codebase) — the validator
	// can't assemble a README without a SourceRoot. Skip rather than
	// false-positive against an early-phase plan.
	plan := &Plan{
		Slug: "pre-scaffold", Framework: "synth",
		Codebases: []Codebase{
			{Hostname: "api", Role: RoleAPI, BaseRuntime: "nodejs@22"}, // no SourceRoot
		},
	}
	if violations := requireRF1PD1OnCanonicalAppsRepo(plan); len(violations) > 0 {
		t.Errorf("pre-scaffold canonical-apps-repo should no-op; got %+v", violations)
	}
}

// TestCompletePhase_RequiresRF1PD1OnCanonicalAppsRepo — the wired-in
// gate: un-scoped main-agent `complete-phase phase=codebase-content`
// refuses when canonical-apps-repo's assembled README is missing
// RF1+PD1. Pre-fix, the missing sections shipped silently because no
// gate enforced them.
func TestCompletePhase_RequiresRF1PD1OnCanonicalAppsRepo(t *testing.T) {
	t.Parallel()
	gates := CodebaseContentGates()
	var found bool
	for _, g := range gates {
		if g.Name == "canonical-apps-repo-rf1-pd1" {
			found = true
		}
	}
	if !found {
		t.Errorf("CodebaseContentGates() must register `canonical-apps-repo-rf1-pd1`; got %v", gateNames(gates))
	}
}

// canonicalAppsRepoTestPlan returns a synthetic plan whose canonical
// apps-repo (api codebase) has an un-slotted IG fragment with the
// caller-supplied body. Body shape lets each test target one of
// "missing both" / "has both" / "missing PD1" / etc. without sharing
// state.
func canonicalAppsRepoTestPlan(t *testing.T, unslottedIGBody string) *Plan {
	t.Helper()
	plan := syntheticShowcasePlan()
	for i, cb := range plan.Codebases {
		plan.Codebases[i].SourceRoot = "/var/www/" + cb.Hostname + "dev"
	}
	plan.Fragments = map[string]string{}
	for _, cb := range plan.Codebases {
		base := "codebase/" + cb.Hostname
		plan.Fragments[base+"/intro"] = "Codebase intro paragraph.\n"
		// Whole-yaml fragment so AssembleCodebaseREADME doesn't trip on
		// the missing-on-disk-yaml hardening (M-2 in assemble.go).
		plan.Fragments[fragmentIDCodebaseZeropsYAML(cb.Hostname)] = "zerops:\n  - setup: " + cb.Hostname + "\n    build:\n      base: nodejs@22\n    run:\n      base: nodejs@22\n      start: npm start\n"
		// Two slotted IG items so the IG validator passes; we test the
		// canonical-apps-repo RF1/PD1 gate independently.
		plan.Fragments[base+"/integration-guide/2"] = "### 2. Trust the reverse proxy\n\nSet trust proxy.\n"
		plan.Fragments[base+"/integration-guide/3"] = "### 3. Drain on SIGTERM\n\nGraceful shutdown.\n"
		plan.Fragments[base+"/knowledge-base"] = "- **404 on Topic** — explanation.\n"
		plan.Fragments[base+"/claude-md"] = "# " + cb.Hostname + "\n\nApp scaffold.\n"
	}
	if unslottedIGBody != "" {
		plan.Fragments["codebase/api/integration-guide"] = unslottedIGBody
	}
	return plan
}
