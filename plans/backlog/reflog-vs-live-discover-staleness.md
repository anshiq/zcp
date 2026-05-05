---
Surfaced: 2026-05-05 — flow-eval scenario `develop-add-managed-dep-to-existing` (suite 20260505-065502) self-review.md: "the most important thing to internalize: the conversation context can claim infrastructure exists when it doesn't."
Why deferred: existing CLAUDE.md disclaimer ("verify current state via zerops_discover") saved this run; no agent has acted on stale REFLOG to date. Mitigation is working but the trap is real.
Trigger to promote: a flow-eval scenario where an agent acts on stale REFLOG state (skips bootstrap or deploys against an env var that doesn't resolve) without the disclaimer rescuing it.
---

# REFLOG entries in CLAUDE.md persist across cleanup and contradict live state

## Problem

`internal/eval/cleanup.go::CleanupProject` regenerates `CLAUDE.md` via
`init.Run` between scenarios. Per `internal/init/init.go::generateCLAUDEMD`,
the regeneration only replaces the `<!-- ZCP:BEGIN/END -->` marker block;
content **after** the markers — including `## REFLOG` entries appended by
the bootstrap workflow — is preserved.

Result: a scenario can complete bootstrap → REFLOG entry written → cleanup
deletes services and refreshes CLAUDE.md → next scenario starts with a
REFLOG that claims infrastructure exists but `zerops_discover` shows none
of it.

The agent in `develop-add-managed-dep-to-existing` saw a historical
REFLOG entry dated today with the exact same intent and a session ID,
listing `cache (valkey@7.2)` as already provisioned. Live discovery
contradicted it. The agent recovered because the embedded
"verify current state via zerops_discover" disclaimer is loud, but:

> "if you trusted the historical note you'd skip bootstrap and try to
>  deploy against an env var that doesn't resolve. … the conversation
>  context can claim infrastructure exists when it doesn't."

## Why this happens

Trade-off baked into `init.go` ~2026-04: REFLOG is user-facing reflection
that survives re-init by design (so users get a continuous log across
sessions). For real users this is correct. For eval-zcp where every
scenario starts from a clean project, REFLOG should NOT survive the
inter-scenario CleanupProject.

## Sketch

Two viable approaches:

1. **Eval-only**: `CleanupProject` strips the REFLOG section as part of
   the workdir clean step (between current step 2 and step 3, after
   unmount but before `cleanWorkDir`). Production `init.Run` behavior
   unchanged. Lowest blast radius. Implementation: regex-strip
   `\n## REFLOG\n[\s\S]*` from CLAUDE.md OR remove CLAUDE.md entirely
   (cleanWorkDir already handles non-protected files; just unmark
   CLAUDE.md as protected — but it's not in protectedPaths today, so
   it's ALREADY removed by cleanWorkDir on each scenario per the
   comment in `cleanup.go:25-28`. This means the REFLOG comes back in
   on **the second scenario after** a REFLOG-writing scenario somehow.
   Investigate timing.

2. **Production**: bootstrap's REFLOG-write timestamp + service-state
   snapshot, and a post-bootstrap "if any service in REFLOG no longer
   exists per discover, prepend a stale-marker to that REFLOG entry".
   Bigger surface, ships staleness signaling to real users too.

(1) is cheap and right for the eval surface that surfaced this. (2) is
the broader fix.

## Verify before acting

The cleanup.go comment claims `protectedPaths` excludes CLAUDE.md, so
cleanWorkDir already removes it. Yet the agent saw REFLOG persist.
Either:
- `init.Run` re-creates CLAUDE.md and copies a REFLOG from somewhere
  else (templates? a side file?). Investigate `internal/content/templates/`.
- Or the REFLOG lives in `.claude/` which IS protected. Investigate.

Without confirming the actual persistence path, any fix is speculative.

## Risks

- Stripping REFLOG from production `init.Run` would be a regression
  for real users who rely on it.
- Eval-only strip might mask the production-side staleness risk —
  user-facing fix should follow if the trap repeats outside eval.
