// Tests for emit's auto-copy of <run>/<host>dev/CLAUDE.md into
// fragments-new/<host>/codebase__<host>__claude-md.md. claudemd-author
// is policy-skipped in sim (run-23 §sim-scope); the source run's
// CLAUDE.md is reused verbatim as if a sub-agent had authored the
// fragment.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmit_CopiesClaudeMdFromRun asserts the auto-copy path: given a
// run dir with <host>dev/CLAUDE.md present, after `runEmit` the file
// appears at <out>/fragments-new/<host>/codebase__<host>__claude-md.md.
func TestEmit_CopiesClaudeMdFromRun(t *testing.T) {
	runDir := t.TempDir()
	simDir := t.TempDir()
	if err := writeMinimalRunDir(t, runDir); err != nil {
		t.Fatalf("writeMinimalRunDir: %v", err)
	}
	const claudeBody = `# api codebase CLAUDE.md from source run

## Architecture

The api codebase is a NestJS app that talks to Postgres.

## Build & run

- pnpm install
- pnpm build
- pnpm start
`
	mustWrite(t, filepath.Join(runDir, "apidev", "CLAUDE.md"), claudeBody)

	if err := runEmit([]string{"-run", runDir, "-out", simDir}); err != nil {
		t.Fatalf("runEmit: %v", err)
	}

	dst := filepath.Join(simDir, "fragments-new", "api", "codebase__api__claude-md.md")
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("expected staged claude-md fragment at %s; %v", dst, err)
	}
	if string(body) != claudeBody {
		t.Errorf("staged claude-md body mismatch\nwant: %q\n got: %q", claudeBody, string(body))
	}
}

// TestEmit_SkipsMissingClaudeMd asserts that when the source run dir
// has no CLAUDE.md, runEmit logs a warning to stderr but does NOT
// fail. Stitch will fall back to engine placeholder downstream.
func TestEmit_SkipsMissingClaudeMd(t *testing.T) {
	runDir := t.TempDir()
	simDir := t.TempDir()
	if err := writeMinimalRunDir(t, runDir); err != nil {
		t.Fatalf("writeMinimalRunDir: %v", err)
	}
	// Sanity: no CLAUDE.md in fixture.
	if _, err := os.Stat(filepath.Join(runDir, "apidev", "CLAUDE.md")); err == nil {
		t.Fatalf("fixture invariant: writeMinimalRunDir should not stage CLAUDE.md")
	}

	if err := runEmit([]string{"-run", runDir, "-out", simDir}); err != nil {
		t.Fatalf("runEmit must not fail when CLAUDE.md is missing: %v", err)
	}

	// fragments-new/api/ exists but the claude-md fragment does NOT.
	dst := filepath.Join(simDir, "fragments-new", "api", "codebase__api__claude-md.md")
	if _, err := os.Stat(dst); err == nil {
		t.Errorf("claude-md fragment should not be staged when source missing; %s exists", dst)
	}
	// fragments-new/api/ itself should still exist (created upfront for
	// the codebase-content sub-agent's writes).
	if _, err := os.Stat(filepath.Join(simDir, "fragments-new", "api")); err != nil {
		t.Errorf("fragments-new/api/ not created: %v", err)
	}
}

// TestEmit_CopiesClaudeMdMultiCodebase asserts the copy fires per
// codebase (not just the first). Pinned because the loop body is
// shared with stageCodebaseArtifacts and a future refactor could
// accidentally hoist a hostname literal.
func TestEmit_CopiesClaudeMdMultiCodebase(t *testing.T) {
	runDir := t.TempDir()
	simDir := t.TempDir()

	// Build a 2-codebase run dir manually (writeMinimalRunDir is
	// single-codebase). plan.json carries api + worker; both have
	// minimal yaml + CLAUDE.md.
	for _, host := range []string{"api", "worker"} {
		hostDir := filepath.Join(runDir, host+"dev")
		mustWrite(t, filepath.Join(hostDir, "zerops.yaml"), minimalYAML)
		mustWrite(t, filepath.Join(hostDir, "package.json"), `{"name":"`+host+`"}`)
		mustWrite(t, filepath.Join(hostDir, "src", "main.ts"), "// "+host+"\n")
		mustWrite(t, filepath.Join(hostDir, "CLAUDE.md"),
			"# "+host+" codebase\n\n## Architecture\n\nNestJS-style "+host+" entry that runs a small server backed by Postgres so dispatched sub-agents have a concrete read surface.\n\n## Build & run\n\n- pnpm install\n- pnpm build\n- pnpm start\n")
	}
	// Plan with two codebases.
	plan := minimalPlan(filepath.Join(runDir, "apidev"))
	plan.Codebases = append(plan.Codebases, plan.Codebases[0])
	plan.Codebases[1].Hostname = "worker"
	plan.Codebases[1].Role = "worker"
	plan.Codebases[1].SourceRoot = filepath.Join(runDir, "workerdev")
	plan.Codebases[1].IsWorker = true

	envDir := filepath.Join(runDir, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir env: %v", err)
	}
	body, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(envDir, "plan.json"), body, 0o600); err != nil {
		t.Fatalf("write plan.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "facts.jsonl"), nil, 0o600); err != nil {
		t.Fatalf("write facts.jsonl: %v", err)
	}

	if err := runEmit([]string{"-run", runDir, "-out", simDir}); err != nil {
		t.Fatalf("runEmit 2-codebase: %v", err)
	}

	for _, host := range []string{"api", "worker"} {
		dst := filepath.Join(simDir, "fragments-new", host, "codebase__"+host+"__claude-md.md")
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Errorf("expected staged claude-md for %s at %s: %v", host, dst, err)
			continue
		}
		if !strings.Contains(string(got), "# "+host+" codebase") {
			t.Errorf("staged claude-md for %s missing host marker; got:\n%s", host, string(got))
		}
	}
}
