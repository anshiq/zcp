# Codebase-content phase — parallel sub-agent dispatch per codebase

After scaffold + feature complete, every codebase gets two sub-agents
dispatched in parallel:

1. **`codebase-content`** — Zerops-aware. Authors `codebase/<h>/intro`,
   `codebase/<h>/integration-guide/<n>` (slotted; engine pre-stamps
   n=1, agent authors n=2 through 5),
   `codebase/<h>/knowledge-base`, and the whole commented zerops.yaml
   as one fragment `codebase/<h>/zerops-yaml`. Reads the recorded fact
   stream (porter_change + field_rationale + tier_decision) plus on-
   disk source / zerops.yaml / spec.

2. **`claudemd-author`** — Zerops-free. Authors only
   `codebase/<h>/claude-md` (single slot). Brief is strictly platform-
   free; agent reads package.json / src/* directly and produces
   `/init`-style output. Does NOT read facts; does NOT see Zerops
   integration content; sibling sub-agent owns IG/KB/yaml comments.

## Dispatch shape — main agent's responsibility

For each codebase, the main agent calls `build-subagent-prompt` TWICE
(once for `briefKind=codebase-content`, once for
`briefKind=claudemd-author`), then issues all 2N briefs in a single
message with parallel `Agent` tool calls. For each response, branch on
the inline-or-pointer contract (next section): pass `response.prompt`
byte-identical when set; otherwise wrap `response.briefPath` in a thin
"Read this file first" dispatch.

```
[message]
  Agent(description: "codebase-content-api", prompt: <inline body or "Read <briefPath>" wrapper>)
  Agent(description: "claudemd-author-api",  prompt: <inline body or "Read <briefPath>" wrapper>)
  Agent(description: "codebase-content-app", prompt: <inline body or "Read <briefPath>" wrapper>)
  Agent(description: "claudemd-author-app",  prompt: <inline body or "Read <briefPath>" wrapper>)
  ...
```

Net savings vs serial: 5-15 minutes for 3-codebase dispatches.

## Dispatch contract — pass response.prompt verbatim

Two correct dispatch shapes. Pick by main-agent context budget:

- **Inline**: pass `response.prompt` from `build-subagent-prompt`
  byte-identically as the `Agent` prompt parameter (only when
  `response.prompt` is non-empty — see inline-or-pointer rule below).
- **Self-fetch wrapper**: when context is tight, send the sub-agent a
  one-sentence context cue plus the
  `zerops_recipe action=build-subagent-prompt slug=<slug>
  briefKind=codebase-content codebase=<host>` invocation so it fetches
  the prompt itself.

## Dispatch — inline-or-pointer

`build-subagent-prompt` returns ONE OF two response shapes per call:

- **Inline** (body ≤ 40 KB) — `response.prompt` is the full composed
  brief; dispatch with `prompt=<response.prompt>` byte-identical.
- **Pointer** (body > 40 KB) — `response.prompt` is empty;
  `response.briefPath` is the absolute path to the engine-persisted
  brief on disk under `<outputRoot>/.briefs/`. `response.briefSize`
  carries the byte count for sanity-check. Dispatch with a thin
  wrapper telling the sub-agent to `Read <briefPath>` first thing,
  then proceed.

Branch on `briefPath != ""`. The two shapes are mutually exclusive —
the engine never populates both. Below the threshold the inline path
keeps run-13 §B2's byte-identical dispatch; above, disk-fallback
closes the cap-treadmill (run-29 Fix #1) by making disk-write the
designed primary path for large briefs rather than the error-recovery
fallback.

Hand-typed paraphrase wrappers — out. Re-stating the brief in your
own words compounds math errors and path drift (run-13 §B2) and at
codebase-content phase historically dropped run-specific findings
(run-26 F-31). The brief carries cross-codebase managed-service
facts so connection-shape decisions stay consistent across codebases —
the engine surfaces a sister codebase's finding (e.g. worker scaffold
recording a NATS auth-shape crash) into this codebase's brief when
both consume the same managed service.

## Why two sub-agents

Mixing CLAUDE.md authoring into the codebase-content brief leaks Zerops
context into CLAUDE.md (run-15 R-15-4: `## Zerops service facts` /
`## Zerops dev (hybrid)` headings appeared because the brief was
Zerops-aware). The sibling Zerops-free brief makes bleed-through
structurally impossible — there is no platform principles atom, no
`zerops.yaml` pointer, no managed-service hints in the
`claudemd-author` brief.

## Engine-emitted facts the codebase-content sub-agent fills

The brief includes engine-emitted shells (§7.1-§7.2):
- Class B universal-for-role: `<host>-bind-and-trust-proxy`,
  `<host>-sigterm-drain`, `<host>-no-http-surface` (worker)
- Class C umbrella: `<host>-own-key-aliases`
- Per-managed-service shells: `<host>-connect-<svc>`

For every shell with empty Why (per-managed-service shells, worker no-
HTTP heading), the agent calls `zerops_knowledge runtime=<svc-type>`
and fills via `fill-fact-slot factTopic=<topic> why=... heading=...`.

## Common record-fragment rejections — pre-empt these

The validator catches many drift classes; these three are the
**most-frequent** rejection patterns observed across recent runs and
account for the bulk of record-fragment iteration. Author with these
in mind from the start. This is NOT an exhaustive list — `docs/spec-
content-surfaces.md` is the surface contract and lists the full
validator set; treat the three below as a head-start, not a
sufficient checklist.

1. **KB stem must be symptom-first or directive-tightly-mapped-to-
   observable.** WRONG: `Re-fire seeds without re-running migrations`
   (author-claim). RIGHT: `Seed silently skipped after a partial-
   failure redeploy` (symptom-first — names what the porter actually
   sees).
2. **Slug citations are inline prose, never noun-phrase.** WRONG:
   ``the `env-var-model` guide covers ...`` (backticked slug as
   noun). RIGHT: `the env-var-model guide on Zerops docs covers ...`
   (inline prose; slug is referenced, not named as the subject).
3. **Classification × surface refusal**: `intersection` is KB-only,
   never IG. If a fact records `candidateClass=intersection`, route
   the body to KB; for IG, restate the principle without the
   intersection class.

## Complete-phase gate

Every codebase declared in `plan.codebases` must have all five
fragment ids recorded (intro + ≥1 integration-guide slot + knowledge-
base + zerops-yaml whole-yaml + claude-md). Codebase-scoped
validators run.
