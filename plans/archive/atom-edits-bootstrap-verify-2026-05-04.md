# Plan: Atom edits — bootstrap two-phase, plan nesting, dev-mode verify precondition

**Status**: Proposed.
**Surfaced**: 2026-05-04 — flow-eval suite `20260504-104436` (16 / 19 scenarios completed). Three friction classes appeared independently in 7 / 8 / 3 retrospectives respectively; all three classes are atom-content gaps where the underlying fact is documented but buried, mis-led, or absent at the routing site the agent reads first.

This plan is self-contained — no external references beyond `CLAUDE.md`, `CLAUDE.local.md`, `MEMORY.md`, `eval/behavioral/README.md`, `docs/spec-knowledge-distribution.md`, `docs/spec-scenarios.md`, and the source tree.

---

## How an LLM implementer should approach this plan

1. **Read top-to-bottom before starting Phase 1.** Each phase is a single atom edit; phases are independent and can be ordered freely, but the verification rituals are shared and described once in §"Cross-phase verification".
2. **TDD per `CLAUDE.md`.** Each phase has a RED check (what should fail before the edit), a GREEN check (after), and a slow-loop check (re-run a representative flow-eval scenario, compare retrospectives).
3. **Pause points are explicit.** End of each phase: commit, run `make lint-local`, run targeted `go test`. STOP if any fast check is red. Do not start the slow loop until the fast checks are clean.
4. **No new atoms. No routing changes. No handler-side edits. No schema changes.** Pure content edits to existing atoms.
5. **Atom hygiene rules apply** (`internal/content/atoms_lint*.go`). Axis K (handler-behavior verbs), Axis L (env-only title qualifiers — HARD-FORBID), Axis M (invisible-state field names), Axis N (plan-doc paths). When in doubt, run `go test ./internal/content -run TestAtomAuthoringLint` after each edit.

---

## Why now

Friction frequency in suite `20260504-104436`:

| Class | Scenarios surfacing it (out of 16 successful retrospectives) |
|---|---|
| Two-phase bootstrap-start (`kind: route-menu` vs `kind: session-active`) | 7 — classic-static-nginx, cross-deploy, develop-add-managed-dep, existing-standard, greenfield-node-postgres-dev-stage, recipe-laravel-minimal, verify-subdomain |
| Plan-nesting `runtime` object (flat placement of `bootstrapMode`/`stageHostname` rejected) | 8 — classic-go, classic-python, classic-rust, classic-static-nginx, develop-add-managed-dep, existing-standard, greenfield-node-postgres-dev-stage, verify-subdomain |
| Dev-mode dynamic-runtime first-deploy verify 502 (Trap-2 reprise) | 3 new + historical — classic-bun, classic-go, classic-rust |

These three classes are the highest-leverage edits in the suite. Each one is a single atom file. Combined they should drop friction notes in 13+ of the next suite's retrospectives (counts overlap; same scenario can hit two classes).

---

## What is NOT in scope

- Routing changes (which atom fires when). All three atoms already route to the right phase/route/step combinations; the friction is content, not routing.
- Schema or workflow handler edits (e.g. making `route` required on first call, or branching verify on mode at the handler level). Those are valid but separate plans.
- New atoms. Splitting `develop-first-deploy-verify` into mode-specific variants is a structural option for a later plan if Phase 3's content edit proves insufficient.
- Recipe-content (`internal/knowledge/recipes/*.md`) edits. Recipe-specific gotchas live there per `CLAUDE.md` "Recipe-specific findings go in recipes, not atoms".
- Render-probe golden refreshes that change atom IDs — only golden text refreshes that mechanically follow from the edit text are in scope.
- Updates to `MEMORY.md`. Memory tracks invariants; these atom edits don't introduce invariants, only surface existing ones better.

---

## Phase 1 — `bootstrap-route-options`: lead with two-phase pattern, name the `kind` field

### Current state

`internal/content/atoms/bootstrap-route-options.md` opens with a code example calling `start workflow=bootstrap` without `route`, then prose: *"BootstrapDiscoveryResponse returns priority-ordered routeOptions[]; no session is committed."* Then *"Pick one option, then call `start` again with its route plus required `recipeSlug` / `sessionId`."*

The two-phase nature **is** stated. The `kind` field is **not named anywhere in the atom**.

### Friction (verbatim retrospective excerpts)

- *"The two-phase bootstrap start is the thing most likely to read as an error if you're not paying attention. … Read the `kind` field — that's the contract."* (`classic-static-nginx-simple`)
- *"The bootstrap is two-phase and the first call doesn't open a session — it returns a `kind: \"route-menu\"` … If you don't notice the `kind` field on the first response and try to advance steps, you'll be confused."* (`verify-subdomain-recovery-before-browser`)
- *"The route-menu response did explain this clearly in its `message` field, but the schema for `start` doesn't make `route` look required, so an agent who skims past the response body and just looks at `kind` could easily assume the session is open."* (`develop-add-managed-dep-to-existing`)

The agents are reading the response correctly **after** they trip; the atom is what should pre-empt the trip.

### Edit goal

Lead with the two-call pattern and name the `kind` field on each call. Keep the existing route table — that part works. Order:

1. `### Bootstrap is two-phase` — three-line opener naming `kind: "route-menu"` (call 1) vs `kind: "session-active"` (call 2). Show both calls verbatim.
2. Existing route table.
3. Existing collision semantics + explicit-override sections, unchanged.

### RED check

Before the edit, run:

```
go test ./internal/content -run TestAtomAuthoringLint
go test ./internal/workflow -run TestScenario_S1_NewProjectRecipeMatch
go test ./internal/workflow -run TestScenario_S3_AdoptOnlyUnmanaged
```

All should be green (RED for this phase is the friction signal in self-reviews, not a code test). The friction signal is documented in §"Why now"; the slow-loop check below confirms it drops.

### GREEN check

After the edit:

```
go test ./internal/content/...
go test ./internal/workflow/...
make lint-local
```

`atoms_lint` must pass; the new opener must not introduce Axis K verbs (e.g. avoid handler-behavior phrasing like "the handler stamps", "auto-attaches"). Use observable-state phrasing only: "the response carries `kind: \"route-menu\"`". Render-probe goldens may need refresh if any pin embeds full atom body — refresh via the existing `-update` flag and visually inspect the diff.

### Slow-loop check

Re-run two scenarios that cited this friction prominently:

```
./eval/behavioral/flow-eval.sh classic-static-nginx-simple
./eval/behavioral/flow-eval.sh verify-subdomain-recovery-before-browser
```

Read each new `self-review.md`. The two-phase friction line should be **absent or downgraded** ("noticed `kind=route-menu` on the first call, called again with route — clean") versus the prior runs at `eval/behavioral/runs/20260504-104436/`.

If the friction persists, the edit didn't surface enough — extend the opener with a sentence reading "If you see `kind: \"route-menu\"`, the session is NOT open yet; call again with the chosen `route`." Do not bloat past 5 added lines.

---

## Phase 2 — Plan-nesting `runtime` example in recipe-route + classic-route discover atoms

### Current state

Two routes share the plan-submission step but only one of three relevant atoms shows the nested JSON shape:

- `bootstrap-recipe-match.md` (priority 1, recipe route, discover step) — describes plan via prose only: *"Per runtime pair: `devHostname`/`stageHostname` from recipe's `zeropsSetup: dev`/`prod` services; `type` + `bootstrapMode` verbatim..."*. **No JSON example.**
- `bootstrap-classic-plan-dynamic.md` (priority 2, classic route, discover step) — short prose only. **No JSON example.**
- `bootstrap-mode-prompt.md` (priority 3, classic route, discover step) — has the inline example `{"runtime": {"devHostname": "appdev", "type": "...", "bootstrapMode": "standard", "stageHostname": "appstage"}}`.

The example exists but lives at priority 3 on the **classic route only**. Recipe-route agents never see it. Classic-route agents see it but read priority 1-2 atoms first and may submit before scrolling to priority 3.

### Friction (verbatim retrospective excerpts)

- *"The plan submission for `complete step=discover` … there's an explicit warning that flattening `bootstrapMode`/`stageHostname` to top-level rejects with a hard error. That's good that the warning is there, but it suggests other agents are tripping on this regularly."* (`develop-add-managed-dep-to-existing`)
- *"The plan-submission shape … `bootstrapMode` and `stageHostname` MUST be inside the `runtime` object — flat placement is hard-rejected. I got it right on the first try but only because I read the parameter description carefully; a casual reading would skip past this."* (`greenfield-node-postgres-dev-stage`)
- *"The plan JSON shape during bootstrap discover took some squinting. The schema description has a good example for a dev/stage pair, but the recipe-specific guidance just says 'construct from the recipe's `zeropsSetup: dev`/`prod` services' without showing the full nested `runtime` + `dependencies` shape against the recipe's actual three services."* (`recipe-laravel-minimal-standard`)

Eight retrospectives surface this; the recipe-route ones are particularly clear that the full nested shape should be near the front, not derived from prose.

### Edit goal — Phase 2A: `bootstrap-recipe-match.md`

Insert a short worked-example block under `### Plan shape (no collisions)`, showing the full nested `runtime` + `dependencies` JSON for a dev/stage pair plus one managed dep. Mirror the shape `bootstrap-mode-prompt.md` already uses.

### Edit goal — Phase 2B: `bootstrap-classic-plan-dynamic.md`

Append the same worked-example block. Classic-route dynamic-runtime path should not depend on agents scrolling to priority 3 (`bootstrap-mode-prompt`) to find the JSON shape.

### RED check (Phase 2)

```
go test ./internal/content -run TestAtomAuthoringLint
go test ./internal/workflow -run TestScenario_S1_NewProjectRecipeMatch
go test ./internal/workflow -run TestScenario_S10_RecipeActive
```

### GREEN check

After both edits:

```
go test ./internal/content/...
go test ./internal/workflow/...
make lint-local
```

Watch for Axis K (no handler-behavior verbs in the example annotation) and Axis M (no invisible-state field names — only fields that exist on the user-facing schema).

### Slow-loop check

```
./eval/behavioral/flow-eval.sh recipe-laravel-minimal-standard
./eval/behavioral/flow-eval.sh classic-rust-postgres-standard
```

Compare each new `self-review.md` against the prior run. Plan-nesting friction should be absent or strictly downgraded ("submitted nested shape on first try, no rejection").

---

## Phase 3 — `develop-first-deploy-verify`: dev-mode dynamic-runtime 502 precondition

### Current state

`internal/content/atoms/develop-first-deploy-verify.md` opens with the verify call and goes straight to "If unhealthy" troubleshooting (localhost binding, run.start mis-config, etc). It does **not** flag the dev-mode dynamic-runtime 502 case as a precondition. The atom routes for `runtimes: [dynamic, implicit-webserver]` and `deployStates: [never-deployed]` — fires on every first deploy of a dynamic runtime regardless of mode. Adjacent atom `develop-checklist-dev-mode.md` carries the relevant fact (*"Zerops keeps the runtime container idle; you start the dev process yourself via `zerops_dev_server action=start` after each deploy"*) but it's a separate routing slice (`modes: [dev]`, `environments: [container]`).

### Friction

This is the original Trap-2 from the POC scenario coverage. It surfaced in three new scenarios in suite `20260504-104436`:

- *"After the first `zerops_deploy targetService=\"appdev\"` came back `status: DEPLOYED, subdomainUrl: ...`, my next instinct was to verify. Verify came back `status: degraded, http_root: HTTP 502`. That looks like a broken deploy on a casual read, but it isn't — the dev-mode setup intentionally starts with `zsc noop --silent` so you can run `cargo run` yourself via `zerops_dev_server`."* (`classic-rust-postgres-standard`)
- *"There's a window where the subdomain is 'live' but nothing is listening on port 3000 yet. … The atoms mention this in passing … but it's easy to skim past when you're focused on getting the deploy out."* (`classic-bun-simple`)
- *"The single piece of guidance that almost steered me wrong was the dev-mode dynamic-runtime checklist in the develop response."* (`classic-go-simple`)

The fact is in the corpus. It is not at the verify atom, where the agent is when 502 happens.

### Edit goal

Add a precondition gate at the **top** of `develop-first-deploy-verify.md` body:

```
### Before verify on dev-mode dynamic runtimes

Dev-mode dynamic runtimes start as `zsc noop --silent` after the first
deploy — nothing is listening yet. `zerops_verify` will return
`http_root: HTTP 502` and that is NOT a deploy failure. Start the dev
process via `zerops_dev_server action=start` first, then verify.

For simple-mode and standard-mode runtimes the runtime starts on
deploy; verify directly.
```

Place this section before the existing "Verify the first deploy" section. Add a `references-atoms: [develop-dynamic-runtime-start-container]` cross-link in the frontmatter so the agent has a one-click follow-on.

### RED check

```
go test ./internal/content -run TestAtomAuthoringLint
go test ./internal/workflow -run TestScenario_S6_DevelopDeployOKPendingVerify
```

The `references-atoms` field must point at an atom that exists. Verify with:

```
grep -l "^id: develop-dynamic-runtime-start-container" internal/content/atoms/
```

### GREEN check

```
go test ./internal/content/...
go test ./internal/workflow/...
make lint-local
```

Axis K alert: avoid "the runtime auto-starts" handler-behavior phrasing. Use observable framing: "deploys idle on `zsc noop --silent`" / "the runtime starts on deploy" — the user can SEE this in the run.start of zerops.yaml.

Axis M alert: do not name invisible-state fields like `BootstrappedAt` or `firstDeployedAt`. The added text only references user-visible verify check status (`http_root: HTTP 502`) and tool actions (`zerops_dev_server action=start`).

### Slow-loop check

```
./eval/behavioral/flow-eval.sh classic-bun-simple
./eval/behavioral/flow-eval.sh classic-rust-postgres-standard
./eval/behavioral/flow-eval.sh classic-go-simple
```

The dev-mode 502 friction note should be absent in all three new retrospectives. If even one still surfaces it, the precondition wording isn't strong enough — escalate by including the literal mode-check the agent should run before verify (e.g. *"If the snapshot reports `mode: \"dev\"` and `runtimeClass: \"dynamic\"`, run `zerops_dev_server action=status` before `zerops_verify`."*) and re-run.

---

## Cross-phase verification

After all three phases land:

1. **Fast (~2 min)**:
   ```
   make lint-local
   go test ./internal/content/... -count=1
   go test ./internal/workflow/... -count=1
   ```
   All green is required before slow-loop.

2. **Slow (~30 min on the targeted scenarios above)**: Re-run the seven scenarios cited in §"Slow-loop check" sections. If wall-time budget is tight, run the three Phase 3 scenarios + one each from Phase 1 and Phase 2 (5 scenarios, ~25 min).

3. **Inspection**: read each new `self-review.md` against `eval/behavioral/runs/20260504-104436/<scenario>/self-review.md`. Friction line for the targeted class should be absent or strictly weaker.

4. **Decision point**: If 5+ of 7 scenarios show the friction dropped, plan succeeds. If 3 or fewer, escalate per the in-phase "if friction persists" notes. If 4, judgment call — either escalate one phase or accept partial drop and document remaining surface as backlog.

---

## Round-trip protocol (per `eval/behavioral/README.md` §"Round-trip protocol for fix verification")

1. Land each phase as a separate commit (atoms only — no test changes unless goldens drift mechanically).
2. Re-run the targeted scenarios via `flow-eval.sh`.
3. Compare `self-review.md` files conversationally; do **not** add an automated grader. The local Claude Code session reading the new and old reviews is the grader, per the existing eval contract.

---

## Risks

- **Render-probe goldens drift** — atoms render their bodies into snapshots; any text edit changes downstream golden test output. Refresh via existing `-update` flag where present, and visually inspect diffs to confirm only intended changes. Pinned by `internal/workflow/aggregate_render_probe_test.go` and adjacent.
- **Atom bloat** — adding a section per atom risks pushing the atom past the "TRIGGER + ACTION + FAILURE MODE in 1-3 lines per finding" principle in `CLAUDE.local.md`. Each phase's added text MUST stay under 8 lines body-side; if it doesn't fit, the edit is wrong (likely repeating context that should already be in the routing).
- **Friction-frequency overlap** — the 7 / 8 / 3 counts in §"Why now" overlap. A successful Phase 1 + 2 may suppress Phase 3 friction indirectly (less time on bootstrap means more attention on verify). Read the slow-loop diffs against the suite as a whole, not just per-phase.
- **Phase 2 over-correction** — adding the JSON example in two atoms (recipe + classic-dynamic) is duplicative content. Acceptable per `CLAUDE.md` "three similar lines is better than premature abstraction" — both atoms route differently and both miss the example today. If a third or fourth atom later needs the same example, extract a shared atom.
- **The simple-mode case** — Phase 3 explicitly says "for simple-mode and standard-mode runtimes the runtime starts on deploy; verify directly." Validate this against the develop-first-deploy-* siblings; if a sibling already says something subtly different, reconcile them in the same commit.

---

## Out-of-scope follow-ons surfaced by suite `20260504-104436`

These are observed but deliberately NOT in this plan. Either backlog them or tackle in a separate plan:

- `setup` parameter UX on cross-deploy — workflow develop "Next" hint omits `setup` for multi-setup zerops.yaml; recipe projects fail. Backlog: `plans/backlog/develop-next-hint-setup-aware.md` candidate.
- `error_logs` info-status detail strings reading as warnings (classic-rust, recipe-laravel-minimal). Tool/handler concern, not atom.
- `buildDuration` semantics on failed deploy. Tool/handler concern.
- Two scenarios hit `error_max_turns` (greenfield-fullstack-multi-runtime, recipe-nextjs-ssr-frontend-standard). Likely needs MaxTurns increase to 100-120 for fullstack/SSR scenarios. Backlog candidate.
- `existing-simple-mode-add-endpoint` fixture buildFromGit URL needs verification — separate from this plan; fix in a small commit by either patching the URL or switching to `startWithoutCode: true`.
- `bootstrap-mode-prompt` already has the example; consider whether a future Phase 4 deletes it from there once recipe + classic-dynamic carry their own canonical example, or whether mode-prompt continues to carry the canonical and the others reference it.

---

## Done definition

This plan is complete when:

- All three atom edits are committed.
- `make lint-local` is green.
- Slow-loop verification on at least 5 of the 7 cited scenarios shows the targeted friction class absent or downgraded in the new `self-review.md`.
- `plans/atom-edits-bootstrap-verify-2026-05-04.md` is moved to `plans/archive/` per `CLAUDE.md` plan-doc lifecycle.
