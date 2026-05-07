package recipe

import (
	"strings"
	"testing"
)

// Run-29 Fix #3 — per_tier_authoring atom + rubric edits.
//
// The within-tier scope rule prevents the run-28 "Switch ... at tier 5"
// drift in service-block comments at tier 3/4. Friendly-authority
// phrasings must name a knob the porter turns WITHIN this tier's yaml,
// not a path that crosses the tier ladder. The rubric anchor at
// embedded_rubric.md gains a `within-tier` qualifier; the forbidden-
// narrative section extends to cover yaml service-block comments AND
// READMEs (refinement-time Notice, NOT a finalize-blocking gate).

func TestPerTierAuthoringAtom_WithinTierScopeSection_Present(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/env-content/per_tier_authoring.md")
	if err != nil {
		t.Fatalf("read per_tier_authoring.md: %v", err)
	}
	if !strings.Contains(body, "## Porter signals MUST be reachable within THIS tier's deployment shape") {
		t.Errorf("per_tier_authoring.md missing within-tier scope section heading")
	}
}

func TestPerTierAuthoringAtom_NamesWithinTierKnobs(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/env-content/per_tier_authoring.md")
	if err != nil {
		t.Fatalf("read per_tier_authoring.md: %v", err)
	}
	for _, want := range []string{
		"`verticalAutoscaling.minRam`",
		"`minFreeRamGB`",
		"`objectStorageSize`",
		"`enableSubdomainAccess`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("per_tier_authoring.md within-tier knob list missing %q", want)
		}
	}
}

func TestPerTierAuthoringAtom_NamesCrossTierMoves(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/env-content/per_tier_authoring.md")
	if err != nil {
		t.Fatalf("read per_tier_authoring.md: %v", err)
	}
	for _, want := range []string{
		"`mode: NON_HA`",
		"`mode: HA`",
		"`minContainers` jumps",
		"`corePackage` tier changes",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("per_tier_authoring.md cross-tier moves list missing %q", want)
		}
	}
}

func TestPerTierAuthoringAtom_BadGoodExamples_BothPresent(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/env-content/per_tier_authoring.md")
	if err != nil {
		t.Fatalf("read per_tier_authoring.md: %v", err)
	}
	idx := strings.Index(body, "## Porter signals MUST be reachable within THIS tier's deployment shape")
	if idx < 0 {
		t.Fatal("within-tier section heading missing")
	}
	// Anchor on the next ## heading (the worked-example section that
	// follows). The Worked example block lives BETWEEN the within-tier
	// scope heading and the next ## heading.
	rest := body[idx:]
	end := strings.Index(rest[1:], "\n## ")
	if end < 0 {
		t.Fatal("within-tier section has no terminating ## heading")
	}
	window := rest[:end+1]
	for _, want := range []string{
		"**BAD**",
		"**GOOD**",
		"Switch mode to HA",          // BAD example
		"`minRam`",                   // GOOD example anchor (inline in text)
		"verticalAutoscaling.minRam", // GOOD example yaml
	} {
		if !strings.Contains(window, want) {
			t.Errorf("within-tier section missing BAD/GOOD anchor %q", want)
		}
	}
}

// TestPerTierAuthoringAtom_ArrivesAtTierBadExample_Present — F-33.
// Run-29 dogfood evidence: tier 4 db block at runs/29/environments/
// "4 — Small Production"/import.yaml:62-63 shipped "true failover
// arrives at tier 5 with `mode: HA`" — a cross-tier reference the
// existing BAD example (which uses "Switch mode to HA") didn't preempt
// because the agent's verb ("arrives") fell outside the BAD example's
// vocabulary. Atom now carries a second BAD example using the verb
// shapes that slipped past in run-29 ("arrives at tier N" / "available
// at tier N" / "comes online at tier N") with the matching GOOD
// revision: elide the cross-tier reference entirely (the tier table in
// the root README is where the porter learns tier 5 has HA).
func TestPerTierAuthoringAtom_ArrivesAtTierBadExample_Present(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/env-content/per_tier_authoring.md")
	if err != nil {
		t.Fatalf("read per_tier_authoring.md: %v", err)
	}
	for _, want := range []string{
		"arrives at tier",
		"available at tier",
		"comes online at tier",
		"unlocks at tier",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("per_tier_authoring.md missing arrives-at-tier-N BAD example anchor %q", want)
		}
	}
}

func TestEmbeddedRubric_RubricAnchor_ReferencesWithinTier(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/refinement/embedded_rubric.md")
	if err != nil {
		t.Fatalf("read embedded_rubric.md: %v", err)
	}
	for _, want := range []string{
		"1–2 within-tier",
		"3+ within-tier with named signals",
		"≥3 within-tier with named signals AND each tied to a real adapt path the porter would take by editing THIS yaml",
		"The `within-tier` qualifier is load-bearing",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("embedded_rubric.md rubric scoring anchor missing within-tier qualifier %q", want)
		}
	}
}

func TestEmbeddedRubric_TierPromotionForbid_CoversYAMLServiceBlocks(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/refinement/embedded_rubric.md")
	if err != nil {
		t.Fatalf("read embedded_rubric.md: %v", err)
	}
	for _, want := range []string{
		"### Tier-promotion narrative — README extracts AND yaml service-block comments",
		"yaml comments inside service blocks",
		"refinement-time Notice",
		"Finalize closure does\nNOT block on this regex set",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("embedded_rubric.md forbidden-narrative section missing yaml-comment scope anchor %q", want)
		}
	}
}
