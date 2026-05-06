# Atom rendering — state axes vs. shrink, skepticism

**Surfaced**: 2026-05-06 — Codex deep-synthesis on four findings from pgbench
agent retro (`runs/2026-05-06` session, `make a simple PG benchmark tool`).
Codex named **Trunk A — chybějící stavové osy v atom delivery** as the root
cause behind findings #1 (develop-active firehose, 25 KB single-service /
43-47 KB two-pair, dangerously close to Claude Code's 32 KB stdio cap),
#2 (ToolSearch deferral), and #4 (`apiMeta` atom repetition 4×/session).
The proposed surgical fix replays `friction-root-causes.md §P3` (RC-C):
add a `ServiceMaturity = {first-run, edit-loop}` axis derived from
`WorkSession.Verifies[hostname]`, gate first-run-heavy atoms behind it.

**Why deferred**: skepticism about the proposed direction. Adding state
axes to the atom pipeline reads like a patch on a patch.

The architecture today already filters on a rich axis vector
(`internal/workflow/atom.go::AxisVector` — phases, modes, environments,
closeDeployModes, gitPushStates, buildIntegrations, runtimes, routes,
steps, idleScenarios, deployStates, envelopeDeployStates,
serviceStatuses, exportStatuses, multiService). Each axis added comes
with: new frontmatter contract, new lint enforcement, new corpus
coverage gate, new authoring discipline, new place for drift, new
test fixture matrix expansion. RC-C's `maturity` is one new axis;
finding #4 implies a sibling `errorCondition` / `triggers` axis;
the open `develop-response-atom-proliferation.md` backlog item
suggests a `dependencies:` axis and a `scopeSize:` axis. Four new
axes to make the corpus bearable is a tax we keep paying without
asking whether the corpus shape itself is wrong.

## The unresolved question

**Is "filter the corpus more aggressively" the correct trunk fix, or
does the corpus need to be smaller / structured differently in the first
place?**

Concrete alternatives that ARE NOT "more axes":

1. **Aggressive prose surgery.** 52 develop-active atoms total ~1742
   lines. Most are 1-2 KB; many could be 200-500 bytes if rewritten as
   trigger + action + failure-mode (per CLAUDE.local.md "LLM-facing
   prose" rule). Pure content edit. No architecture change. Worth
   measuring before adding axes.

2. **Move bulk to pull-on-demand.** `internal/tools/knowledge.go` could
   take a `mode=5 key=<atom-id>` (already drafted in
   `friction-root-causes.md §P3 step 6` for `develop-verify-matrix`).
   Phase response becomes status + plan + a list of pointer titles
   (each ~50 bytes); agent fetches the body it actually needs. Inverts
   the model from push-everything to pull-on-need.

3. **Trust the agent's context window across the session.** Today every
   `action="status"` re-renders the full atom set against the same
   envelope. The agent already saw it on develop-start; re-rendering
   pays the cost again with no new information. A single "render-once
   per session-phase, terse afterward" rule would resolve #1 and #4
   without any new axis — but requires session memory the server
   currently rejects (KD-01 "stateless STDIO"). The interesting
   question: does KD-01 mean "no session memory in the server" or "no
   state mutation in the server"? The latter allows derivation from
   `WorkSession` which is already persisted; the former forbids it.

4. **Demote cross-cutting reference content out of phase atoms.**
   Things like the `apiMeta` shape, deployFiles geometry, env-var
   channel rules — these aren't phase-specific. They could live in
   one `claude.md`-style static reference fetched once at session
   start, never rendered into phase responses. Moves the firehose
   off the hot path entirely.

5. **Hard cap phase response size.** Force authors to make
   prioritization choices. 8 KB cap on `develop-active` response
   would make the current 25 KB shape impossible — authors would
   have to cut, not filter. Aligns with the 32 KB MCP wire cap as
   load-bearing constraint, not aspirational target.

Each of these is plausibly cheaper and cleaner than RC-C's
`ServiceMaturity`. None has been measured or compared.

## The pattern that worries me

The corpus has been expanding without a corresponding shrink discipline.
Each eval cycle surfaces a new "X is too noisy in scenario Y" → response
is "add axis Z to filter X out of Y" → axis vector grows → corpus authoring
contract gets stricter → next cycle surfaces a new gap. Filter axes
are the easy answer because they preserve every atom; the harder
question is whether half the corpus belongs there at all.

`plans/backlog/develop-response-atom-proliferation.md` already named
this same pattern in 2026-05-03 ("Real root cause: 14+ close-mode
atoms covering every (mode × close-mode × environment × strategy)
combination. Each is small, but they pile up.") and listed four
fix shapes (a-d) escalating in cost — option (a) was "audit +
consolidate close-mode atoms — cheapest." That option got listed
and then never run. If we had run it, we would know whether the
firehose is a corpus-size problem or a filter-precision problem.
We still don't.

## What I'd want before promoting

Before committing to ship `ServiceMaturity` (RC-C) or any sibling axis:

1. **Run option (a) from `develop-response-atom-proliferation.md`**:
   audit + consolidate the close-mode atom cluster (14+ atoms). Measure
   develop-active wire-frame before/after. If the corpus shrinks 30 %+
   on this alone, axis additions are premature.

2. **Run a prose-surgery pass** on the top 5 service-agnostic atoms
   (`develop-platform-rules-common`, `develop-verify-matrix`,
   `develop-env-var-channels`, `develop-knowledge-pointers`,
   `develop-platform-rules-container`). Apply the LLM-prose contract
   from CLAUDE.local.md. Measure size delta.

3. **Spike a pull-on-demand variant**: extract `develop-verify-matrix`
   (3 399 B reference manual) into `zerops_knowledge` mode 5; replace
   in-line render with a 50-byte pointer atom. Measure first-deploy
   brief size + count agent calls to fetch it back. If agents fetch
   it ≤30 % of the time, push-everything was the wrong default.

4. **Articulate KD-01 boundaries explicitly**. "Stateless STDIO" is
   pinned by `TestNoCrossCallHandlerState`. Does that pin "no
   package-level vars" (load-bearing) or "no envelope derivation
   from WorkSession" (incidentally narrow)? If the latter, RC-C is
   permitted; if reading the spec gives the former, RC-C is fine
   but the boundary needs to be sharpened in `spec-knowledge-distribution.md`.

If after (1)-(3) the corpus is already under 14 KB on every fixture,
RC-C ships into a much smaller surface and the axis tax is paid
against a much smaller corpus. Different tradeoff.

If after (1)-(3) the corpus is still ~20 KB, the answer to "is the
problem corpus size or filter precision?" is "filter precision," and
RC-C is justified — at that point we know we're paying axis cost
deliberately, not by default.

## Trigger to promote

Promote (i.e., extract into a real plan) when EITHER:

- **(1)-(3) above have been run and measured**, and the data says axis
  additions are necessary (not just convenient).
- **Or** another eval session in 2026-05 hits the 32 KB MCP wire-cap
  spillover for real (current session is 25 KB — close but not yet),
  AND the cheapest shrink option is shown to be insufficient.

## Trigger to reject

Reject (move to `rejected/`) if:

- Corpus shrink + pull-on-demand together get the develop-active
  wire-frame under 12 KB on the worst measured fixture, AND the four
  findings from 2026-05-06 disappear from subsequent retros without
  any axis addition. At that point the trunk diagnosis was wrong;
  the real root was corpus discipline, not filter expressivity.

## Refs

- Codex unified review (2026-05-06 conversation, parent `aba837ec620310b3b`)
- `plans/friction-root-causes.md §P3 / RC-C` — the unshipped design this would replay
- `plans/backlog/develop-response-atom-proliferation.md` — sibling backlog item
  with four fix-shapes (a-d); option (a) is the cheapest-shrink route
- `plans/atom-corpus-context-trim-2026-04-26.md §17` — empirical wire-frame numbers,
  32 KB MCP cap context
- `plans/flow-eval-followup-fixes-2026-05-03.md §A` — drafted-but-unshipped
  preload-hint atoms (covers finding #2; orthogonal to this question)
- `plans/backlog/next-actions-log-error-after-success-deploy.md` — finding #3
  (Codex Trunk B; orthogonal to this question)
- `internal/workflow/atom.go::AxisVector` — current axis vector
- `internal/workflow/envelope.go::WorkSessionSummary` lines 116-124 — already
  carries `Verifies` history that RC-C would derive maturity from
- `docs/spec-knowledge-distribution.md §10 KD-01` — statelessness invariant
  whose interpretation gates RC-C admissibility

## Open questions for whoever picks this up

1. Which option from `develop-response-atom-proliferation.md` (a/b/c/d)
   was meant to run first? It hasn't. Why?
2. Is there a measurement of how often agents call `action="status"` mid-session
   in real eval runs? If status is rarely re-called, the "every status re-renders"
   complaint is overstated; if it's called every 3-5 turns post-compaction,
   it dominates token cost.
3. The agent's pgbench retro estimated "12 KB" for develop-active; reality is
   25 KB. The agent under-estimated. Does this mean the firehose is felt LESS
   than the byte count suggests? Worth asking real eval retros what fraction
   of the atom block they read vs. skim.
