---
Surfaced: 2026-05-05 — flow-eval scenario `classic-go-simple` (suite 20260505-064504) self-review.md: "destruction warnings make `[.]` feel risky on first read; it isn't."
Why deferred: small atom-phrasing tweak; not blocking; one-scenario signal so far.
Trigger to promote: another flow-eval scenario flags the same friction, OR an agent picks a narrower deployFiles than `[.]` for a self-deploy and breaks (the trap DM-2 was originally written to prevent).
---

# DM-2 self-deploy `deployFiles: [.]` destruction warnings overcorrect

## Problem

Atom guidance around DM-2 (self-deploy invariant: source IS target,
narrower-than-`[.]` `deployFiles` destroys the target) leans heavily on
"destruction" language to discourage cherry-picking. From the
`classic-go-simple` retrospective:

> "(b) `deployFiles: [.]` is correct for a self-deploy even though the
>  guidance keeps warning you that narrower patterns destroy the target —
>  `[.]` is the safe value, full stop. The destruction warnings make
>  `[.]` feel risky on first read; it isn't."

The friction: agent reads "destroys the target" framing, second-guesses
the canonical `[.]` value, hesitates or experiments with narrower patterns
"to be safe" — the exact failure mode DM-2 is supposed to prevent.

## Where the friction lives

Likely candidates (verify before acting):
- `internal/content/atoms/develop-deploy-self-vs-cross.md` (or similar
  develop-active atom describing self-deploy vs cross-deploy)
- `develop-platform-rules-common.md`
- the `zerops_deploy` tool description for the `deployFiles` parameter

Look for phrases like "destroy the target" / "cherry-pick destroys" /
"hard-rejected" wrapping the `[.]` recommendation.

## Sketch

Reframe in two passes:

1. **Lead with the safe value, then the trap.** Today's atoms tend to
   structure as "trap → therefore use `[.]`". Flip to "use `[.]` for
   self-deploy. Narrower patterns destroy the source you're deploying
   from (DM-2)."

2. **Bound the destruction warning to cross-deploy contrast.** Self-deploy
   should be presented as "boring, always `[.]`"; cross-deploy is where
   `deployFiles` choice has interesting consequences (post-build-tree
   relative paths). Today they share warning language.

## Risks

- Tone-down too much and DM-2 stops triggering when an agent does
  pick a narrower pattern. Pinned test
  `TestValidateZeropsYml_DM2_*` is the actual safety; atom phrasing
  is the prevention layer. As long as the test stays, atom edit is safe.
- Agent in `classic-go-simple` happened to be a single-scenario signal.
  Validate via re-run after edit; if friction persists, escalate to
  splitting the self-vs-cross atom.
