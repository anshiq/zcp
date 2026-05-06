package recipe

import (
	"strings"
	"testing"
)

// Run-29 Fix #4 — synthesis_workflow atom: surface ownership +
// authoring order (IG-mechanisms first, yaml-comment-WHY-choices
// second). Refinement suspects (criterion-6-ig-yamlcomment-dup) is the
// defense-in-depth detector tested in refinement_suspects_run29_test.go.

func TestSynthesisWorkflowAtom_AuthoringOrderSection_Present(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/codebase-content/synthesis_workflow.md")
	if err != nil {
		t.Fatalf("read synthesis_workflow.md: %v", err)
	}
	for _, want := range []string{
		"## Surface ownership — mechanisms on IG, field-choices on yaml comments",
		"### Authoring order — IG first, yaml-comments second",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("synthesis_workflow.md missing surface-ownership anchor %q", want)
		}
	}
}

func TestSynthesisWorkflowAtom_NamesYAMLCommentsFirst(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/codebase-content/synthesis_workflow.md")
	if err != nil {
		t.Fatalf("read synthesis_workflow.md: %v", err)
	}
	// The authoring-order rule names IG first (mechanisms), yaml-
	// comments second (WHY-choices), KB last (post-deploy symptoms).
	// Spec is explicit: IG-first, yaml-comments-second (per spec-
	// content-surfaces.md §Surface 7). This pin guards the order.
	for _, want := range []string{
		"**Author IG #2-N first.**",
		"**Author zerops.yaml comments second",
		"**Author KB last.**",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("synthesis_workflow.md missing authoring-order anchor %q", want)
		}
	}
}

func TestSynthesisWorkflowAtom_BadGoodExamples_BothPresent(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/codebase-content/synthesis_workflow.md")
	if err != nil {
		t.Fatalf("read synthesis_workflow.md: %v", err)
	}
	idx := strings.Index(body, "### Worked example — same-key shadow trap (api codebase)")
	if idx < 0 {
		t.Fatal("worked-example heading missing")
	}
	rest := body[idx:]
	end := strings.Index(rest[1:], "\n## ")
	if end < 0 {
		end = len(rest) - 1
	}
	window := rest[:end+1]
	for _, want := range []string{
		"**BAD**",
		"**GOOD**",
		"yaml comment teaches the mechanism",
		"yaml comment owns the\nWHY-choice",
	} {
		if !strings.Contains(window, want) {
			t.Errorf("synthesis_workflow.md surface-ownership worked example missing %q", want)
		}
	}
}

func TestSynthesisWorkflowAtom_IG1EngineStampedNote_Present(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/codebase-content/synthesis_workflow.md")
	if err != nil {
		t.Fatalf("read synthesis_workflow.md: %v", err)
	}
	for _, want := range []string{
		"### Special case — IG #1 is engine-stamped",
		"the engine emit\nfulfills the contract by construction",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("synthesis_workflow.md missing IG #1 engine-stamped special-case anchor %q", want)
		}
	}
}
