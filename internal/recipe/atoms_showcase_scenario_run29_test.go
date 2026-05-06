package recipe

import (
	"strings"
	"testing"
)

// Run-29 Fix #5 — showcase_scenario atom: dev-loop teaching.
//
// Run-28 features-frontend agent dispatched 8 cross-deploys
// appdev→appstage debugging one card. The brief described the WHAT
// (cards + states + browser-walk) but not the HOW (iterate on appdev
// HMR; cross-deploy ONCE per feature-pass close). The new section pins
// the four-step loop + when-cross-deploy-IS-right + when-cross-deploy-
// is-WRONG lists.

func TestShowcaseScenarioAtom_DevLoopSection_Present(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/feature/showcase_scenario.md")
	if err != nil {
		t.Fatalf("read showcase_scenario.md: %v", err)
	}
	if !strings.Contains(body, "## Dev loop — appdev HMR first, cross-deploy last") {
		t.Errorf("showcase_scenario.md missing dev-loop section heading")
	}
}

func TestShowcaseScenarioAtom_FourStepLoop_AllStepsNamed(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/feature/showcase_scenario.md")
	if err != nil {
		t.Fatalf("read showcase_scenario.md: %v", err)
	}
	for _, want := range []string{
		"**Author the card on appdev.**",
		"**Browser-walk on appdev**",
		"**Iterate WITHIN appdev.**",
		"**Cross-deploy to appstage ONCE per feature-pass close.**",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("showcase_scenario.md dev-loop missing step anchor %q", want)
		}
	}
}

func TestShowcaseScenarioAtom_WhenCrossDeployRight_ListPresent(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/feature/showcase_scenario.md")
	if err != nil {
		t.Fatalf("read showcase_scenario.md: %v", err)
	}
	for _, want := range []string{
		"### When cross-deploy IS the right tool",
		"build-time env-var bake",
		"`VITE_API_URL`",
		"CORS / cross-origin / TLS",
		"feature-pass is closing",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("showcase_scenario.md when-cross-deploy-right list missing %q", want)
		}
	}
}

func TestShowcaseScenarioAtom_WhenCrossDeployWrong_ListPresent(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/feature/showcase_scenario.md")
	if err != nil {
		t.Fatalf("read showcase_scenario.md: %v", err)
	}
	for _, want := range []string{
		"### When cross-deploy is the WRONG tool",
		"The click handler doesn't fire",
		"A fetch returns wrong data",
		"A card renders incorrectly",
		"ANY in-bundle behavior",
		"cross-deployed the same source twice in a row",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("showcase_scenario.md when-cross-deploy-wrong list missing %q", want)
		}
	}
}
