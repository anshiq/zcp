package eval

import (
	"fmt"
	"os"
	"path/filepath"
)

// PrepareIsolatedClaudeHome creates an isolated HOME for the agent's
// `claude` process so the eval doesn't read or pollute the operator's
// real ~/.claude/ (auto-memory, sessions, settings, plugins, slash
// commands). It builds an empty .claude/ inside claudeHome and — if
// the operator has an OAuth credentials file at ~/.claude/.credentials.json —
// symlinks it through so the eval can authenticate Anthropic without
// forcing the operator to set ANTHROPIC_API_KEY (which `--bare` would
// require).
//
// Why symlink instead of copy: token refresh writes by claude propagate
// to the real file rather than diverging.
//
// Idempotent: re-running on an existing claudeHome is safe (existing
// symlink target is left alone via os.Symlink's EEXIST behavior).
func PrepareIsolatedClaudeHome(claudeHome string) error {
	if claudeHome == "" {
		return fmt.Errorf("claudeHome is required")
	}
	sandboxClaude := filepath.Join(claudeHome, ".claude")
	if err := os.MkdirAll(sandboxClaude, 0o700); err != nil {
		return fmt.Errorf("mkdir sandbox .claude: %w", err)
	}
	realHome, err := os.UserHomeDir()
	if err != nil {
		// No operator home — skip credential symlink (eval will need
		// ANTHROPIC_API_KEY in env to auth).
		return nil
	}
	realCreds := filepath.Join(realHome, ".claude", ".credentials.json")
	if _, err := os.Stat(realCreds); err != nil {
		// Operator has no OAuth file (e.g. uses ANTHROPIC_API_KEY directly).
		// Eval will need that env var; nothing to symlink.
		return nil
	}
	sandboxCreds := filepath.Join(sandboxClaude, ".credentials.json")
	if _, err := os.Lstat(sandboxCreds); err == nil {
		// Already exists (idempotent re-run) — leave alone.
		return nil
	}
	if err := os.Symlink(realCreds, sandboxCreds); err != nil {
		return fmt.Errorf("symlink credentials: %w", err)
	}
	return nil
}
