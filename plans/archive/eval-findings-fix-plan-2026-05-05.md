# eval-findings-fix-plan — 2026-05-05

Source: behavioral-eval suite `20260505-151844` (21 scenarios, 20 with self-review + 1 seed-fail at `existing-simple-mode-node-add-endpoint` — fixture seed errored, no transcript, plan claims do not extend to that scenario). Cross-validated through transcript ground-truth + code archaeology + two independent codex passes (resume + fresh).

**Discipline**: every finding is questioned per CLAUDE.local.md "Question the artefact" — why exists, should exist, behave differently, when/why drifted, validate against trunk.

**Two review passes after first synthesis revealed corrections:** original plan placed H2 fix at the wrong layer (Recovery emission) when the root sits one layer up in the workflow router; H3a's contradiction is wider than next_actions.go (classifier baseline emits same lying SuggestedAction); H1's "incomplete sweep" framing holds, H2's does not (status-as-recovery is deliberate per CLAUDE.md:271-277). This document reflects post-review state.

---

## Meta-pattern (refined after fresh-codex review)

H1 and H3a share one shape — single commit introduced a pinned contract, swept some sites, missed others in the same file/concept:

| Finding | Pinned pattern | Originating commit | Swept | Not swept |
|---|---|---|---|---|
| H1 | "non-network failures must NOT carry `category=network`" | `821f6113` (Apr 27) | Credential class for git-token / zcli-login | zcli arg-validation rejections (Cannot find corresponding setup, etc.) |
| H3a | "deploy-status response strings are `hasLogs`-aware" | `c9dd156d` (Apr 5) | `deploySuggestionForStatus(status, hasLogs)` for BUILD_FAILED | `deployNextActionForStatus(status)` AND `baselineForPhase(PhaseBuild).SuggestedAction` (third contradicting string in same response) |

**H2 is structurally different** and was mis-framed in the first synthesis:
- Status-as-recovery is **deliberate architecture** per CLAUDE.md:271-277 ("Lifecycle recovery is via action=status... canonical lifecycle envelope... pinned by P4"). Treating every `WithRecoveryStatus()` as incomplete sweep ignores explicit intent. There ARE legitimate non-recovery cases (e.g. `verify_recovery_test.go:81-107` pins stopped-services-no-recovery; `verify_test.go:333-338` pins URL-not-resolved-skip).
- The actual H2 root sits **one layer up in the workflow router**: `build_plan.go:382-388` `adoptRuntimesAction()` emits `workflow=develop intent=adopt`, but adoption is a bootstrap-route per `router.go:62-69` AND per `workflow_develop.go:108` rejection hint ("Run bootstrap first... route=adopt"). The router and develop-rejection disagree on which workflow owns adoption. Develop's `WithRecoveryStatus()` is then a **defensive** rejection at the wrong layer; even with specific Recovery the agent shouldn't have arrived there.
- Fix layering: (1) primary — fix `adoptRuntimesAction` to emit `workflow=bootstrap, route=adopt`; (2) secondary — `WithRecovery` on develop's PREREQUISITE_MISSING as defense-in-depth.

**Implication**: H1 and H3a are clean "complete the sweep" fixes. H2 is "fix the routing layer first, recovery copy second". No grand restructure either way.

H3b (sub-10s builds → empty buildLogs) is **design boundary**, not bug — tag-scoping is sound by spec invariant. H4 (setup="prod" in Next hints) is **self-review hallucination** confirmed by codex — no scenario actually failed; recipe convention (`setup: prod`) matches recipe yaml, and self-review prose conflated service hostnames with yaml block names.

---

## H1 — classifier emits `category=network` for zcli arg-validation rejection

### Ground truth
- **Transcript** (`cross-deploy-stage-promote-from-dev`): zcli stderr `✗ ERR Cannot find corresponding setup in zerops.yaml`. Wire response: `failureClassification.category=network, likelyCause="Transport-layer error reaching the platform.", signals=[phase:transport]`.
- **Code path traced**:
  1. `internal/ops/deploy_ssh.go:171` → `classifySSHError(err)` 
  2. `internal/ops/deploy_classify.go:55` default branch wraps zcli stderr into `ErrSSHDeployFailed` (intent: "let LLM reason about it" via Diagnostic field)
  3. `internal/tools/deploy_failure_classify.go:36-44` — `Phase` switches to `PhasePreflight` only for 6 PlatformError codes; `ErrSSHDeployFailed` not among them
  4. No signal in `deploy_failure_signals.go` matches "Cannot find corresponding setup"
  5. `baselineForPhase(PhaseTransport)` fires → `category=network`

### Question the artefact

- **Why exists**: commit `821f6113 deploy(E2): structured failureClassification on every failed deploy response` (Apr 27). Author's own intent: *"add FailureClassCredential so git-token / zcli-login failures stop being classified as network errors (14 files swept)"*. Same problem class — non-network failures wrongly carrying `category=network`. He swept Credential. Did not sweep zcli arg-validation.
- **Should exist**: yes. `baselineForPhase` is sound design — the invariant "every failed deploy response carries a category" forces classification discipline. Removing it would create null-classification edge cases agents must handle.
- **Behave differently**: the **phase taxonomy** is too broad. `PhaseTransport` documents *"SSH / zcli could not reach the platform"* but in practice catches both (a) connectivity drops AND (b) zcli local arg-validation rejections (which DO reach zcli but not the platform). Greenfield design would split: `PhaseTransport` = network/SSH only, `PhaseClient` (or extension to `PhasePreflight`) = zcli local rejection.
- **Drift**: not stale code; **incomplete sweep** of 821f6113's pattern. Author saw the friction class (credential errors mis-classed as network), fixed credential, didn't continue to zcli arg-validation. Comment-vs-code mismatch in PhaseTransport baseline (`return network without LikelyCause` vs `LikelyCause: "Transport-layer error..."` populated) is a cosmetic artefact, NOT the load-bearing issue.
- **Validate against trunk**: ZCP's failure-classification architecture is *"Categories live in `topology.FailureClass` (single canonical enum, peer to ops + workflow); classifier + pattern library in `internal/ops/deploy_failure*.go`"* (CLAUDE.md). The fix lives squarely in this layer. No layer-jumping needed.

### Fix

**Two-step, both in commit:**

1. Add signal in `deploy_failure_signals.go`:
   ```go
   {
       id:     "transport:zcli-setup-mismatch",
       phases: []DeployFailurePhase{PhaseTransport},
       logSubstrings: []string{"Cannot find corresponding setup"},
       build: func(_ string) *topology.DeployFailureClassification {
           return &topology.DeployFailureClassification{
               Category:        topology.FailureClassConfig,
               LikelyCause:     "zerops.yaml setup-block name does not match the --setup arg.",
               SuggestedAction: "Pass setup=<block-name>; the block names declared in zerops.yaml are the only valid values.",
               Signals:         []string{"phase:transport", "transport:zcli-setup-mismatch"},
           }
       },
   }
   ```

2. Sweep companion zcli local-validation patterns in same commit (saves a follow-up sweep):
   - `unknown base` / `unknown stack` — same shape, category=config
   - Anything else discoverable via `grep -rn 'fmt.Errorf' zcli source` if accessible; otherwise empirical from past evals

3. **Test**: extend `TestClassifyDeployFailure_*` table with the three new patterns. Pinned per existing convention.

### NOT in scope

- Phase taxonomy refactor (`PhaseClient`) — backlog if future evidence shows more misclasses. Current fix solves the surface friction without the migration cost.
- PhaseTransport baseline LikelyCause text edit — cosmetic; signal short-circuits before baseline now. Drop comment-vs-code mismatch claim from the prior synthesis; it was secondary.

---

## H2 — adoption routed to develop instead of bootstrap (root) + generic Recovery on develop's defensive rejection (symptom)

### Ground truth
- **Transcript** (3 scenarios): `existing-standard-appdev-only-reminders`, `recover-failed-buildfromgit-missing-dep`, `verify-subdomain-recovery-before-browser` all return identical `recovery:{tool:zerops_workflow, action:status}` for `PREREQUISITE_MISSING / "No bootstrapped services found"`.
- **Counter-evidence**: same transcripts contain non-status Recovery for OTHER errors: `recovery:{tool:zerops_logs, action:fetch, args:{...}}` (import override), `recovery:{tool:zerops_subdomain, action:enable, args:{...}}` (verify subdomain). So infrastructure exists.
- **Recovery struct** (`internal/topology/recovery.go`): `{Tool, Action, Args map[string]string}`. Args support has been in struct from the start. Codex's suggestion that "Recovery shape is {tool, action} only" was incomplete — Args field is present.

### Question the artefact

- **Why exists**: commit `d1520c37 tools+platform: unified ErrorWire + recovery hint sweep (G14, G10)` introduced ErrorWire. Comment on `WithRecovery`: *"Reserved for future non-status recoveries"*. P4 contract: *"status is the single entry point for envelope"*. So WithRecoveryStatus was the **original design intent** — status is the canonical recovery primitive.
- **Should exist**: `WithRecoveryStatus` is appropriate for **non-actionable transients** (per CLAUDE.md verify-Recovery convention: *"Skip status reserved for non-actionable transients (URL not yet resolved)"*). For **actionable preconditions** with computable next-call, specific Recovery is the pinned standard.
- **Behave differently**: develop's PREREQUISITE_MISSING is an **actionable precondition**. Next call IS computable: `zerops_workflow action=start workflow=bootstrap` (route=adopt is suggested by route-menu's first-call). Generic status forces a round-trip where specific Recovery would prevent it.
- **Drift**: not stale design. **Incomplete sweep** of the pinned verify-Recovery pattern across rejection paths. Verify checks were swept (per CLAUDE.md invariant). Import override was swept (`e75d01e4` introduced specific `zerops_logs fetch` Recovery). Develop, close-mode, and likely other workflow rejection paths weren't swept.
- **Validate against trunk**: spec — CLAUDE.md says *"Verify checks carry structured Recovery for actionable preconditions"* and pins it with TestVerify_*. The same logic applies to develop / close-mode / dev-server rejections. The pattern is system-canonical; just not yet propagated to all sites.

### Fix — Phase 1 (root, in workflow router)

```go
// internal/workflow/build_plan.go:382-388
func adoptRuntimesAction() NextAction {
    return NextAction{
        Label:     "Adopt unmanaged runtimes",
        Tool:      "zerops_workflow",
        Args:      map[string]string{"action": "start", "workflow": "bootstrap", "route": "adopt"},
        Rationale: "Existing runtime services have no bootstrap metadata yet.",
    }
}
```

This aligns the router with `router.go:62-69` (bootstrap route=adopt) and with develop's own rejection hint (`"Run bootstrap first... route=adopt"`). The `develop intent=adopt` codepath disappears.

Trace consumers: anything that interprets `intent=adopt` on develop start. If the develop tool branches on intent values, prune the adopt branch — it's now unreachable. Verify scenarios in `internal/workflow/scenarios_test.go:239-244` (codex-cited) — update fixtures so the expected Next action is bootstrap+adopt.

### Fix — Phase 2 (defense-in-depth, in develop rejection)

`workflow_develop.go:105-110` — even with phase 1 the rejection might fire if the agent ignores the router and dives into develop directly. Replace `WithRecoveryStatus()` with specific Recovery so the rejection is recoverable without round-tripping status:

```go
WithRecovery(&RecoveryHint{
    Tool:   "zerops_workflow",
    Action: "start",
    Args:   map[string]string{"workflow": "bootstrap", "route": "adopt"},
})
```

Sweep `workflow_close_mode.go:90` (same PREREQUISITE_MISSING shape) in the same commit. Do NOT do a repo-wide WithRecoveryStatus → WithRecovery sweep — codex fresh's correction is right here: status-as-recovery is deliberate per CLAUDE.md:271-277, blanket replacement would break legitimate status-recovery sites.

### Test pin (narrowed scope)

`TestRejectionsHaveSpecificRecovery` — **enumerated** sites only, not repo-wide:
- `workflow_develop.go` PREREQUISITE_MISSING → must carry bootstrap+adopt Recovery
- `workflow_close_mode.go:90` same shape
- Future actionable preconditions added to enumeration as discovered

The "what counts as actionable precondition" predicate is judgment-call; mechanical enforcement isn't possible. Test acts as regression lock for the enumerated set, not as universal enforcer.

### Adjacent improvement (NOT same commit, backlog)

Discover output already carries `managedByZcp:false` per service (verified `internal/ops/discover.go:35`). Agents in transcripts ignored it. Atom-level edit to telegraph "if `managedByZcp:false` on a target, the router will guide you to bootstrap+adopt before develop" would surface the routing intent earlier. Atom only — no code change.

---

## H3a — `deployNextActionForStatus` ignores `hasLogs`; sibling `deploySuggestionForStatus` honors it

### Ground truth
- **Transcript** (`greenfield-fullstack-multi-runtime`): same deploy response carries:
  - `suggestion: "BUILD phase failed — build logs unavailable. Check zerops.yaml buildCommands syntax..."`  ← honest about no logs
  - `nextActions: "Build failed — check buildLogs in response for build output. Fix and redeploy."`  ← tells agent to read non-existent logs
- **Self-contradiction within one response**.
- **Code**: `next_actions.go:46` `deploySuggestionForStatus(status, hasLogs)` branches on hasLogs. `next_actions.go:73` `deployNextActionForStatus(status)` doesn't take hasLogs. For `BUILD_FAILED` the latter unconditionally returns the static `nextActionDeployBuildFail`.

### Question the artefact

- **Why exists**: commit `c9dd156d feat: distinguish BUILD_FAILED/PREPARING_RUNTIME_FAILED/DEPLOY_FAILED + auto-fetch right logs` (Apr 5, Aleš). Two functions, distinct semantic role: `Suggestion` = "what happened + where logs are", `NextActions` = "what to fix". Split is reasonable design — different fields in wire response, different reading purposes.
- **Should exist**: both functions, yes. Split is sound. But **both need hasLogs-awareness**. Not just one.
- **Behave differently**: NextActions for BUILD_FAILED should branch on hasLogs identical to Suggestion. When logs unavailable, action shouldn't say "check buildLogs in response".
- **Drift**: hasLogs branching was added to `deploySuggestionForStatus` at introduction (c9dd156d shows it). `deployNextActionForStatus` was never updated when the same dichotomy applied. **Incomplete sweep within one commit's introduced concept**.
- **Validate against trunk**: deploy-response wire shape has both fields. Both reach the agent. Agent reads both. Both must be coherent. No architectural ambiguity — just an oversight.

### Fix — extends to three sites, not just two

Codex fresh found the third contradicting string: `internal/ops/deploy_failure.go:127-133` PhaseBuild **classifier baseline** also emits `SuggestedAction: "Read buildLogs for the exact stderr"` unconditionally. For sub-10s build with empty logs, all three fields lie:

1. `failureClassification.suggestedAction` — `deploy_failure.go:131` (classifier baseline, unconditional)
2. `deployResult.suggestion` — `next_actions.go:46` (already hasLogs-aware, OK)
3. `deployResult.nextActions` — `next_actions.go:73` (NOT hasLogs-aware, broken)

```go
// internal/tools/next_actions.go
func deployNextActionForStatus(status string, hasLogs bool) string {
    switch status {
    case statusBuildFailed:
        if hasLogs {
            return "Build failed — read buildLogs in response, fix buildCommands in zerops.yaml, redeploy."
        }
        return "Build failed and no logs were captured (possible sub-10s container exit). Read zerops.yaml buildCommands / dependencies syntactically, redeploy. If recurring, simplify buildCommands to isolate the failing step."
    case statusPreparingRuntimeFailed:
        // same hasLogs split as deploySuggestionForStatus
        ...
    }
}
```

Codex fresh corrected my call-site count: `deployNextActionForStatus` has **one** call site (`internal/tools/deploy_poll.go:103-105`), not four. Update is single-site. `statusDeployFailed` does NOT need hasLogs — runtime logs require separate `zerops_logs` call by design (codex resume's note, valid exception).

For `deploy_failure.go:127-133` PhaseBuild baseline, same dichotomy applies — make `SuggestedAction` conditional on whether `BuildLogs` field of `FailureInput` is empty:
```go
case PhaseBuild:
    cls := &topology.DeployFailureClassification{
        Category:    topology.FailureClassBuild,
        LikelyCause: "Build pipeline failed; no recognized log pattern matched.",
        Signals:     []string{"phase:build"},
    }
    if len(in.BuildLogs) > 0 {
        cls.SuggestedAction = "Read buildLogs for the exact stderr; fix buildCommands or dependencies in zerops.yaml."
    } else {
        cls.SuggestedAction = "Build logs were not captured before the container exited. Re-check zerops.yaml buildCommands syntax + manifests; consider simplifying to bisect."
    }
    return cls
```

PhasePrepare baseline (line 134-139) has same shape — also worth conditioning on `len(in.BuildLogs)`.

### Test (golden-style, codex fresh's framing)

`TestNextActionsBuildFailedHasLogsContradiction` — golden-test specific cases (BUILD_FAILED with hasLogs=true vs false), assert no contradicting log-reference text across the three response fields. NOT a structural "both branch or neither" rule — that would false-positive on legitimate asymmetries like `statusDeployFailed`.

---

## H3b — buildLogs unavailable for sub-10s build failures (NOT a fix)

Codex correct: this is **design boundary**. Per CLAUDE.md invariant *"Per-build log scoping uses tag identity — querying by serviceStackId alone returns historical entries; FetchBuildLogs scopes by Tags: [zbuilder@ + event.ID]"* — tag-scoping is sound. For 5s build failures, zbuilder hasn't emitted tagged log entries yet. Empty-array response is correct.

The 5s build dies → zbuilder process events DO carry status info (transcript: `"hint": "FAILED: Process failed."`). **Backlog candidate**: when `buildLogs=[]` AND build duration < threshold, surface zbuilder process event hint in the deploy response (not as buildLogs but as `processFailureHint` field). Independent of fix-now plan.

---

## H4 — workflow Next hint setup= ambiguity (real residual, but already-known symptom-fix)

Initial framing as pure self-review hallucination was wrong. Codex fresh resurrected real residuals:
- `internal/workflow/build_plan.go:265` — `deployActionFor` hardcodes `"setup": "prod"` for stage cross-deploy, omits setup for self-deploy. For users with hostname-named blocks (e.g. `appdev`/`appstage`), `setup="prod"` is literally non-matching.
- `classic-php-mariadb-standard` self-review confirms: *"my zerops.yaml setup blocks were named appdev and appstage... the next-action hint from the workflow said setup=\"prod\""* — agent recovered by reading docstring, no deploy failure.
- Recipe-convention users (e.g. `classic-rust-postgres-standard` with dev/prod blocks) get correct hint.

**Crucial: the file's own comment block at lines 252-256 explicitly flags this as a known symptom-fix pending structural correction**:
> *"The current implementation is a symptom fix that infers the same data inline at dispatch time. See audit-prerelease-internal-testing-2026-04-29.md H1 + plan §3 for the structural target."*

So this is **not a new finding** — already-acknowledged technical debt with a referenced plan. Don't duplicate work; reference the existing plan and link H4 evidence to that ticket. Out of scope for this plan; surface as evidence reinforcing the structural-target plan's priority.

---

## Fix order + per-fix RED-GREEN cadence

| Order | Fix | Test (RED first) | Files | Risk |
|---|---|---|---|---|
| 1 | H3a — thread hasLogs into deployNextActionForStatus | `TestNextActionsHasLogsSymmetry` | `internal/tools/next_actions.go` + 4 call sites | low (mechanical) |
| 2 | H1 — signal pattern for zcli setup-mismatch | extend `TestClassifyDeployFailure_*` table | `internal/ops/deploy_failure_signals.go` | low |
| 3 | H2 — specific Recovery for develop PREREQUISITE_MISSING | `TestRejectionsHaveSpecificRecovery` | `internal/tools/workflow_develop.go` (+ sweep close_mode etc.) | medium (sweep audit) |

Each fix in its own commit. RED test first, then implementation. CLAUDE.md invariant updates land with the fix that introduces the pinned pattern.

---

## Elevation: prevent the meta-pattern from recurring

The meta-finding is that pinned patterns get stuck mid-sweep. Three observable defenses:

1. **`TestRejectionsHaveSpecificRecovery`** — table-driven, lists every `convertError(...)` call site in `internal/tools/`, asserts: if the next-call is computable from the rejection context, Recovery shape MUST be specific (tool + action + args), not status. New rejection sites added without specific Recovery fail this test. Pin in CLAUDE.md as Recovery contract invariant.

2. **`TestClassifierSignalCoverage`** — table-driven, lists every known zcli/SSH error string surfaced by past evals, asserts each has a matching signal in `deploy_failure_signals.go`. Adding a new error class without signal coverage fails this test. (Augments existing TestClassifyDeployFailure_* which is fixture-driven; this one is exhaustive across the known-error corpus.)

3. **`TestNextActionsSemanticSymmetry`** — for every status string the deploy response handles, assert that Suggestion and NextActions either both branch on hasLogs or both don't. Asymmetric introduction of hasLogs (or any future similar contextualization) fails this test.

CLAUDE.md additions (one bullet each, attached to the right convention block) — phrased as **system-level invariants** that future sweeps must satisfy, not just one-off rules.

---

## Out of scope (logged here, not acted on)

- **Phase taxonomy refactor** (PhaseTransport split) — backlog if more zcli misclasses surface.
- **discover atom edit** for `managedByZcp:false` pre-emption — atom corpus hygiene.
- **First-deploy hint missing `setup=` for multi-setup yaml** — atom prose tweak; tracked under audit-prerelease-internal-testing-2026-04-29.md H1 §3.
- **Surface zbuilder process hint when buildLogs empty** — backlog UX improvement.
- **Standard-pair scope auto-includes stage** (`workflow_develop.go:259-265`) — codex fresh resurrected as design tension. Verified: this is **deliberate** per code comment ("real-session evidence in two adopted-pair workflows that ended at the dev preview without promoting"). User-intent-vs-design-intent friction (dev-add-managed-dep scenario), but not a bug. Backlog if design-revisit warranted.
- **Recipe scope items** (e.g., setup-block-name conventions in atoms) — Aleš's scope per CLAUDE.local.md.

---

## Cross-validation log (after two review passes)

**First codex pass (initial verification, hypotheses A/B/C/D):**
- H1: real bug, converged on fix shape
- H2: design mismatch, fix shape converged
- H3: incorrectly classified as "no fix needed" — missed the suggestion-vs-nextActions self-contradiction
- H4: correctly identified as self-review hallucination
- Recovery struct shape: incomplete claim ("tool+action only"); ground-truth shows Args field exists and is in active use elsewhere

**Codex resume (review of v1 plan):**
- Acknowledged H3 verdict was wrong; confirmed plan's H3a finding
- Held meta-pattern for H1+H3a; flagged it weaker for H2 ("site missed established pattern" vs "incomplete sweep")
- Noted statusPreparingRuntimeFailed in deployNextActionForStatus also lacks hasLogs
- Pointed out architectural alternative for H1 (classifySSHError pattern detection at wrap time vs signal pattern matching diagnostic)
- Flagged statusDeployFailed asymmetry must be excluded from any symmetry test (legitimate — runtime logs require separate call by design)

**Codex fresh (cold-eyes critique of v1 plan):**
- Confirmed H1, H3a as real bugs
- **Refuted H2 framing**: status-as-recovery is deliberate per CLAUDE.md:271-277. "Incomplete sweep" narrative ignores explicit intent. Real H2 root sits one layer up: `build_plan.go:382-388` `adoptRuntimesAction` emits `workflow=develop intent=adopt`, but adoption is bootstrap-route per `router.go:62-69`. Routing layer disagrees with itself.
- Found third H3a contradiction: `deploy_failure.go:127-133` PhaseBuild baseline `SuggestedAction` unconditional. Plan extended.
- Corrected call-site count: `deployNextActionForStatus` has 1 call site, not 4.
- H4 setup-hint hardcoding (`build_plan.go:265`) is real but already-known symptom-fix per its own code comment + audit doc reference. Don't duplicate.
- Test scoping: TestRejectionsHaveSpecificRecovery must be enumerated, not repo-wide; TestNextActionsSemanticSymmetry → golden tests, not implementation symmetry; TestClassifierSignalCoverage → rename TestClassifierKnownSignals (regression guard, not completeness).
- Resurrected scope auto-include as missed pattern. Verified deliberate per code comment with real-session evidence — design tension, not bug. Backlog.
- Acknowledged seed-fail scenario boundary that v1 plan didn't honestly mark.

**Plan integrates corrections from both reviews.** Things v1 had wrong: H2 layer (had it at Recovery; root is at routing); H3a scope (missed third contradiction in classifier baseline); test predicates (had structural symmetry; need semantic golden); H4 framing (downgraded too far — real residual exists but is already-tracked).
