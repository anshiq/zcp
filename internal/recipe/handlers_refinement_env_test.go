package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run-34 Fix 1 — refinement-time Replace on env fragments
// (`env/<N>/intro`, `env/<N>/import-comments/<host>`) MUST persist to
// disk, mirroring the codebase-fragment re-stitch path.
//
// Pre-fix: handlers.go's refinement transactional wrapper re-stitched
// only when host != "" (codebase fragments). Refinement called
// `record-fragment mode=replace` on env/0/intro through env/5/intro,
// engine returned ok:true for each — but the published env tier READMEs
// still showed the original priorBody. Six refinement ACTs landed in
// session state but never propagated to disk.
//
// Diagnosed in plans/run-34-validation.md §"Top 5 surprises" #1.

// TestRefinementReplaceEnvIntro_PersistsToDisk pins the fix: a
// refinement-phase Replace on env/0/intro must rewrite the published
// <outputRoot>/<tier.Folder>/README.md so the new body lands on disk.
func TestRefinementReplaceEnvIntro_PersistsToDisk(t *testing.T) {
	t.Parallel()
	sess := buildRefinementSessionWithDisk(t)

	const newIntro = "An AI-agent workspace tuned for cheap iterate-and-discard cycles."
	in := RecipeInput{
		Action:     "record-fragment",
		Slug:       sess.Slug,
		FragmentID: "env/0/intro",
		Fragment:   newIntro,
		Mode:       "replace",
	}
	r := handleRecordFragment(sess, in, RecipeResult{Action: "record-fragment", Slug: sess.Slug})
	if r.Error != "" {
		t.Fatalf("unexpected error: %s", r.Error)
	}

	tier, _ := TierAt(0)
	path := filepath.Join(sess.OutputRoot, tier.Folder, "README.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env README: %v", err)
	}
	if !strings.Contains(string(body), newIntro) {
		t.Errorf("env README on disk does NOT contain new intro %q after refinement Replace.\nFile: %s\nGot:\n%s",
			newIntro, path, string(body))
	}
}

// TestRefinementReplaceEnvImportComment_PersistsToDisk pins the fix
// for the second env-fragment shape: a Replace on
// `env/0/import-comments/<host>` must rewrite the published
// <outputRoot>/<tier.Folder>/import.yaml so the new comment lands on
// disk.
func TestRefinementReplaceEnvImportComment_PersistsToDisk(t *testing.T) {
	t.Parallel()
	sess := buildRefinementSessionWithDisk(t)

	const newComment = "Postgres for iterate-and-discard agent slots."
	in := RecipeInput{
		Action:     "record-fragment",
		Slug:       sess.Slug,
		FragmentID: "env/0/import-comments/db",
		Fragment:   newComment,
		Mode:       "replace",
	}
	r := handleRecordFragment(sess, in, RecipeResult{Action: "record-fragment", Slug: sess.Slug})
	if r.Error != "" {
		t.Fatalf("unexpected error: %s", r.Error)
	}

	tier, _ := TierAt(0)
	path := filepath.Join(sess.OutputRoot, tier.Folder, "import.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tier import.yaml: %v", err)
	}
	if !strings.Contains(string(body), newComment) {
		t.Errorf("tier import.yaml on disk does NOT contain new comment %q after refinement Replace.\nFile: %s\nGot:\n%s",
			newComment, path, string(body))
	}
}

// TestRefinementReplaceEnvIntro_NonRefinementPhase_StillPersistsViaStitch
// — the env-stitch path is wired only at PhaseRefinement (mirrors the
// codebase wrapper). Outside refinement, the unwrapped recordFragment
// path applies the Replace directly; on-disk persistence is the
// stitch-content path's responsibility (called by complete-phase as
// usual). This test pins that we don't accidentally re-stitch on every
// non-refinement Replace (which would be an unwarranted I/O cost).
func TestRefinementReplaceEnvIntro_NonRefinementPhase_NoDiskWrite(t *testing.T) {
	t.Parallel()
	sess := buildRefinementSessionWithDisk(t)
	sess.Current = PhaseCodebaseContent

	tier, _ := TierAt(0)
	path := filepath.Join(sess.OutputRoot, tier.Folder, "README.md")
	priorOnDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read prior env README: %v", err)
	}

	const newIntro = "Should not land on disk during codebase-content phase."
	in := RecipeInput{
		Action:     "record-fragment",
		Slug:       sess.Slug,
		FragmentID: "env/0/intro",
		Fragment:   newIntro,
		Mode:       "replace",
	}
	r := handleRecordFragment(sess, in, RecipeResult{Action: "record-fragment", Slug: sess.Slug})
	if r.Error != "" {
		t.Fatalf("unexpected error: %s", r.Error)
	}

	// Body landed in plan state.
	if got := sess.SnapshotFragment("env/0/intro"); got != newIntro {
		t.Errorf("plan state should hold new body; got %q", got)
	}
	// But on-disk file is still the prior content (no auto-stitch
	// outside refinement; complete-phase / stitch-content owns disk
	// writes for non-refinement phases).
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read env README: %v", err)
	}
	if string(current) != string(priorOnDisk) {
		t.Errorf("non-refinement env Replace should NOT auto-write disk; file changed.\nWant: %s\nGot:  %s",
			string(priorOnDisk), string(current))
	}
}

// buildRefinementSessionWithDisk builds a refinement session with the
// env READMEs + tier import.yamls already stitched to disk so the
// persist-to-disk tests can compare prior vs post-replace file bodies.
func buildRefinementSessionWithDisk(t *testing.T) *Session {
	t.Helper()
	sess := buildRefinementSession(t)

	// Seed env intros + per-host comments so AssembleEnvREADME and
	// EmitDeliverableYAML render content the test can detect changes
	// against.
	if sess.Plan.Fragments == nil {
		sess.Plan.Fragments = map[string]string{}
	}
	for i := range Tiers() {
		key := "env/" + tierIntStr(i) + "/intro"
		if _, ok := sess.Plan.Fragments[key]; !ok {
			sess.Plan.Fragments[key] = "Tier " + tierIntStr(i) + " — original intro paragraph."
		}
	}
	if sess.Plan.EnvComments == nil {
		sess.Plan.EnvComments = map[string]EnvComments{}
	}
	for i := range Tiers() {
		key := tierIntStr(i)
		ec := sess.Plan.EnvComments[key]
		if ec.Service == nil {
			ec.Service = map[string]string{}
		}
		if ec.Service["db"] == "" {
			ec.Service["db"] = "Postgres — original comment."
		}
		sess.Plan.EnvComments[key] = ec
	}

	// Stitch initial state so the file tree exists.
	if _, err := stitchContent(sess); err != nil {
		t.Fatalf("seed stitch: %v", err)
	}
	return sess
}

func tierIntStr(i int) string {
	return [...]string{"0", "1", "2", "3", "4", "5"}[i]
}
