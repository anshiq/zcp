---
slug: local-eval-home-isolation-macos-keychain
opened: 2026-05-06
trigger-to-promote: when a second local-mode behavioral scenario fails because operator settings/slash-commands/memory contaminate eval signal, OR when we want to add `--bare`-shaped isolation as a configurable mode
---

# Local-mode flow-eval HOME isolation broken on macOS

`cmd/zcp/eval_behavioral_local.go::runBehavioralRunLocal` accepts an
`--isolate-claude-home` flag that creates a sandbox HOME for the spawned
`claude` process so the operator's real `~/.claude/` (auto-memory,
sessions, settings, slash commands, plugins) doesn't contaminate the
eval signal. Default is **off** because the implementation doesn't
authenticate on macOS.

## Symptoms

Two consecutive `zcp eval behavioral run-local` invocations with
isolation on (suites `20260506-122155` + `20260506-122501`) failed
fast with `apiKeySource: "none"` and assistant text "Not logged in ·
Please run /login" — claude headless rejected before the scenario
could even start.

## What we tried

1. **Symlink `~/.claude/.credentials.json` → sandbox** — that file
   doesn't exist on macOS at all (only `~/.claude.json` in HOME root,
   plus the macOS Keychain entry `Claude Code-credentials`).
2. **Copy `~/.claude.json` snapshot → sandbox `~/.claude.json`** —
   sandbox got the 158KB file, but claude still reported `apiKeySource:
   "none"`. The actual OAuth bearer token lives in the Keychain
   (verified via `security find-generic-password -s
   "Claude Code-credentials"`); claude's HOME-based config file holds
   only metadata + caches.

## Hypothesis

On macOS the Keychain entry is likely keyed in a way that requires
either the operator's own HOME or a HOME-paired session-state in
`~/.claude.json`. Our HOME override breaks that pairing. The
`--bare` flag's escape hatch (set ANTHROPIC_API_KEY, skip OAuth +
keychain entirely) confirms there are TWO auth paths and only one
survives HOME isolation.

## Open questions

- Does the Keychain entry's `acct` field (`macbook`) bind to the
  operator's HOME or just their UID? If just UID, what other
  HOME-paired state is claude reading at startup that breaks the auth
  resolution?
- Would copying additional files (`~/.claude/sessions/`, the rest of
  `~/.claude/`) into the sandbox restore the binding?
- Is the cleanest path actually to support `--bare + ANTHROPIC_API_KEY`
  as the official isolated mode (operator provisions an API key
  alongside their OAuth subscription specifically for evals)?

## Why this matters

Without isolation, the eval reads operator's settings.json, plugins,
slash-commands, and CLAUDE.md auto-discovery. That contaminates
behavioral signal — the agent's behavior reflects operator
customizations, not a clean baseline. Per-eval session+memory writes
also accumulate in the operator's `~/.claude/projects/` (under a
`/tmp/...workdir`-shaped key, so the operator's main project memory
isn't touched, but it's still cruft).

For the **first** scenario this is acceptable. For systematic
evaluation across N scenarios, it's not.

## Promote-to-plan trigger

Promote when EITHER:
- A second scenario shows clear contamination (e.g. agent invoking a
  user-installed slash command that doesn't exist in production)
- We commit to add `--bare` isolation mode and need to design the
  ANTHROPIC_API_KEY provisioning UX

## Files / commits relevant

- `cmd/zcp/eval_behavioral_local.go` — `--isolate-claude-home` flag wiring
- `internal/eval/local_setup.go::PrepareIsolatedClaudeHome` — current
  best-effort copy implementation (works structurally, fails functionally
  on macOS)
- `internal/eval/behavioral_run.go::Runner.claudeEnv` — HOME override
  via `cmd.Env = append(os.Environ(), "HOME=...")`
- Suite outputs: `eval/behavioral/runs-local/20260506-122155/` and
  `20260506-122501/` — auth-failure transcripts
