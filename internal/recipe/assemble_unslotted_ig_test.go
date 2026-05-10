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

// v9.79.0 Fix 1 — engine-side teeth for the absence axis on
// non-canonical apps-repos: the canonical apps-repo OWNS RF1/PD1; sibling
// apps-repos (non-worker, non-canonical) MUST NOT carry these recipe-
// level H2 sections. Run-37 regression: substrate fixes 3-5 in v9.78.0
// caused the agent to over-apply RF1+PD1 stitching onto every apps-repo,
// not just the canonical one; the existing presence gate catches missing
// sections on canonical, NOT duplicated sections on siblings.

func TestForbidRF1PD1OnNonCanonicalAppsRepos_HasRF1OnNonCanonical_Blocks(t *testing.T) {
	t.Parallel()
	// canonicalAppsRepoTestPlan sets SourceRoot for every codebase and
	// authors the un-slotted IG fragment under the canonical (api)
	// codebase. We override the app codebase's IG fragment to inject
	// the forbidden RF1 H2.
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- ok\n\n## Production vs. Development\n\n- ok\n")
	plan.Fragments["codebase/app/integration-guide"] = "## Recipe features\n\n- forbidden\n"

	violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan)
	if len(violations) == 0 {
		t.Fatal("expected blocking violation when non-canonical apps-repo (app) carries RF1")
	}
	var sawRF1 bool
	for _, v := range violations {
		if v.Code != "non-canonical-apps-repo-has-rf1" {
			continue
		}
		sawRF1 = true
		if v.Severity != SeverityBlocking {
			t.Errorf("violation %q must be blocking, got severity %v", v.Code, v.Severity)
		}
		if !strings.Contains(v.Message, "Recipe features") {
			t.Errorf("violation message must name RF1 (`Recipe features`); got %q", v.Message)
		}
	}
	if !sawRF1 {
		t.Errorf("expected `non-canonical-apps-repo-has-rf1` violation; got %+v", violations)
	}
}

func TestForbidRF1PD1OnNonCanonicalAppsRepos_HasPD1OnNonCanonical_Blocks(t *testing.T) {
	t.Parallel()
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- ok\n\n## Production vs. Development\n\n- ok\n")
	plan.Fragments["codebase/app/integration-guide"] = "## Production vs. Development\n\n- forbidden\n"

	violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan)
	if len(violations) == 0 {
		t.Fatal("expected blocking violation when non-canonical apps-repo (app) carries PD1")
	}
	var sawPD1 bool
	for _, v := range violations {
		if v.Code != "non-canonical-apps-repo-has-pd1" {
			continue
		}
		sawPD1 = true
		if v.Severity != SeverityBlocking {
			t.Errorf("violation %q must be blocking, got severity %v", v.Code, v.Severity)
		}
		if !strings.Contains(v.Message, "Production vs. Development") {
			t.Errorf("violation message must name PD1 (`Production vs. Development`); got %q", v.Message)
		}
	}
	if !sawPD1 {
		t.Errorf("expected `non-canonical-apps-repo-has-pd1` violation; got %+v", violations)
	}
}

func TestForbidRF1PD1OnNonCanonicalAppsRepos_AllSiblingsClean_Passes(t *testing.T) {
	t.Parallel()
	// Helper baseline: canonical (api) carries RF1+PD1 via un-slotted IG
	// body; sibling app/worker have only the slotted IG items + intro/KB,
	// no recipe-level H2.
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- ok\n\n## Production vs. Development\n\n- ok\n")
	if violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan); len(violations) > 0 {
		t.Errorf("expected zero violations when sibling apps-repos carry no RF1/PD1; got %+v", violations)
	}
}

func TestForbidRF1PD1OnNonCanonicalAppsRepos_RF1OnCanonical_NotFlagged(t *testing.T) {
	t.Parallel()
	// Canonical (api) carries RF1+PD1 — required by the presence gate.
	// Absence gate must NOT flag canonical. Other siblings clean.
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- canonical\n\n## Production vs. Development\n\n- canonical\n")
	for _, v := range forbidRF1PD1OnNonCanonicalAppsRepos(plan) {
		if strings.Contains(v.Path, "/var/www/apidev/") {
			t.Errorf("absence gate flagged canonical apidev README; violations: %+v", v)
		}
	}
}

func TestForbidRF1PD1OnNonCanonicalAppsRepos_WorkerWithRF1_NoOp(t *testing.T) {
	t.Parallel()
	// Worker codebases are skipped by the gate — the "## Understand
	// Zerops Core Concepts" link is the only recipe-level content
	// workers carry, and RF1/PD1 don't apply there.
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- ok\n\n## Production vs. Development\n\n- ok\n")
	plan.Fragments["codebase/worker/integration-guide"] = "## Recipe features\n\n- forbidden-but-skipped\n"

	violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan)
	for _, v := range violations {
		if strings.Contains(v.Path, "workerdev") {
			t.Errorf("absence gate must skip workers; got violation against worker: %+v", v)
		}
	}
}

func TestForbidRF1PD1OnNonCanonicalAppsRepos_PreScaffold_NoOp(t *testing.T) {
	t.Parallel()
	// Pre-scaffold (no SourceRoot on the app codebase) — gate skips
	// because the README can't assemble without a SourceRoot. Same
	// reason as the presence gate.
	plan := &Plan{
		Slug: "pre-scaffold", Framework: "synth",
		Codebases: []Codebase{
			{Hostname: "api", Role: RoleAPI, BaseRuntime: "nodejs@22", SourceRoot: "/var/www/apidev"},
			{Hostname: "app", Role: RoleFrontend, BaseRuntime: "nodejs@22"}, // no SourceRoot
		},
		Fragments: map[string]string{
			"codebase/app/integration-guide": "## Recipe features\n\n- forbidden\n",
		},
	}
	if violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan); len(violations) > 0 {
		t.Errorf("pre-scaffold non-canonical apps-repo should no-op; got %+v", violations)
	}
}

// TestForbidRF1PD1OnNonCanonicalAppsRepos_NoCanonicalAppsRepo_NoOp —
// when CanonicalAppsRepoCodebase() returns ok=false (worker-only plan,
// or two non-worker codebases without an api role and without a single
// non-worker), the absence gate has no canonical to compare against
// and must no-op. Without the early-return guard, the gate would scan
// every non-worker codebase against an empty canonicalHostname and
// emit violations referencing the canonical hostname as the empty
// string.
func TestForbidRF1PD1OnNonCanonicalAppsRepos_NoCanonicalAppsRepo_NoOp(t *testing.T) {
	t.Parallel()
	// Worker-only plan — CanonicalAppsRepoCodebase() returns ok=false.
	// Even if a worker codebase carried RF1 (it doesn't here), the gate
	// must not fire. Worker is also skipped by the gate's own RoleWorker
	// branch; this case isolates the no-canonical path.
	plan := &Plan{
		Slug: "worker-only", Framework: "synth",
		Codebases: []Codebase{
			{Hostname: "worker", Role: RoleWorker, BaseRuntime: "nodejs@22", IsWorker: true, SourceRoot: "/var/www/workerdev"},
		},
	}
	if violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan); len(violations) > 0 {
		t.Errorf("worker-only plan should produce no absence-gate violation; got %+v", violations)
	}

	// Two non-worker codebases, neither with RoleAPI — also no canonical.
	// CanonicalAppsRepoCodebase() takes the count==1 non-worker branch
	// only when exactly one non-worker exists; with two, it falls through
	// to ok=false. The absence gate must skip rather than scan against
	// an empty canonical.
	plan2 := &Plan{
		Slug: "no-api", Framework: "synth",
		Codebases: []Codebase{
			{Hostname: "front", Role: RoleFrontend, BaseRuntime: "nodejs@22", SourceRoot: "/var/www/frontdev"},
			{Hostname: "monolith", Role: RoleMonolith, BaseRuntime: "nodejs@22", SourceRoot: "/var/www/monolithdev"},
		},
		Fragments: map[string]string{
			fragmentIDCodebaseZeropsYAML("front"):    "zerops:\n  - setup: prod\n    run:\n      base: nodejs@22\n      start: npm start\n",
			fragmentIDCodebaseZeropsYAML("monolith"): "zerops:\n  - setup: prod\n    run:\n      base: nodejs@22\n      start: npm start\n",
			"codebase/front/integration-guide":       "## Recipe features\n\n- forbidden in canonical-only world but no canonical here\n",
			"codebase/monolith/integration-guide":    "## Production vs. Development\n\n- same\n",
		},
	}
	if violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan2); len(violations) > 0 {
		t.Errorf("plan with no canonical apps-repo should produce no absence-gate violation; got %+v", violations)
	}
}

// TestForbidRF1PD1OnNonCanonicalAppsRepos_RF1InCodeFence_NotFalsePositive —
// an IG step that quotes the literal heading text inside a fenced code
// block (e.g. teaching the porter what RF1 looks like) must NOT trip
// the absence gate. Pre-fix containsHeading scanned every line
// including those inside ``` fences, so a markdown code-block sample
// of `## Recipe features` would block the close on every non-canonical
// apps-repo.
func TestForbidRF1PD1OnNonCanonicalAppsRepos_RF1InCodeFence_NotFalsePositive(t *testing.T) {
	t.Parallel()
	plan := canonicalAppsRepoTestPlan(t, "## Recipe features\n\n- canonical retains\n## Production vs. Development\n\n- canonical retains\n")
	// Author the app codebase's IG with RF1 + PD1 ONLY inside fenced
	// code blocks — this is what an explanatory teaching step looks
	// like, not actual recipe-level content.
	plan.Fragments["codebase/app/integration-guide"] = "### 2. Example heading shapes you'll see\n\nFor reference, the canonical apps-repo carries:\n\n```markdown\n## Recipe features\n\n- bullet\n\n## Production vs. Development\n\n- bullet\n```\n\nThese sections live on the api codebase only.\n"
	violations := forbidRF1PD1OnNonCanonicalAppsRepos(plan)
	if len(violations) > 0 {
		t.Errorf("absence gate must not fire on heading-text inside code fences; got %+v", violations)
	}
}

// TestRequireRF1PD1OnCanonicalAppsRepo_RF1InCodeFence_NotFalsePositive —
// presence gate must STILL FIRE when the only RF1/PD1 in the canonical
// apps-repo is inside a code fence (teaching example, not real
// section). A fenced quote is not a real H2 in the rendered TOC, so
// the porter doesn't see those sections — the gate enforces real H2s.
// Pre-fix containsHeading would have spuriously passed this test
// because it didn't track fence state.
func TestRequireRF1PD1OnCanonicalAppsRepo_RF1InCodeFence_NotFalsePositive(t *testing.T) {
	t.Parallel()
	// Canonical IG body has RF1 + PD1 ONLY inside fenced code blocks.
	// Fence-aware containsHeading correctly reports them as missing.
	plan := canonicalAppsRepoTestPlan(t,
		"### 2. Example heading shapes\n\n```markdown\n## Recipe features\n\n- example\n\n## Production vs. Development\n\n- example\n```\n",
	)
	violations := requireRF1PD1OnCanonicalAppsRepo(plan)
	if len(violations) != 2 {
		t.Errorf("expected RF1+PD1 missing violations when only fenced examples exist; got %+v", violations)
	}
}

// TestContainsHeading_HeadingInsideCodeFence_NotMatched — direct unit
// test of containsHeading's fence-aware behavior. Pinning the helper
// directly so a future containsHeading refactor doesn't lose the
// fence-skip property without surfacing on the gate-level tests
// (which exercise multiple layers).
func TestContainsHeading_HeadingInsideCodeFence_NotMatched(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "real H2 above a fence — matched",
			body: "## Recipe features\n\n- bullet\n\n```\nirrelevant\n```\n",
			want: true,
		},
		{
			name: "H2 only inside fence — not matched",
			body: "Intro paragraph.\n\n```markdown\n## Recipe features\n```\n",
			want: false,
		},
		{
			name: "H2 only inside language-tagged fence — not matched",
			body: "Intro paragraph.\n\n```yaml\n## Recipe features\n```\n",
			want: false,
		},
		{
			name: "H2 outside fence after a fence closes — matched",
			body: "```\ncode\n```\n\n## Recipe features\n",
			want: true,
		},
		{
			name: "H2 inside fence + real H2 after — matched (real H2 wins)",
			body: "```markdown\n## Recipe features\n```\n\n## Recipe features\n",
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := containsHeading(tc.body, "## Recipe features")
			if got != tc.want {
				t.Errorf("containsHeading(...) = %v, want %v\nbody:\n%s", got, tc.want, tc.body)
			}
		})
	}
}

// TestCompletePhase_ForbidsRF1PD1OnNonCanonicalAppsRepos — the wired-in
// gate: un-scoped main-agent `complete-phase phase=codebase-content`
// refuses when a non-canonical apps-repo's assembled README carries
// RF1/PD1. Mirrors TestCompletePhase_RequiresRF1PD1OnCanonicalAppsRepo
// for the absence axis.
func TestCompletePhase_ForbidsRF1PD1OnNonCanonicalAppsRepos(t *testing.T) {
	t.Parallel()
	gates := CodebaseContentGates()
	var foundCC bool
	for _, g := range gates {
		if g.Name == "non-canonical-apps-repo-no-rf1-pd1" {
			foundCC = true
		}
	}
	if !foundCC {
		t.Errorf("CodebaseContentGates() must register `non-canonical-apps-repo-no-rf1-pd1`; got %v", gateNames(gates))
	}
	// FinalizeGates() chains CodebaseGates() which unions
	// CodebaseScaffoldGates() + CodebaseContentGates(); the absence gate
	// must be reachable through that chain so finalize re-runs it.
	finalize := FinalizeGates()
	var foundFinalize bool
	for _, g := range finalize {
		if g.Name == "non-canonical-apps-repo-no-rf1-pd1" {
			foundFinalize = true
		}
	}
	if !foundFinalize {
		t.Errorf("FinalizeGates() must include `non-canonical-apps-repo-no-rf1-pd1`; got %v", gateNames(finalize))
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
