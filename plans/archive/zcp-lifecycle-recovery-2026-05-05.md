# Plan: ZCP non-running lifecycle recovery (v3 — supersedes 2026-05-04 + earlier 2026-05-05)

**Status**: Phase 0 (`seed: settled` + baseline scenario) shipped 2026-05-05. Empirical baseline + Codex independent reviews integrated. Phases 1-3 below.

**Supersedes**:
- `plans/zcp-failed-state-recovery-2026-05-04.md` — predecessor (correct in spirit, narrow in scope).
- `plans/zcp-non-running-state-recovery-2026-05-05.md` — earlier v2 today (over-engineered: parallel classification struct, speculative Signals field, redundant gates, STOPPED imported by mistake).

This v3 was reduced after a whole-plan critique by both Claude (self) and Codex (independent). Both flagged the v2 as solving the symptom path well but missing the root cause (agent's diagnostic instinct) and over-fitting the implementation surface.

---

## The single mental model

When an agent observes a non-running service (FAILED or READY_TO_DEPLOY-with-failed-appVersion), the correct flow is:

1. **OBSERVE** — service status + most-recent appVersion outcome.
2. **INSPECT** — read failureClass/failureCause from events; read runtime logs.
3. **RECOVER** — choose a non-destructive fix, or escalate to a destructive one with eyes open.
4. **BLOCK DESTRUCTIVE** — refuse `override=true` paths that erase the original until diagnosis happened.

Today only step 1 surfaces (raw status). Steps 2-4 either don't surface, surface in the wrong phase, or surface so quietly the agent skips them. Empirical baseline (suite `20260505-115417`) shows the agent skipping straight from step 1 to a destructive override + scaffold-over rebuild.

The plan below is one coherent implementation of those four stages. All design choices follow from the mental model, not from per-surface symmetry.

---

## What the empirical baseline + audit + reviews proved

Friction surface (highest-impact first):

| ID | What | Where the fix lives |
|---|---|---|
| F1 | `bootstrap-adopt::provision` rejects READY_TO_DEPLOY with `expected one of [RUNNING, ACTIVE]` and NO Recovery | Phase 1 (RECOVER) |
| F2 | Bootstrap-adopt provision-step text says "Adopted services are typically already deployed, so this caveat doesn't apply" — actively misleading | Phase 1 (RECOVER) |
| F3 | `develop-ready-to-deploy.md` atom keyed `phases: [develop-active]` only — never fires in bootstrap-adopt where the case actually surfaces | Phase 1 (RECOVER) |
| F4 | `verify::checkServiceRunning` on non-running emits raw status, NO Recovery | Phase 2 (INSPECT signals) |
| F5 | `zerops_dev_server` `KEY=VAL cmd` shell-prefix syntax fails silently under `exec` spawn | Phase 3 (polish) |
| F6 | Agent NEVER read runtime logs; instead destroyed original repo via `override=true` and scaffolded Flask hello-world from scratch | Phase 1 (BLOCK DESTRUCTIVE) — root cause fix |

F7 (apiMeta walls), F8 (adopt boilerplate docs), F9 (skipped wording) — backlog, not plan scope.

Audit's "what's silently broken" matrix from `eval/behavioral/audits/failed-state-zcp-audit-2026-05-04.md` remains correct as a catalog of read-side gaps. Plan covers the three highest-impact (workflow_checks + verify + the destructive-override path).

---

## What the plan does NOT do (explicit out-of-scope)

These were in v2 and pulled out after critique:

- **Parallel classification struct** (`topology.ServiceLifecycleClassification`). Codex Q3 + my self-critique: duplicates existing `topology.DeployFailureClassification`. v3 reuses the existing type with two new categories: `never-deployed` and (already exists) `start`/`build`. No new struct.
- **`Signals []string` on `ops.Recovery` / `tools.RecoveryHint`**. Speculative — agents read tool/action/args. Existing classification already has `Signals` for telemetry/test pinning. Don't conflate.
- **`ops.Manage` (restart/reload/scale/storage) gating**. No empirical evidence this surface drove friction. Restart on FAILED is sometimes the recovery itself — gating it is wrong.
- **`zerops_workflow start` gating** (develop + bootstrap entry points). PREREQUISITE_MISSING already gates develop-on-non-bootstrapped; bootstrap entry on non-running runtime is normal (adopt route handles it). No empirical gap.
- **`zerops_discover` per-service Recovery field**. Read-only surface; agents that don't act diagnostically on discover output won't change behavior just because Recovery appears. Drop unless a future scenario surfaces a discover-only flow that benefits.
- **STOPPED status**. Predecessor plan was right: STOPPED is intentional state, not failure. Treating it as "non-running needing recovery" mis-frames a deliberate user action.
- **`zerops_env auto-restart` only on ACTIVE** (Codex missing #4). Backlog: `plans/backlog/env-auto-restart-non-running.md`.

---

## Phase 0 — already shipped 2026-05-05

`seed: settled` mode + scenario `recover-failed-buildfromgit-missing-dep.md` landed.

`internal/eval/seed.go::SeedSettled` tolerates FINISHED/FAILED/CANCELED on import-side processes (empirical: buildFromGit + missing-dep marks parent process CANCELED, not FAILED) and waits for `ListServices` to settle on terminal status `{ACTIVE, FAILED, READY_TO_DEPLOY, RUNNING, STOPPED}`. Scenario uses retrospective prompt; baseline self-review captured at `eval/behavioral/runs/20260505-115417/recover-failed-buildfromgit-missing-dep/self-review.md`.

Reusable as the verification harness for Phase 1-3.

---

## Phase 1 — Recover: close the bootstrap-adopt provision gap

This is the empirically-highest-leverage fix. F1+F2+F3 all converge on the same call site (`bootstrap-adopt::provision` step) and the same atom (`develop-ready-to-deploy.md`).

### 1.1 — Atom phase axis broadening (one commit, content-only)

`internal/content/atoms/develop-ready-to-deploy.md`: change frontmatter
```yaml
phases: [develop-active, bootstrap-active]
```
Re-read the body to ensure wording works in both phases. The existing body already references `zerops_import override=true + startWithoutCode: true` — the exact recovery the agent eventually figured out manually.

If the body coupling to develop-phase concepts is too deep, fork into a sibling `bootstrap-adopt-ready-to-deploy.md` instead of broadening. Decide at edit time, not now.

Pin: scenario re-render test asserts the atom fires during `bootstrap-active::provision` step when at least one in-scope service is `READY_TO_DEPLOY`.

**Files**: 1 atom + 1 test. ~10 LOC.

### 1.2 — Provision-step guidance correction (one commit, content-only)

The misleading line *"Adopted services are typically already deployed, so this caveat doesn't apply on the adopt route"* lives in the bootstrap-adopt provision-step content (likely `internal/content/workflows/bootstrap/adopt-provision-*.md` or its atom). Replace with a status-aware fragment:

> Adopted services are usually ACTIVE. If `zerops_discover` shows status=READY_TO_DEPLOY, the service was created without `startWithoutCode: true` — see the `develop-ready-to-deploy` atom for the recovery procedure (re-import with `startWithoutCode + override=true`). DESTRUCTIVE: override REPLACES the service stack — back up code via SSH first.

Pin: render-test asserts the new text in adopt-route provision phase.

**Files**: 1 + 1 test. ~15 LOC.

### 1.3 — Recovery on `workflow_checks::checkServiceStatusAny` (one commit)

`internal/tools/workflow_checks.go:163-184` returns `expected one of [...], got X` with no Recovery. This is the empirical stall point.

Add `Recovery` field to `workflow.StepCheck` (`internal/workflow/bootstrap_checks.go:33-52`) and to `tools.CheckWire` (`internal/tools/errwire.go`). Both already flow through `WithChecks`.

When `checkServiceStatusAny` rejects with status==`READY_TO_DEPLOY` AND the service has at least one failed appVersion event in the recent window, attach Recovery pointing at `zerops_import override=true startWithoutCode=true` (with required-args list); when status is the same but no failed appVersion, attach Recovery pointing at `zerops_logs facility=application` (the rare case — service was created without ever attempting deploy). When status==`FAILED`, attach Recovery pointing at `zerops_events serviceHostname=X` (single source of truth for failureClass — see Phase 2.1 below).

Discriminator helper colocated:
```go
// internal/tools/workflow_checks.go
func nonRunningRecovery(events []platform.AppVersionEvent, status, hostname string) *tools.RecoveryHint
```

Pin: `TestCheckServiceStatusAny_ReadyToDeployWithFailedAppVersion_AttachesImportRecovery`, `TestCheckServiceStatusAny_FailedStatus_AttachesEventsRecovery`, `TestCheckWire_RecoveryRoundTrip`.

**Files**: 4 (workflow_checks.go, bootstrap_checks.go, errwire.go, tests). ~80 LOC.

---

## Phase 2 — Inspect: signals where agents already look

After Phase 1, `verify` and `deploy` are the surfaces agents most commonly hit when something's wrong. Both currently emit non-running status without pointing at the existing classification.

### 2.1 — `verify::checkServiceRunning` Recovery (one commit)

`internal/ops/verify_checks.go:56-65` emits raw status detail; attach Recovery via the same discriminator from Phase 1.3 (extracted to a shared ops helper):

```go
// internal/ops/non_running_recovery.go (new)
func RecoveryForNonRunning(ctx context.Context, client platform.Client, svc *platform.ServiceStack) *Recovery
```

The helper internally calls `client.SearchAppVersions` (cheap, already used by events.go) and reuses the existing classification produced by `internal/ops/events.go`'s appVersion enrichment path.

Side fix (audit-flagged): preserve subdomain Recovery emission when service_running fails (`internal/ops/verify.go:118` short-circuit currently discards it).

Pin: `TestCheckServiceRunning_ReadyToDeployAttachesRecovery`, `TestCheckServiceRunning_FailedAttachesRecovery`, `TestVerify_PreservesSubdomainRecoveryWhenServiceNotRunning`.

**Files**: 3. ~50 LOC.

### 2.2 — `zerops_deploy` pre-flight gate (one commit)

`internal/ops/deploy_local.go` and `internal/ops/deploy_ssh.go` (`internal/tools/deploy_ssh.go` for the handler entry): pre-deploy fetch service status; if `READY_TO_DEPLOY-with-failed-appVersion` OR `FAILED`, return ErrorWire with `RecoveryForNonRunning(...)` BEFORE pushing. Skips the wasted build cycle.

Discriminator: same helper from 2.1. `READY_TO_DEPLOY-without-failed-appVersion` is the normal first-deploy case — let through.

Pin: `TestDeploy_FailedServiceReturnsRecovery`, `TestDeploy_ReadyToDeployFreshLetsThrough`, `TestDeploy_ReadyToDeployAfterFailureRefuses`.

**Files**: 3. ~40 LOC.

### 2.3 — `zerops_dev_server` pre-spawn gate (one commit)

`internal/ops/dev_server.go::verifyDevServerTarget` only checks existence. Add status check; non-running terminal → return Recovery instead of SSH probing a doomed runtime.

Pin: `TestDevServer_FailedRefusesWithRecovery`, `TestDevServer_ReadyToDeployRefusesWithRecovery`.

**Files**: 2. ~30 LOC.

---

## Phase 3 — Block destructive: the F6 root-cause fix

The empirical baseline's worst outcome was the agent destroying the original repo via `override=true` without ever reading runtime logs. Recovery hints alone (Phases 1-2) help a smarter agent; they don't stop a confused agent from a destructive shortcut.

**The structural fix**: `zerops_import override=true` requires acknowledged-diagnosis OR explicit destructive-confirm.

### 3.1 — Override gating on import (one commit)

`internal/tools/import.go` (the `zerops_import` handler): when input has `override: true` AND the override target is a service with at least one failed appVersion event in the recent window, the handler:

1. **First call**: returns ErrorWire with code `ErrDiagnosisRequired`, message identifying the failed appVersion event's `failureClass + failureCause` (already classified by events.go), suggestion text `"Run zerops_logs serviceHostname=X facility=application since=15m to read the failure cause, then re-call zerops_import with confirmDestructiveOverride: true if you intentionally want to discard the existing service stack"`. Recovery hint points at `zerops_logs`.
2. **Second call**: when input includes `confirmDestructiveOverride: true`, proceeds with override.

The `confirmDestructiveOverride` flag is per-call, not session-state. Agents read the existing failure context, decide, then explicitly confirm.

When the override target has no failed appVersion (e.g. importing-over a healthy service to update its config), no gate fires — only `READY_TO_DEPLOY-with-failed-appVersion` and `FAILED` services trigger the friction.

Codex flagged Phase 1 alone is "another recovery sentence" not workflow control. This is the workflow control: hard refusal until diagnosis happens, with the diagnosis surface (failureClass already populated) in the rejection itself.

Pin: `TestImport_OverrideOnFailedServiceRequiresConfirmation`, `TestImport_OverrideOnHealthyServicePassesThrough`, `TestImport_ConfirmedOverrideProceedsAfterFirstReject`.

**Files**: 2 + tests. ~70 LOC.

### 3.2 — `zerops_dev_server` exec-not-shell guard (one commit)

`internal/ops/dev_server.go` start helper: detect `command` strings starting with `KEY=VAL ...` (regex `^[A-Z_][A-Z0-9_]*=`). Refuse with structured error pointing at the `env KEY=VAL cmd` form. Tool description gets the same explanation.

Independent of dev_server gating in 2.3 — different code path (parameter validation, not status check).

Pin: `TestDevServer_RejectsShellEnvPrefix_SuggestsEnvForm`.

**Files**: 2 + test. ~30 LOC.

---

## Phase 4 — Verification + invariant pin (one commit)

After Phases 1-3 land:

**Re-run baseline**: `flow-eval.sh recover-failed-buildfromgit-missing-dep`. The new self-review must satisfy a binary success metric:

> **Agent reads `zerops_logs` (or `zerops_events`) at least once before any `confirmDestructiveOverride: true` call AND before any non-confirmed `zerops_import override=true` is attempted.**

If passes: Phase 3.1 is working as the diagnose-first guard. Ship.

If fails (agent issues `confirmDestructiveOverride: true` without reading logs): the gate is too weak — agent treats the confirm flag as a checkbox, not a sentinel. Escalation: tighten the rejection message to demand reading logs explicitly, OR require `diagnosedFailureClass: "<class>"` parameter that has to match the actual failureClass (impossible to fake without reading events). Decide based on what the second baseline shows.

**Cross-suite regression**: re-run `greenfield-node-postgres-dev-stage`. No Recovery on healthy services — confirms populate-only-on-non-running rule.

**CLAUDE.md invariant pin** (after the above passes):

> **Non-running services with failed history surface their failureClass at every read-side surface and gate destructive overrides** — `zerops_verify::service_running`, `zerops_workflow` provision-step status check, `zerops_deploy` pre-flight, and `zerops_dev_server` pre-spawn all attach `Recovery` derived from `ops.RecoveryForNonRunning`, which reuses the existing failed-appVersion classification from `ops/events.go`. `zerops_import override=true` against a service with failed-appVersion history requires `confirmDestructiveOverride: true` after the agent reads the failure cause. Pinned by `TestImport_OverrideOnFailedServiceRequiresConfirmation`, `TestCheckServiceStatusAny_ReadyToDeployWithFailedAppVersion_AttachesImportRecovery`, atom `develop-ready-to-deploy` axis includes `bootstrap-active`.

**Files**: 1 (CLAUDE.md). ~5 LOC.

---

## Sequencing

Phase ordering (NOT parallelism — each phase verifies before the next):

1. **Phase 1** (1.1 + 1.2 + 1.3) — three commits, ~105 LOC, content + workflow_checks plumbing.
2. **Phase 2** (2.1 + 2.2 + 2.3) — three commits, ~120 LOC, surface plumbing reusing the helper from 2.1.
3. **Phase 3** (3.1 + 3.2) — two commits, ~100 LOC, including the root-cause fix.
4. **Phase 4** — one commit, ~5 LOC, invariant pin.

**Total: 9 commits, ~330 LOC**, down from v2's 15 commits / ~700 LOC.

Phase 1 alone closes F1+F2+F3. Phase 2 closes F4 + adds defence-in-depth on agent's most-touched surfaces (verify, deploy, dev_server). Phase 3 closes F6 (the worst empirical outcome) + F5 (cosmetic).

**No parallelism shortcut**. Each phase verifies (ideally with a re-run) before the next ships. The savings versus v2 come from cutting speculative scope, not from compression.

---

## Risks (delta from v2)

- **Phase 3.1 confirmDestructiveOverride may be too weak.** Phase 4's binary success metric is the canary. Mitigation already specced: require `diagnosedFailureClass` parameter that must match actual failureClass.
- **Atom phase broadening (1.1) may need a fork instead of broadening.** Decide at edit time. Plan accommodates both.
- **`StepCheck` Recovery field ripple.** Many existing tests assert `StepCheck` shape via golden comparison. Plan ~30 min for golden updates in Phase 1.3.
- **Helper consolidation between Phase 1.3's tools-layer discriminator and Phase 2.1's ops-layer helper.** Risk: drift if implemented in different commits. Mitigation: ship 2.1 first when it lands; 1.3 imports the helper.

Wait — that last point reveals a sequencing tension. The tools-layer (1.3) can't import ops-layer helper unless we ship 2.1 first. Re-sequence: Phase 2.1 (ops helper) before Phase 1.3 (tools-layer consumer). Updated order:

1. Phase 1.1 + 1.2 (content-only, no helper dependency) — 2 commits
2. Phase 2.1 (ops helper) — 1 commit
3. Phase 1.3 (tools-layer consumer of ops helper) — 1 commit
4. Phase 2.2 + 2.3 (more ops-helper consumers) — 2 commits
5. Phase 3.1 + 3.2 (override gate + dev_server polish) — 2 commits
6. Phase 4 (invariant pin) — 1 commit

Total still 9 commits.

---

## Done definition

This plan is complete when:

1. Phase 0 shipped (DONE).
2. Phase 1 atoms updated, render-tests pass.
3. Phase 2 ops helper + three plumbing sites pass tests.
4. Phase 3 import gate + dev_server polish ship.
5. Re-run of `recover-failed-buildfromgit-missing-dep` self-review satisfies the binary success metric (logs read before override-confirm).
6. Cross-suite regression scenario shows no false-positive Recovery on healthy services.
7. CLAUDE.md invariant added.
8. Plan moved to `plans/archive/`.
9. Predecessors v1 (2026-05-04) and v2 (earlier 2026-05-05) moved to `plans/archive/` referencing this v3 as supersession.

---

## Why this v3, not v2 or v1

**v1 (2026-05-04 FAILED-state)**: framing too narrow. Empirical baseline showed READY_TO_DEPLOY-with-failed-appVersion is the more common case; FAILED service status is rare.

**v2 (earlier 2026-05-05 non-running-state)**: framing right but implementation over-fit. Three glued workstreams instead of one coherent design. Parallel classification struct duplicating existing one. Speculative Signals field. Redundant gates (manage, workflow start). STOPPED imported by mistake. ~700 LOC for what is structurally closer to ~330.

**v3 (this)**: one unified state machine — observe / inspect / recover / block destructive. Reuses existing `events.go` classification (Codex missing #3). One ops helper, three plumbing sites. The root-cause fix (Phase 3.1 import gate) is FIRST-CLASS, not an atom afterthought. Binary success metric. Cuts speculative scope.

The reduction came from:
- Codex independent review (Q3 over-engineering, missing #3 reuse, Q5 single state machine framing, Q7 no-over-engineering audit)
- Self re-evaluation as absolute whole (sequencing-claim correction, STOPPED scope mistake, parallel-struct symmetry-as-virtue mistake)
- Empirical baseline (F6 root-cause = scaffold-over, not Recovery-absence; F1 stall point = workflow_checks, not discover)

Both reviewers independently said: "ship simpler version". This is that.
