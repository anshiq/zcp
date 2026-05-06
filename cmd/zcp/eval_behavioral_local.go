package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zeropsio/zcp/internal/eval"
)

// runBehavioralRunLocal executes a behavioral scenario in local mode:
// agent runs on this Mac (not over SSH to a Zerops container). The
// flow mirrors the existing `run` subcommand but with isolated workdir,
// HOME, and sentinel-gated cleanup so the operator's real source repo,
// $HOME, and ~/.claude/ stay untouched.
//
//	zcp eval behavioral run-local --id <id>
//	zcp eval behavioral run-local --id <id> --scenarios-dir <dir>
//	zcp eval behavioral run-local --id <id> --cleanup-workdir
//
// Sequence:
//  1. Prereqs: ZCP_API_KEY env, `zcp` in PATH (per `make install`).
//  2. Resolve scenario file.
//  3. Compute paths under /tmp/zcp-flow-eval-local/<suite>/<id>/, with
//     results landing back in <repo>/eval/behavioral/runs-local/.
//  4. mkdir workdir + claude-home + results, write sentinel.
//  5. Symlink ~/.claude/.credentials.json into sandbox claude-home so
//     OAuth keeps working without ANTHROPIC_API_KEY.
//  6. Set ZCP_EVAL_* env vars; unset serviceId defensively.
//  7. Delegate to existing initEvalRunner() + RunBehavioralScenario().
//  8. Print result paths; optionally rm -rf the scratch dir on success.
func runBehavioralRunLocal(args []string) {
	id := flagValue(args, "--id")
	scenariosDir := flagValue(args, "--scenarios-dir")
	cleanupWorkdir := hasFlag(args, "--cleanup-workdir")

	if id == "" {
		fmt.Fprintln(os.Stderr, "error: --id <scenario-id> required")
		os.Exit(1)
	}

	if err := eval.AssertLocalRunPrereqs(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getwd: %v\n", err)
		os.Exit(1)
	}

	suiteID := eval.NewSuiteID()
	paths := eval.ComputeLocalRunPaths(suiteID, id, repoRoot)
	if scenariosDir != "" {
		paths.ScenarioDir = scenariosDir
	}

	scenarioPath := filepath.Join(paths.ScenarioDir, id+".md")
	if _, err := os.Stat(scenarioPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: scenario file: %v\n", err)
		os.Exit(1)
	}

	if err := eval.PrepareLocalRunDirs(paths); err != nil {
		fmt.Fprintf(os.Stderr, "error: prepare dirs: %v\n", err)
		os.Exit(1)
	}
	if err := eval.PrepareIsolatedClaudeHome(paths.ClaudeHome); err != nil {
		fmt.Fprintf(os.Stderr, "error: prepare claude home: %v\n", err)
		os.Exit(1)
	}

	// Env handoff to initEvalRunner (which reads these), the Runner
	// (which embeds them in RunnerConfig), and the spawned `claude`
	// process (which inherits via cmd.Env, see Runner.claudeEnv).
	envOverrides := map[string]string{
		"ZCP_EVAL_WORK_DIR":      paths.WorkDir,
		"ZCP_EVAL_CLAUDE_HOME":   paths.ClaudeHome,
		"ZCP_EVAL_RESULTS_DIR":   paths.ResultsDir,
		"ZCP_EVAL_SENTINEL_FILE": eval.SentinelFilename,
	}
	for k, v := range envOverrides {
		_ = os.Setenv(k, v)
	}
	// serviceId in the parent env would flip runtime.Detect() to
	// container mode and re-register the SSH deploy schema — defend
	// against operator shell rc leaks.
	_ = os.Unsetenv("serviceId")

	fmt.Fprintf(os.Stderr, "Running local-mode behavioral scenario: %s (suite=%s)\n", scenarioPath, suiteID)
	fmt.Fprintf(os.Stderr, "  workdir:     %s\n", paths.WorkDir)
	fmt.Fprintf(os.Stderr, "  claude home: %s\n", paths.ClaudeHome)
	fmt.Fprintf(os.Stderr, "  results:     %s/%s/%s/\n", paths.ResultsDir, suiteID, id)

	runner, _, ctx := initEvalRunner()
	result, err := runner.RunBehavioralScenario(ctx, scenarioPath, suiteID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printBehavioralResult(result)

	scenarioRoot := filepath.Dir(paths.WorkDir)
	if cleanupWorkdir && result.Error == "" {
		if err := os.RemoveAll(scenarioRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cleanup workdir %s: %v\n", scenarioRoot, err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Scratch dir kept at %s (--cleanup-workdir to remove)\n", scenarioRoot)
	}

	if result.Error != "" {
		os.Exit(1)
	}
}

// hasFlag reports whether name appears as a bare flag in args (no value).
// Used for boolean toggles that don't need a separate value.
func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}
