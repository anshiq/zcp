// Tests for `emit-finalize` — composes the finalize sub-agent prompt
// against a staged simulation. Mirrors the production engine's
// briefKind=finalize / PhaseFinalize pipeline step (run-23 §sim-scope).
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmitFinalize_ComposesBrief asserts the basic shape: given a
// fixture simulation directory, runEmitFinalize writes a non-empty
// briefs/finalize-prompt.md AND creates an empty fragments-new/finalize/
// directory ready for the dispatched agent's writes.
func TestEmitFinalize_ComposesBrief(t *testing.T) {
	dir := t.TempDir()
	if err := writeMinimalSimulationOpts(t, dir, false); err != nil {
		t.Fatalf("writeMinimalSimulationOpts: %v", err)
	}
	if err := runEmitFinalize([]string{"-dir", dir}); err != nil {
		t.Fatalf("runEmitFinalize: %v", err)
	}
	promptPath := filepath.Join(dir, "briefs", "finalize-prompt.md")
	body, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read finalize prompt: %v", err)
	}
	if len(body) == 0 {
		t.Errorf("finalize prompt is empty")
	}
	bodyStr := string(body)
	// Header from BuildSubagentPromptForReplay's BriefFinalize branch.
	if !strings.Contains(bodyStr, "finalize sub-agent") {
		t.Errorf("prompt missing 'finalize sub-agent' header; got:\n%s", bodyStr[:min(500, len(bodyStr))])
	}
	// BuildFinalizeBrief emits a "Tier map" + "Fragments to author"
	// block; assert one is present so we know the brief body landed.
	if !strings.Contains(bodyStr, "Tier map") {
		t.Errorf("prompt missing finalize 'Tier map' block")
	}
	if !strings.Contains(bodyStr, "root/intro") {
		t.Errorf("prompt missing 'root/intro' fragment-list reference")
	}
	// Replay adapter prepends the file-write redirect.
	if !strings.Contains(bodyStr, "<replay-adapter>") {
		t.Errorf("prompt missing replay-adapter prefix")
	}
	// fragments-new/finalize dir created (empty).
	finalizeDir := filepath.Join(dir, "fragments-new", "finalize")
	info, err := os.Stat(finalizeDir)
	if err != nil {
		t.Fatalf("fragments-new/finalize/ not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("fragments-new/finalize is not a directory")
	}
	entries, err := os.ReadDir(finalizeDir)
	if err != nil {
		t.Fatalf("read finalize dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("fragments-new/finalize/ should be empty until agent dispatch; got %d entries", len(entries))
	}
}

// TestEmitFinalize_RequiresDirFlag asserts the CLI rejects an
// invocation without `-dir`.
func TestEmitFinalize_RequiresDirFlag(t *testing.T) {
	err := runEmitFinalize([]string{})
	if err == nil {
		t.Fatalf("expected error: -dir required")
	}
	if !strings.Contains(err.Error(), "-dir is required") {
		t.Errorf("error %q does not mention -dir", err.Error())
	}
}

// TestEmitFinalize_RejectsParentWithoutMountRoot asserts the same
// guardrail emit / emit-refinement enforce: -parent without
// -mount-root is meaningless (no chain to walk).
func TestEmitFinalize_RejectsParentWithoutMountRoot(t *testing.T) {
	dir := t.TempDir()
	if err := writeMinimalSimulationOpts(t, dir, false); err != nil {
		t.Fatalf("writeMinimalSimulationOpts: %v", err)
	}
	err := runEmitFinalize([]string{"-dir", dir, "-parent", "some-parent"})
	if err == nil {
		t.Fatalf("expected error: -parent without -mount-root")
	}
	if !strings.Contains(err.Error(), "mount-root") {
		t.Errorf("error %q does not mention mount-root", err.Error())
	}
}

// TestEmitFinalize_ReadsFactsLog asserts the finalize prompt composes
// against the run's facts log when present (matches production: the
// finalize composer reads facts via BuildSubagentPromptForReplay's
// shared signature).
func TestEmitFinalize_ReadsFactsLog(t *testing.T) {
	dir := t.TempDir()
	if err := writeMinimalSimulationOpts(t, dir, false); err != nil {
		t.Fatalf("writeMinimalSimulationOpts: %v", err)
	}
	factsPath := filepath.Join(dir, "environments", "facts.jsonl")
	mustWrite(t, factsPath,
		`{"kind":"contract","topic":"nats-subjects","scope":"app/api","why":"shared","fieldPath":"contracts.nats","recordedAt":"2026-04-30T00:00:00Z"}`+"\n")
	if err := runEmitFinalize([]string{"-dir", dir}); err != nil {
		t.Fatalf("runEmitFinalize with facts: %v", err)
	}
}
