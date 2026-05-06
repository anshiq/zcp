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

// TestPrepareIsolatedClaudeHome_SymlinksCredentialsWhenPresent pins the
// OAuth-passthrough behavior: real ~/.claude/.credentials.json is exposed
// inside the sandbox via symlink so token refresh writes propagate.
func TestPrepareIsolatedClaudeHome_SymlinksCredentialsWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	realCreds := filepath.Join(home, ".claude", ".credentials.json")
	if err := os.MkdirAll(filepath.Dir(realCreds), 0o700); err != nil {
		t.Fatalf("seed real .claude: %v", err)
	}
	if err := os.WriteFile(realCreds, []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("seed credentials: %v", err)
	}

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	sandboxCreds := filepath.Join(sandbox, ".claude", ".credentials.json")
	target, err := os.Readlink(sandboxCreds)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", sandboxCreds, err)
	}
	if target != realCreds {
		t.Errorf("symlink target = %q, want %q", target, realCreds)
	}
}

// TestPrepareIsolatedClaudeHome_NoCredentialsNoOp pins the no-OAuth case:
// if the operator authenticates via ANTHROPIC_API_KEY rather than OAuth,
// there's no credentials file to symlink — should succeed cleanly.
func TestPrepareIsolatedClaudeHome_NoCredentialsNoOp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No .credentials.json seeded under home.

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(sandbox, ".claude", ".credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("no credentials file should be created when source absent; lstat err: %v", err)
	}
}

// TestPrepareIsolatedClaudeHome_IdempotentReRun pins that calling twice
// on the same sandbox doesn't error or duplicate. Real eval flow may
// re-prepare across scenarios that share a claude-home.
func TestPrepareIsolatedClaudeHome_IdempotentReRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	realCreds := filepath.Join(home, ".claude", ".credentials.json")
	if err := os.MkdirAll(filepath.Dir(realCreds), 0o700); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := os.WriteFile(realCreds, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	sandbox := t.TempDir()
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	if err := PrepareIsolatedClaudeHome(sandbox); err != nil {
		t.Fatalf("second prepare (idempotent): %v", err)
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
