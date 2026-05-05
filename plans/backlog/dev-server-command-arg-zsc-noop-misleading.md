---
Surfaced: 2026-05-05 — flow-eval scenario `classic-go-simple` (suite 20260505-064504) self-review.md, top finding: "actively misleading guidance"
Why deferred: orthogonal to the bootstrap/verify atom edits in plans/archive/atom-edits-bootstrap-verify-2026-05-04.md; needs scope alignment with whichever atom or tool description owns the dev-server command argument doc.
Trigger to promote: another flow-eval scenario surfaces the same friction, OR a dev-server tool revision lands and we want to fix in one pass.
---

# `zerops_dev_server` `command` arg doc misleads on dev-mode dynamic runtimes

## Problem

The `zerops_dev_server` tool's `command` argument doc says (paraphrased):

> `command` — exact `run.start` from `zerops.yaml`.

For dev-mode dynamic-runtime services, `run.start` is `zsc noop --silent`
(per the `develop-checklist-dev-mode` atom — Zerops keeps the runtime
container idle so the agent can drive the dev process out-of-band). Passing
`zsc noop --silent` as the dev-server `command` therefore does nothing.

The agent in `classic-go-simple` flagged this as the **single most actively
misleading guidance** they hit:

> "If you follow that literally you'd pass `zsc noop --silent` as the
>  dev-server command, which obviously starts nothing. The right thing is
>  to pass your actual app launch command (`./app` for the Go binary).
>  The two pieces of guidance contradict each other — somebody fresh
>  will read the dev-server doc and waste a round."

## Where the doc lives

Likely candidates (verify before acting):
- `internal/tools/dev_server.go` — tool description / param annotations
- `internal/content/atoms/develop-dynamic-runtime-start-container.md` —
  has `command — exact run.start from zerops.yaml.` per current head
- `internal/content/atoms/develop-checklist-dev-mode.md` — adjacent atom

`develop-dynamic-runtime-start-container.md` is the strongest candidate
since the contradiction is dev-server-tool-vs-dev-mode-checklist and that
atom describes the dev-server tool surface.

## Sketch

Atom-side fix wording:

```
- `command` — the actual app launch command (e.g. `./app`, `npm run dev`,
  `python -m uvicorn ...`). NOT `run.start` for dev-mode dynamic runtimes:
  there `run.start` is `zsc noop --silent` so the runtime stays idle and
  the dev-server tool drives the real process.
```

Plus pinned test in atoms_lint_axes.go: forbid the literal phrase
"exact `run.start` from `zerops.yaml`" without an immediately-following
exception clause naming `zsc noop --silent`.

## Risks

- Tool description might be the primary source the agent reads; atom
  edit alone may not surface the fix. Verify with a re-run of
  `classic-go-simple` after the change.
- Wording must not Axis-K-handler-behavior the dev-server tool ("the
  tool spawns the process") — keep observable framing.
