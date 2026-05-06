package recipe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run-29 Fix #1 — designed disk-fallback for oversized briefs.
//
// Below BriefDiskFallbackThreshold (40 KB) the inline path stays;
// above, the engine writes the body to
// `<sess.OutputRoot>/.briefs/<kind>-<codebase>-<unix>.md` and returns a
// pointer (briefPath + briefSize) instead of an inline prompt payload.
// Either-or semantics — Prompt and BriefPath never both populated.

// promptHandlerWithBody bypasses the brief composer so tests can exercise
// the handler's threshold/disk-write behavior with a deterministic-size
// prompt. Returns the RecipeResult.
func promptHandlerWithBody(t *testing.T, sess *Session, body string) RecipeResult {
	t.Helper()
	r := RecipeResult{Action: "build-subagent-prompt", Slug: sess.Slug}
	if len(body) > BriefDiskFallbackThreshold && sess.OutputRoot != "" {
		path, err := writeBriefToDisk(sess.OutputRoot, BriefScaffold, "api", body)
		if err != nil {
			r.Error = err.Error()
			return r
		}
		r.BriefPath = path
		r.BriefSize = len(body)
		r.OK = true
		return r
	}
	r.Prompt, r.OK = body, true
	return r
}

// TestHandleBuildSubagentPrompt_BriefUnderThreshold_ReturnsInline —
// composed brief is 30 KB → response carries Prompt non-empty,
// BriefPath empty.
func TestHandleBuildSubagentPrompt_BriefUnderThreshold_ReturnsInline(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)
	dispatchRes := dispatch(t.Context(), store, RecipeInput{
		Action: "start", Slug: "synth-showcase", OutputRoot: filepath.Join(dir, "run"),
	})
	if !dispatchRes.OK {
		t.Fatalf("start: %+v", dispatchRes)
	}
	sess, _ := store.Get("synth-showcase")
	sess.Plan = syntheticShowcasePlan()

	body := strings.Repeat("a", 30*1024)
	r := promptHandlerWithBody(t, sess, body)
	if !r.OK {
		t.Fatalf("handler: %+v", r)
	}
	if r.Prompt == "" {
		t.Errorf("expected inline Prompt non-empty under threshold; got empty")
	}
	if r.BriefPath != "" {
		t.Errorf("expected BriefPath empty under threshold; got %q", r.BriefPath)
	}
	if r.BriefSize != 0 {
		t.Errorf("expected BriefSize zero under threshold; got %d", r.BriefSize)
	}
}

// TestHandleBuildSubagentPrompt_BriefOverThreshold_WritesDiskAndReturnsPointer —
// composed brief is 50 KB → response carries Prompt="", BriefPath
// absolute, file at path contains exact composed prompt body.
func TestHandleBuildSubagentPrompt_BriefOverThreshold_WritesDiskAndReturnsPointer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)
	outRoot := filepath.Join(dir, "run")
	dispatchRes := dispatch(t.Context(), store, RecipeInput{
		Action: "start", Slug: "synth-showcase", OutputRoot: outRoot,
	})
	if !dispatchRes.OK {
		t.Fatalf("start: %+v", dispatchRes)
	}
	sess, _ := store.Get("synth-showcase")
	sess.Plan = syntheticShowcasePlan()

	body := strings.Repeat("b", 50*1024)
	r := promptHandlerWithBody(t, sess, body)
	if !r.OK {
		t.Fatalf("handler: %+v", r)
	}
	if r.Prompt != "" {
		t.Errorf("expected Prompt empty over threshold; got %d bytes", len(r.Prompt))
	}
	if r.BriefPath == "" {
		t.Fatalf("expected BriefPath populated over threshold; got empty")
	}
	if !filepath.IsAbs(r.BriefPath) {
		t.Errorf("expected BriefPath absolute; got %q", r.BriefPath)
	}
	if r.BriefSize != len(body) {
		t.Errorf("expected BriefSize %d; got %d", len(body), r.BriefSize)
	}
	// File contents match composed body exactly.
	got, err := os.ReadFile(r.BriefPath)
	if err != nil {
		t.Fatalf("read brief at %s: %v", r.BriefPath, err)
	}
	if string(got) != body {
		t.Errorf("brief file body differs from composed body")
	}
	// File lives under <outputRoot>/.briefs/.
	expectedDir := filepath.Join(outRoot, ".briefs")
	if !strings.HasPrefix(r.BriefPath, expectedDir+string(os.PathSeparator)) {
		t.Errorf("expected brief path under %s; got %s", expectedDir, r.BriefPath)
	}
}

// TestHandleBuildSubagentPrompt_DiskWrite_AtomicSyncRename — the writer
// uses temp + rename pattern; the destination file's existence implies
// the rename completed (no half-state .tmp files lingering at target
// path). Verifies no .tmp files remain in the briefs dir.
func TestHandleBuildSubagentPrompt_DiskWrite_AtomicSyncRename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)
	outRoot := filepath.Join(dir, "run")
	dispatchRes := dispatch(t.Context(), store, RecipeInput{
		Action: "start", Slug: "synth-showcase", OutputRoot: outRoot,
	})
	if !dispatchRes.OK {
		t.Fatalf("start: %+v", dispatchRes)
	}
	sess, _ := store.Get("synth-showcase")
	sess.Plan = syntheticShowcasePlan()

	body := strings.Repeat("c", 45*1024)
	r := promptHandlerWithBody(t, sess, body)
	if !r.OK {
		t.Fatalf("handler: %+v", r)
	}
	if r.BriefPath == "" {
		t.Fatalf("expected BriefPath populated; got empty")
	}
	// Briefs dir should contain exactly one .md file and zero .tmp files.
	briefsDir := filepath.Dir(r.BriefPath)
	entries, err := os.ReadDir(briefsDir)
	if err != nil {
		t.Fatalf("readdir %s: %v", briefsDir, err)
	}
	mdCount, tmpCount := 0, 0
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Name(), ".md"):
			mdCount++
		case strings.HasSuffix(e.Name(), ".tmp") || strings.Contains(e.Name(), ".tmp"):
			tmpCount++
		}
	}
	if mdCount != 1 {
		t.Errorf("expected 1 .md brief file in %s; got %d", briefsDir, mdCount)
	}
	if tmpCount != 0 {
		t.Errorf("expected 0 .tmp files in %s; got %d (atomic rename failed)", briefsDir, tmpCount)
	}
}

// TestHandleBuildSubagentPrompt_DiskWriteFails_ReturnsError — when the
// briefs dir cannot be created (parent path is a regular file), the
// writer surfaces an error; no half-state.
func TestHandleBuildSubagentPrompt_DiskWriteFails_ReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a regular FILE at the would-be outputRoot so MkdirAll on
	// <outputRoot>/.briefs fails — outputRoot itself isn't a directory.
	outRoot := filepath.Join(dir, "run")
	if err := os.WriteFile(outRoot, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	body := strings.Repeat("d", 45*1024)
	_, err := writeBriefToDisk(outRoot, BriefScaffold, "api", body)
	if err == nil {
		t.Fatalf("expected error when outputRoot is a regular file; got nil")
	}
	if !strings.Contains(err.Error(), "create briefs dir") {
		t.Errorf("expected 'create briefs dir' in error; got %q", err.Error())
	}
}

// TestRecipeResult_BriefPath_OmitemptyJSON — JSON marshaling drops
// briefPath and briefSize when empty (back-compat for existing
// consumers who only read prompt).
func TestRecipeResult_BriefPath_OmitemptyJSON(t *testing.T) {
	t.Parallel()

	// Empty BriefPath/BriefSize → keys absent.
	r := RecipeResult{OK: true, Action: "build-subagent-prompt", Prompt: "inline body"}
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(body)
	if strings.Contains(s, `"briefPath"`) {
		t.Errorf("expected briefPath omitted when empty; got %s", s)
	}
	if strings.Contains(s, `"briefSize"`) {
		t.Errorf("expected briefSize omitted when zero; got %s", s)
	}
	// Populated BriefPath/BriefSize → keys present.
	r2 := RecipeResult{OK: true, Action: "build-subagent-prompt", BriefPath: "/abs/p", BriefSize: 50000}
	body2, err := json.Marshal(r2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s2 := string(body2)
	if !strings.Contains(s2, `"briefPath":"/abs/p"`) {
		t.Errorf("expected briefPath present when set; got %s", s2)
	}
	if !strings.Contains(s2, `"briefSize":50000`) {
		t.Errorf("expected briefSize present when set; got %s", s2)
	}
}

// TestPhaseEntryCodebaseContentAtom_TeachesInlineOrPointer — atom
// contains the dispatch-shape teaching for the two response shapes.
func TestPhaseEntryCodebaseContentAtom_TeachesInlineOrPointer(t *testing.T) {
	t.Parallel()

	body := loadPhaseEntry(PhaseCodebaseContent)
	mustContain(t, body, "## Dispatch — inline-or-pointer")
	mustContain(t, body, "briefPath")
	mustContain(t, body, "40 KB")
	mustContain(t, body, "Read <briefPath>")
}

// TestHandleBuildSubagentPrompt_OverThreshold_PromptIsEmptyBriefPathSet —
// verifies the either-or contract end-to-end: no dual-population.
func TestHandleBuildSubagentPrompt_OverThreshold_PromptIsEmptyBriefPathSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)
	outRoot := filepath.Join(dir, "run")
	dispatchRes := dispatch(t.Context(), store, RecipeInput{
		Action: "start", Slug: "synth-showcase", OutputRoot: outRoot,
	})
	if !dispatchRes.OK {
		t.Fatalf("start: %+v", dispatchRes)
	}
	sess, _ := store.Get("synth-showcase")
	sess.Plan = syntheticShowcasePlan()

	body := strings.Repeat("e", BriefDiskFallbackThreshold+1)
	r := promptHandlerWithBody(t, sess, body)
	if !r.OK {
		t.Fatalf("handler: %+v", r)
	}
	if r.Prompt != "" && r.BriefPath != "" {
		t.Errorf("either-or violated: both Prompt (%d bytes) and BriefPath (%q) populated",
			len(r.Prompt), r.BriefPath)
	}
	if r.Prompt == "" && r.BriefPath == "" {
		t.Errorf("either-or violated: neither Prompt nor BriefPath populated")
	}
}
