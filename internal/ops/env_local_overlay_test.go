package ops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureEnvLocal_CreatesWhenAbsent pins the create-once contract:
// absent file → ZCP writes the header + seed entries. Used by
// recipe-local bootstrap (Theme 1) and brownfield-adopt (Theme 3).
func TestEnsureEnvLocal_CreatesWhenAbsent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	seed := map[string]string{
		"APP_ENV":   "local",
		"LOG_LEVEL": "debug",
	}
	if err := EnsureEnvLocal(tmpDir, seed); err != nil {
		t.Fatalf("EnsureEnvLocal: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(tmpDir, ".env.local"))
	if err != nil {
		t.Fatalf("read .env.local: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "Created by ZCP") {
		t.Errorf("missing ZCP header; got:\n%s", got)
	}
	if !strings.Contains(got, "APP_ENV=local") || !strings.Contains(got, "LOG_LEVEL=debug") {
		t.Errorf("seed entries missing; got:\n%s", got)
	}
}

// TestEnsureEnvLocal_RefusesWhenPresent pins the no-overwrite contract.
// Once .env.local exists ZCP MUST NOT touch it — it's the user's
// no-touch zone. Caller decides whether ErrEnvLocalAlreadyExists is
// expected (e.g. re-running bootstrap on already-set-up project).
func TestEnsureEnvLocal_RefusesWhenPresent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	const userContent = "USER_AUTHORED=should-survive\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env.local"), []byte(userContent), 0600); err != nil {
		t.Fatalf("write .env.local: %v", err)
	}

	err := EnsureEnvLocal(tmpDir, map[string]string{"WOULD_BE_SEEDED": "ignored"})
	if !errors.Is(err, ErrEnvLocalAlreadyExists) {
		t.Errorf("expected ErrEnvLocalAlreadyExists, got: %v", err)
	}

	// Critical: existing user content untouched.
	body, _ := os.ReadFile(filepath.Join(tmpDir, ".env.local"))
	if string(body) != userContent {
		t.Errorf("existing .env.local was modified; got:\n%s", string(body))
	}
}

// TestEnsureEnvLocal_HeaderStable pins the header text. Atom guidance
// references this header verbatim ("Created by ZCP. Edit freely —
// ZCP will not overwrite this file.") — drift here breaks
// agent-facing instructions.
func TestEnsureEnvLocal_HeaderStable(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := EnsureEnvLocal(tmpDir, nil); err != nil {
		t.Fatalf("EnsureEnvLocal: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(tmpDir, ".env.local"))
	got := string(body)
	for _, want := range []string{
		"Created by ZCP",
		"Edit freely",
		"ZCP merges these values into .env",
		"will not overwrite this file",
		".gitignore",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("header missing %q; got:\n%s", want, got)
		}
	}
}

// TestEnsureEnvLocal_NilSeed pins that nil seed produces a header-
// only file (callers may seed lazily).
func TestEnsureEnvLocal_NilSeed(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	if err := EnsureEnvLocal(tmpDir, nil); err != nil {
		t.Fatalf("EnsureEnvLocal: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(tmpDir, ".env.local"))
	if !strings.Contains(string(body), "Created by ZCP") {
		t.Error("header should still be written for nil seed")
	}
}
