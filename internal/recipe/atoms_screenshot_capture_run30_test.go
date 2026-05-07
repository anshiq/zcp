package recipe

import (
	"strings"
	"testing"
)

// Run-30 Fix #4 — browser screenshot capture at close.
//
// Run-30 ANALYSIS surfaced that the showcase verification atoms use
// `snapshot -i -c` (accessibility tree) + `get text [selector]`
// assertions only — no pixel screenshot, no visual baseline. Visual
// defects (oval status-dots in run-30, missing card backgrounds,
// broken grid, FOUC mid-render, ...) pass through. Post-run analysis
// has no visual artifact for eyeball-diff.
//
// agent-browser supports `screenshot [path]` and `screenshot --full
// [path]`. The ZCP `zerops_browser` MCP tool passes inner commands
// through verbatim (only `open`/`close` are stripped). The frontend-
// pass close-step (integration_validator.md) is the right place for
// the teaching: AFTER the final cross-deploy verify on the stage URL,
// when the dashboard is fully wired (every status dot green, every
// card populated), capture ONE screenshot of the full page to
// `<outputRoot>/screenshots/dashboard-close.png`.

func TestIntegrationValidatorAtom_TeachesScreenshotAtClose(t *testing.T) {
	t.Parallel()
	body, err := readAtom("briefs/feature/integration_validator.md")
	if err != nil {
		t.Fatalf("read integration_validator.md: %v", err)
	}
	for _, want := range []string{
		// Section header anchoring the close-step teaching.
		"## Capture a final-state screenshot before close",
		// The exact path the screenshot lands at — load-bearing for
		// post-run analysis to know where to look.
		"screenshots/dashboard-close.png",
		// The agent-browser command shape — passes through `zerops_browser`'s
		// `commands` field as `["screenshot", "--full", "<path>"]`.
		`["screenshot"`,
		`"--full"`,
		// Anchors the timing — AFTER the final cross-deploy + verify, NOT
		// during iterative dev-loop walks (those would capture half-wired
		// state).
		"after the final cross-deploy",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("integration_validator.md missing screenshot-capture anchor %q", want)
		}
	}
}
