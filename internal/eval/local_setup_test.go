package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrepareIsolatedClaudeHome_CreatesSandboxDir pins that the sandbox
// .claude directory always lands, regardless of operator state.
func TestPrepareIsolatedClaudeHome_CreatesSandboxDir(t *testing.T) {
	// non-parallel: t.Setenv is process-global.
	home := t.TempDir()
	t.Setenv("HOME", home)

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	info, err := os.Stat(filepath.Join(sandbox, ".claude"))
	if err != nil {
		t.Fatalf("sandbox .claude missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("sandbox .claude is not a dir: %v", info.Mode())
	}
}

// TestPrepareIsolatedClaudeHome_CopiesClaudeConfigWhenPresent pins the
// OAuth-passthrough behavior: real ~/.claude.json is COPIED (not
// symlinked) into the sandbox so the eval gets valid auth at run start
// without write-back contamination of the operator's real file.
func TestPrepareIsolatedClaudeHome_CopiesClaudeConfigWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realConfig := filepath.Join(home, ".claude.json")
	original := []byte(`{"oauth":"operator-token","scratch":"a"}`)
	if err := os.WriteFile(realConfig, original, 0o600); err != nil {
		t.Fatalf("seed real .claude.json: %v", err)
	}

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	sandboxConfig := filepath.Join(sandbox, ".claude.json")
	got, err := os.ReadFile(sandboxConfig)
	if err != nil {
		t.Fatalf("read sandbox config: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("sandbox config != operator config (snapshot copy expected)")
	}
	// Must NOT be a symlink (otherwise eval writes pollute operator file).
	info, err := os.Lstat(sandboxConfig)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("sandbox config must be a copy, not a symlink — symlink would let eval writes pollute operator's real ~/.claude.json")
	}

	// Verify isolation: writing to sandbox does NOT change operator file.
	if err := os.WriteFile(sandboxConfig, []byte(`{"new":"sandbox-only"}`), 0o600); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}
	stillOriginal, err := os.ReadFile(realConfig)
	if err != nil {
		t.Fatalf("re-read real: %v", err)
	}
	if string(stillOriginal) != string(original) {
		t.Errorf("operator's real ~/.claude.json was modified by sandbox write — isolation broken")
	}
}

// TestPrepareIsolatedClaudeHome_NoCredentialsNoOp pins the no-OAuth case:
// if the operator authenticates via ANTHROPIC_API_KEY rather than OAuth,
// there's no ~/.claude.json to copy — should succeed cleanly.
func TestPrepareIsolatedClaudeHome_NoCredentialsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No ~/.claude.json seeded.

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(sandbox, ".claude.json")); !os.IsNotExist(err) {
		t.Fatalf("no config copy should be created when source absent; lstat err: %v", err)
	}
}

// TestPrepareIsolatedClaudeHome_IdempotentReRun pins that calling twice
// on the same sandbox doesn't error and doesn't re-copy (preserves any
// sandbox-local writes since the first prepare).
func TestPrepareIsolatedClaudeHome_IdempotentReRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	realConfig := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(realConfig, []byte(`{"v":1}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	// Sandbox-local edit (simulates claude session writes during run).
	sandboxConfig := filepath.Join(sandbox, ".claude.json")
	if err := os.WriteFile(sandboxConfig, []byte(`{"sandbox-modified":true}`), 0o600); err != nil {
		t.Fatalf("sandbox edit: %v", err)
	}
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("second prepare (idempotent): %v", err)
	}
	got, _ := os.ReadFile(sandboxConfig)
	if string(got) != `{"sandbox-modified":true}` {
		t.Errorf("idempotent prepare must NOT clobber sandbox writes; got: %s", got)
	}
}

// TestPrepareIsolatedClaudeHome_EmptyClaudeHomeRejects pins the input
// validation: caller-side bug if claudeHome is empty.
func TestPrepareIsolatedClaudeHome_EmptyClaudeHomeRejects(t *testing.T) {
	t.Parallel()
	if err := PrepareIsolatedClaudeHome(""); err == nil {
		t.Fatal("empty claudeHome must error")
	}
}

// TestRunner_ClaudeEnv_NilWhenClaudeHomeUnset pins container-mode
// behavior: cmd.Env=nil means inherit os.Environ — exec preserves
// ZCP_API_KEY, PATH, etc. without us building an explicit list.
func TestRunner_ClaudeEnv_NilWhenClaudeHomeUnset(t *testing.T) {
	t.Parallel()
	r := &Runner{config: RunnerConfig{}}
	if got := r.claudeEnv(); got != nil {
		t.Fatalf("empty ClaudeHome → claudeEnv = nil; got: %v", got)
	}
}

// TestComputeLocalRunPaths_ShapeAndSeparation pins the directory layout:
// workdir under /tmp (so rm-rf cannot reach the repo), results inside
// the repo (so the local Claude session can read them).
func TestComputeLocalRunPaths_ShapeAndSeparation(t *testing.T) {
	t.Parallel()
	p := ComputeLocalRunPaths("20260506-130000", "scenario-x", "/Users/op/repo")

	if !strings.HasPrefix(p.WorkDir, "/tmp/zcp-flow-eval-local/") {
		t.Errorf("WorkDir %q must live under /tmp/zcp-flow-eval-local/", p.WorkDir)
	}
	if !strings.Contains(p.WorkDir, "20260506-130000/scenario-x") {
		t.Errorf("WorkDir %q must include suiteID + scenarioID", p.WorkDir)
	}
	if strings.HasPrefix(p.WorkDir, "/Users/op/repo") {
		t.Errorf("WorkDir %q must NOT live under the source repo", p.WorkDir)
	}
	if !strings.HasPrefix(p.ResultsDir, "/Users/op/repo/eval/behavioral/runs-local") {
		t.Errorf("ResultsDir %q must be repo/eval/behavioral/runs-local for inspection", p.ResultsDir)
	}
	if !strings.HasSuffix(p.Sentinel, "/.zcp-eval-workdir") {
		t.Errorf("Sentinel %q must end in /.zcp-eval-workdir", p.Sentinel)
	}
	if !strings.HasPrefix(p.Sentinel, p.WorkDir) {
		t.Errorf("Sentinel %q must live inside WorkDir %q", p.Sentinel, p.WorkDir)
	}
}

// TestPrepareLocalRunDirs_CreatesAllPathsAndSentinel pins the
// dir-creation sequence: workdir + claude-home + results all mkdir -p,
// sentinel touched.
func TestPrepareLocalRunDirs_CreatesAllPathsAndSentinel(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	p := LocalRunPaths{
		WorkDir:    filepath.Join(tmp, "wd"),
		ClaudeHome: filepath.Join(tmp, "ch"),
		ResultsDir: filepath.Join(tmp, "results"),
		Sentinel:   filepath.Join(tmp, "wd", ".zcp-eval-workdir"),
	}
	if err := PrepareLocalRunDirs(p); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	for _, dir := range []string{p.WorkDir, p.ClaudeHome, p.ResultsDir} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Errorf("dir %q missing or not a dir: %v", dir, err)
		}
	}
	if _, err := os.Stat(p.Sentinel); err != nil {
		t.Errorf("sentinel missing at %q: %v", p.Sentinel, err)
	}
}

// TestAssertLocalRunPrereqs_MissingTokenFailsLoud pins the most common
// operator setup miss.
func TestAssertLocalRunPrereqs_MissingTokenFailsLoud(t *testing.T) {
	t.Setenv("ZCP_API_KEY", "")
	err := AssertLocalRunPrereqs()
	if err == nil {
		t.Fatal("missing ZCP_API_KEY must error")
	}
	if !strings.Contains(err.Error(), "ZCP_API_KEY") {
		t.Errorf("error must mention ZCP_API_KEY; got: %v", err)
	}
}

// TestAssertLocalRunPrereqs_TokenSetButZcpMissing pins the post-token,
// pre-install case: token is exported, zcp not in PATH yet.
func TestAssertLocalRunPrereqs_TokenSetButZcpMissing(t *testing.T) {
	t.Setenv("ZCP_API_KEY", "fake-token")
	t.Setenv("PATH", t.TempDir()) // PATH with no zcp

	err := AssertLocalRunPrereqs()
	if err == nil {
		t.Fatal("missing zcp in PATH must error")
	}
	if !strings.Contains(err.Error(), "zcp") || !strings.Contains(err.Error(), "make install") {
		t.Errorf("error must mention zcp + make install; got: %v", err)
	}
}

// TestRunner_ClaudeEnv_OverridesHomeWhenClaudeHomeSet pins local-mode
// isolation: cmd.Env contains the parent env PLUS HOME=<sandbox> so
// claude resolves ~/.claude/ inside the sandbox, while inheriting
// every other env var (incl. ZCP_API_KEY for the spawned MCP server's
// auth).
func TestRunner_ClaudeEnv_OverridesHomeWhenClaudeHomeSet(t *testing.T) {
	// non-parallel: t.Setenv mutates process env.
	t.Setenv("ZCP_API_KEY", "test-token-from-parent")

	r := &Runner{config: RunnerConfig{ClaudeHome: "/tmp/eval-sandbox-home"}}
	env := r.claudeEnv()
	if env == nil {
		t.Fatal("ClaudeHome set → claudeEnv must return non-nil env list")
	}

	var sawHome, sawAPIKey bool
	homeValue := ""
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "HOME="):
			sawHome = true
			homeValue = strings.TrimPrefix(kv, "HOME=")
		case strings.HasPrefix(kv, "ZCP_API_KEY="):
			sawAPIKey = true
		}
	}
	if !sawHome {
		t.Error("HOME entry missing from claudeEnv output")
	}
	// append wins: the LAST HOME= entry wins per exec semantics, so the
	// override must be present (we don't care if an inherited HOME= also
	// appears earlier).
	if homeValue != "/tmp/eval-sandbox-home" {
		t.Errorf("HOME override = %q, want %q", homeValue, "/tmp/eval-sandbox-home")
	}
	if !sawAPIKey {
		t.Error("ZCP_API_KEY from parent env missing — claude→zcp inheritance would break")
	}
}
