# Plan: Diagnose-before-destruct invariant (v4 — final)

**Status**: Phase 0 (`seed: settled` + baseline scenario) shipped 2026-05-05. Architectural reconsideration complete after four parallel investigations (Codex × 2, Explore × 2). Phases 1-4 below.

**Supersedes**:
- `plans/zcp-failed-state-recovery-2026-05-04.md` (v1 — narrow framing)
- `plans/zcp-non-running-state-recovery-2026-05-05.md` (v2 — over-engineered surface)
- `plans/zcp-lifecycle-recovery-2026-05-05.md` (v3 — narrowed scope, missing the architectural pattern)

This v4 is the final synthesis after the user asked: *"Should ZCP wire 'read state/events/logs before any action' as a foundational architectural concept?"* The answer, from four independent investigations, is **No — but with a structural gating invariant that promotes the destructive-override pattern from a one-off Phase 3.1 fix to a reusable contract.**

---

## The architectural verdict (whole-plan synthesis)

**Read-before-act as a generalized invariant is the wrong abstraction.** Three findings establish this:

1. **Codex (PAO architectural review)**: "Pushing a generalized enforcement contract across 23 tools would add overhead everywhere while the empirical problem is a single missing gate on `zerops_import`." Most "writes" already read state, but only to resolve IDs (e.g. `ListServices` → `FindService`) — that's not diagnosis. Conflating ID-resolve with failure-context-inspection misclassifies the deficiency.

2. **Explore (envelope failure-context audit)**: Eager event fetching in the envelope would break `spec-workflows.md §8` P4 — *"status is the canonical recovery primitive after context compaction"* — by adding 1-10s latency to every status call. Failure context belongs in **Recovery hints** (already-existing pattern at `internal/ops/verify.go:36`) and **lazy `zerops_events`** (already classified at `internal/ops/events.go:258-276`), not in eager envelope axes.

3. **Codex (PAO honest critique)**: *"PAO as guidance cannot override LLM instruction-following when the tool description already presents override as the recovery path. The eval agent did not ignore guidance — it followed the tool description correctly. The fix must be structural (gate) not prose (guidance)."*

**The empirical truth (from baseline 2026-05-05)**: the agent destroyed user code via `zerops_import override=true` not because guidance was missing — because there was no structural gate. Adding more "read first" prose in atoms doesn't change a destructive-tool's behavior at the API boundary; only an enforcement gate at the API boundary changes it.

**The right invariant** is **diagnose-before-destruct**: every always-dangerous operation refuses to mutate without an acknowledgment of what's being destroyed. The acknowledgment is structured (not prose), uniform across destructive tools, and validated server-side (not advisory).

This v4 plan implements that invariant + a small atom-corpus consistency pass that improves guidance quality independently of the structural enforcement.

---

## Empirical destructive surface — the catalog that drives scope

From the destructive-ops audit, ZCP's "always-dangerous, no/weak gate" surface today:

| Tool/Op | What it destroys | Today's guard | v3/v4 status |
|---|---|---|---|
| `zerops_import override=true` | Replaces service stack: container + deployed code + env vars + SSHFS contents on matching hostnames | Warning AFTER mutation — too late | **v3 Phase 3.1 (DONE in v3)** — adds first-call rejection with `confirmDestructiveOverride` |
| `zerops_env action=set` (service-level) | Tool says "upsert" but writes only provided pairs — silently replaces entire service env | None — silently replaces | **v4 deferred until empirical signal** |
| `zerops_env action=delete` | Irrecoverable env var deletion | None | **v4 deferred until empirical signal** |
| `zerops_env action=generate-dotenv` | Overwrites local `.env` with no backup, no existing-file check | None | **v4 deferred** (local-side) |
| `zerops_delete` | Permanent service deletion | Tool prose ("explicit user approval"); self-delete blocked | **v4 keep current — empirically OK** |
| `zerops_workflow start force=true` | Discards active WorkSession | Manual close-mode blocks unless `force=true` | **v4 keep current — already gates** |
| `zerops_workflow reset` | Clears local sessions + orphan metas | Returns structured cleared/preserved report | **v4 keep current — local-only state** |
| `zerops_deploy` self-deploy | Self-deploy with narrow `deployFiles` destroys source | DM-2 hard error (`INVALID_ZEROPS_YML`) — best example of structural gate | **v4 reference exemplar** |

The **empirical priority is `zerops_import override=true`**. The other always-dangerous surfaces (`env set/delete/generate-dotenv`) are documented gaps but have no eval signal showing destructive misuse. v4 implements the **shared shape** that makes adding their gates a 30-LOC delta when evidence emerges, without speculatively gating today.

---

## What this plan does — three coordinated phases

### Phase 0 — already shipped 2026-05-05

`seed: settled` mode + scenario `recover-failed-buildfromgit-missing-dep.md`. Reusable verification harness for v4.

### Phase 1 — workflow_checks Recovery + atom phase coverage (the v3 core, kept)

Closes F1–F4 from the empirical baseline. Same scope as v3's structural commits, no changes.

#### 1.1 — `develop-ready-to-deploy.md` phase axis broadening

`internal/content/atoms/develop-ready-to-deploy.md` frontmatter:
```yaml
phases: [develop-active, bootstrap-active]
```
If body wording proves too develop-coupled, fork into a sibling. Decide at edit time.

Pin: scenario re-render test asserts atom fires during `bootstrap-active::provision` for any in-scope service with `serviceStatus=READY_TO_DEPLOY`.

**~10 LOC, content + 1 test.**

#### 1.2 — Bootstrap-adopt provision-step guidance correction

Fix the misleading *"Adopted services are typically already deployed, so this caveat doesn't apply"* line. Replace with status-aware fragment that points at the W1.1 atom.

**~15 LOC, content + 1 test.**

#### 1.3 — `LatestFailedAppVersionContext` ops helper

**New file** `internal/ops/failed_context.go`:
```go
// LatestFailedAppVersionContext returns the most-recent failed appVersion's
// failureClass + failureCause + a suggested-read tool hint, OR nil when no
// failed history exists. Reuses the existing classification path in
// ops/events.go (does NOT re-implement). Backed by a single
// SearchAppVersions call + optional log fetch on hit.
func LatestFailedAppVersionContext(
    ctx context.Context,
    client platform.Client,
    fetcher platform.LogFetcher,
    projectID, hostname string,
) (*FailedDeployContext, error)

type FailedDeployContext struct {
    FailedAt          time.Time
    FailureClass      topology.FailureClass
    FailureCause      string
    SuggestedReadTool string  // "zerops_logs" or "zerops_events"
    SuggestedArgs     map[string]string
}
```

Pin: `TestLatestFailedAppVersionContext_NoHistoryReturnsNil`, `_ReturnsClassifiedRecent`, `_PassesThroughClassifyDeployFailure`.

**~80 LOC + tests. Foundational helper used by 1.4 + Phase 2.**

#### 1.4 — Recovery on `workflow_checks::checkServiceStatusAny`

Add `Recovery` field to `workflow.StepCheck` (`internal/workflow/bootstrap_checks.go:33-52`) and `tools.CheckWire` (`internal/tools/errwire.go`). Use the helper from 1.3.

When `checkServiceStatusAny` rejects with `READY_TO_DEPLOY` AND the service has failed-appVersion history: Recovery → `zerops_import override=true startWithoutCode=true confirmDestructiveOverride=true`. When `READY_TO_DEPLOY` without failed history: Recovery → `zerops_logs facility=application` (rare case). When `FAILED`: Recovery → `zerops_events serviceHostname=X`.

Pin: `TestCheckServiceStatusAny_ReadyToDeployWithFailedAppVersion_RecoveryToImport`, `TestCheckWire_RecoveryRoundTrip`.

**~80 LOC + 4 file ripple, ~30 min for golden-test sweeps.**

---

### Phase 2 — Recovery on read-side surfaces (verify + deploy + dev_server)

Closes F4 + adds defence-in-depth on the surfaces agents most commonly hit. Reuses 1.3 helper.

#### 2.1 — `verify::checkServiceRunning` Recovery

`internal/ops/verify_checks.go:56-65`: when status is non-running terminal, attach Recovery via `LatestFailedAppVersionContext`.

Side fix (audit-flagged): preserve subdomain Recovery emission when service_running fails (`internal/ops/verify.go:118` short-circuit currently discards it).

**~50 LOC, 3 files.**

#### 2.2 — `zerops_deploy` pre-flight gate

`internal/ops/deploy_local.go` + `internal/ops/deploy_ssh.go`: pre-deploy fetch service status + `LatestFailedAppVersionContext`. If `FAILED` or `READY_TO_DEPLOY-with-failed-appVersion`, return ErrorWire with Recovery BEFORE pushing. `READY_TO_DEPLOY` without failed history is the normal first-deploy path → let through.

**~40 LOC, 3 files.**

#### 2.3 — `zerops_dev_server` pre-spawn gate

`internal/ops/dev_server.go::verifyDevServerTarget`: add status check. Non-running terminal → return Recovery. Independent of 3.2 (different code path).

**~30 LOC, 2 files.**

---

### Phase 3 — Diagnose-before-destruct invariant (THE architectural piece)

Promotes the v3 Phase 3.1 idea from a one-off override gate into a **reusable structural contract**. Empirical scope is `zerops_import override=true` only — the contract makes it cheap to add gates to other always-dangerous tools when evidence emerges.

#### 3.1 — Shared destructive-acknowledgment shape (new vocabulary)

**`internal/platform/errors.go`**: add error code
```go
ErrDiagnosisRequired = "DIAGNOSIS_REQUIRED"
```

**`internal/tools/errwire.go`**: add typed wire field
```go
type DiagnosedDestruction struct {
    Operation string             `json:"operation"`           // "import-override", "env-set", ...
    Targets   []string           `json:"targets"`             // service hostnames affected
    Loss      DestructionLoss    `json:"wouldDestroy"`        // structured "what's about to be erased"
}

type DestructionLoss struct {
    ServiceStacks   []string `json:"serviceStacks,omitempty"`
    EnvVars         []string `json:"envVars,omitempty"`
    LocalFiles      []string `json:"localFiles,omitempty"`
    UncommittedCode bool     `json:"uncommittedCode,omitempty"`
}
```

`ErrorWire` gains `WouldDestroy *DiagnosedDestruction \`json:"wouldDestroy,omitempty"\``.

**Tool input convention**: every destructive-tool input gets a `confirmDestructive` field carrying:
```go
ConfirmDestructive *DestructiveAck `json:"confirmDestructive,omitempty"`

type DestructiveAck struct {
    Operation              string `json:"operation"`               // must match tool's `wouldDestroy.operation`
    AcknowledgedTargets    []string `json:"acknowledgedTargets"`   // must equal `wouldDestroy.targets` (set comparison)
    DiagnosedFailureClass  string `json:"diagnosedFailureClass,omitempty"`  // when LatestFailedAppVersionContext returned non-nil
}
```

**Validation contract**: handler runs `LatestFailedAppVersionContext` → builds `DiagnosedDestruction` → if input lacks matching `confirmDestructive`, returns `ErrDiagnosisRequired` with the structured `wouldDestroy` payload. Second call with matching `confirmDestructive` proceeds.

When `LatestFailedAppVersionContext` returns nil (target has no failed history), the gate is **bypassed** — destructive operations on healthy services don't need diagnosis. This is the empirical-priority discriminator: only services with failure history trigger the gate.

Pin: `TestDestructiveAck_ShapeRoundTrip`, `TestErrDiagnosisRequired_WireForm`.

**~80 LOC across errors.go, errwire.go, types.go.**

#### 3.2 — `zerops_import override=true` gate (the empirical case)

`internal/tools/import.go`: when input has `override: true` AND any override target has failed-appVersion history (via 1.3 helper), apply the 3.1 contract.

`ImportInput` gains `confirmDestructive *DestructiveAck`. Handler:
1. Resolve override targets, build `DiagnosedDestruction`
2. If any target has failed history AND input lacks matching `confirmDestructive` → return `ErrDiagnosisRequired`
3. Else proceed with override

Pin: `TestImport_OverrideOnFailedRequiresAck`, `_OverrideOnHealthyPasses`, `_AcknowledgedOverrideProceeds`, `_PartialAckRejected`.

**~70 LOC + tests.**

#### 3.3 — `zerops_dev_server` exec-not-shell guard (F5 polish)

Independent of the gate pattern — different validation path. `internal/ops/dev_server.go` start helper detects `^[A-Z_][A-Z0-9_]*=` shell prefix and refuses with Recovery suggesting `env KEY=VAL cmd` form. Tool description gets the explanation.

**~30 LOC + test.**

---

### Phase 4 — Verification + invariant pin

Re-run `recover-failed-buildfromgit-missing-dep` via `flow-eval.sh`. Binary success metric:

> **Agent reads `zerops_logs` (or `zerops_events`) at least once before any `zerops_import override=true confirmDestructive=...` call AND no un-acknowledged `override=true` is attempted.**

If passes: ship. If fails (agent treats `confirmDestructive` as a checkbox): tighten by requiring `diagnosedFailureClass` to match observed class (impossible to fake without reading events).

Cross-suite regression: re-run `greenfield-node-postgres-dev-stage`. No Recovery on healthy services, no `wouldDestroy` payload.

**CLAUDE.md invariant pin** (after the above passes):

> **Diagnose-before-destruct gates always-dangerous operations** — `zerops_import override=true` (and future destructive tools) refuse to mutate when target services have failed-appVersion history unless the call carries `confirmDestructive: {operation, acknowledgedTargets, diagnosedFailureClass?}` matching the structured `wouldDestroy` payload returned in the first-call rejection. Failure context surfaces lazily via `zerops_events` (`internal/ops/events.go::ClassifyDeployFailure` reuse path). Recovery hints on `verify::service_running`, `workflow_checks::checkServiceStatusAny`, `deploy` pre-flight, and `dev_server` pre-spawn point at the same gate. Lives on `ErrDiagnosisRequired` error code + `tools.DiagnosedDestruction` wire shape + `ops.LatestFailedAppVersionContext` helper. Pinned by `TestImport_OverrideOnFailedRequiresAck`, `TestCheckServiceStatusAny_ReadyToDeployWithFailedAppVersion_RecoveryToImport`, `TestLatestFailedAppVersionContext_*`.

**~5 LOC.**

---

## What this plan does NOT do (explicit out-of-scope)

Cut after the four-investigation synthesis:

- **Generalized "Read-Before-Write" middleware across all 23 tools** — Codex verdict: misclassifies the problem, adds overhead, doesn't change behavior.
- **Eager event-fetching in the envelope** — breaks P4 cheap-status invariant (Explore envelope audit).
- **New atom routing axis `failureClass`/`hasRecentFailure`** — adds ambiguity (cross-session boundary semantics), bloats envelope, redundant with lazy `zerops_events` (Explore envelope audit).
- **Failure context surfaced on `zerops_discover`** — agents don't use discover diagnostically (empirical baseline).
- **`Signals []string` on `ops.Recovery`** — speculative, no consumer.
- **Parallel `topology.ServiceLifecycleClassification` struct** — duplicates `topology.DeployFailureClassification`. Reuse instead.
- **STOPPED status as "non-running needing recovery"** — STOPPED is intentional state.
- **`ops.Manage` (restart/scale/storage) gating** — restart is often the recovery itself; scale/storage have no empirical evidence of destructive misuse.
- **Atom corpus consistency pass for the 12 read-before-act-missing atoms** — DEFERRED. Codex verdict: prose can't override LLM instruction-following when tool description presents override as recovery. The structural gate solves the empirical problem; atom polish is independent improvement that can wait for separate plan.
- **Speculative gates on `zerops_env set/delete/generate-dotenv`** — empirically uncovered; the 3.1 shared shape makes adding them a 30-LOC delta when evidence emerges.
- **`zerops_delete` structured acknowledgment** — current prose-level guard is empirically sufficient (no eval signal of mis-deletion).

These are **future-proof by design, not pre-implementation**. The 3.1 shared shape is the architectural foundation; specific gate additions follow empirical evidence.

---

## Sequencing — strict ordering, no parallelism shortcuts

1. **Phase 1.1 + 1.2** — content commits, no helper dependency (2 commits, ~25 LOC)
2. **Phase 1.3** — `LatestFailedAppVersionContext` helper (1 commit, ~80 LOC)
3. **Phase 1.4** — `workflow_checks` Recovery (consumes 1.3) (1 commit, ~80 LOC + golden sweeps)
4. **Phase 3.1** — shared destructive-ack shape (1 commit, ~80 LOC)
5. **Phase 3.2** — `zerops_import` gate (consumes 1.3 + 3.1) (1 commit, ~70 LOC)
6. **Phase 2.1** — `verify` Recovery (consumes 1.3) (1 commit, ~50 LOC)
7. **Phase 2.2** — `deploy` pre-flight (consumes 1.3) (1 commit, ~40 LOC)
8. **Phase 2.3** — `dev_server` pre-spawn (consumes 1.3) (1 commit, ~30 LOC)
9. **Phase 3.3** — `dev_server` exec-not-shell (independent) (1 commit, ~30 LOC)
10. **Phase 4** — re-run baseline + CLAUDE.md invariant (1 commit, ~5 LOC)

**Total: 10 commits, ~490 LOC.** Up from v3's 9/330 because 3.1 shared shape (+80) and 1.3 helper formalization (+80) are now first-class. Down from v2's 15/700 by cutting speculative scope.

Each phase has its own RED check; Phase 4's binary success metric is the final gate.

---

## Risks (delta from v3)

- **Shared `DiagnosedDestruction` wire shape forward-evolves with new destructive tools.** Mitigation: the shape is intentionally minimal (operation/targets/loss); per-tool extensions live in `Loss` field categories without breaking older clients.
- **`confirmDestructive` parameter could be treated as a checkbox by agents.** Phase 4's binary metric catches this. Escalation path: require `diagnosedFailureClass` exact match (impossible to fake without reading events).
- **Phase 1.3 helper adds one network call per pre-flight check.** Cost analysis: single `SearchAppVersions` per service in scope, ~50-200ms typical. Acceptable for verify/deploy/dev_server pre-flight (already multi-second operations). NOT acceptable for the envelope's compaction-recovery path — explicitly excluded from envelope plumbing per investigation findings.
- **Atom phase axis broadening (1.1) may need a fork.** Decide at edit time if body coupling is too develop-specific.

---

## Done definition

1. Phase 0 shipped (DONE).
2. Phases 1.1 + 1.2 atom + content updates committed.
3. Phase 1.3 helper + tests green.
4. Phase 1.4 `workflow_checks` Recovery wired through `StepCheck` → `CheckWire`.
5. Phase 3.1 shared shape + Phase 3.2 import gate ship.
6. Phases 2.1 + 2.2 + 2.3 Recovery surfaces ship.
7. Phase 3.3 dev_server polish ships.
8. Re-run baseline satisfies binary success metric.
9. Cross-suite regression confirms no false-positive Recovery / `wouldDestroy` on healthy services.
10. CLAUDE.md invariant added.
11. v4 plan + predecessors v1/v2/v3 moved to `plans/archive/`.

---

## Why v4 — the architectural contribution

v3 was correct in mechanism but framed as "lifecycle recovery" — making the core insight (structural gate, not prose) feel like one phase among three. v4 reframes around the **diagnose-before-destruct invariant** as the architectural through-line, with the v3 phases as the implementation.

The v4 contributions beyond v3:
- **3.1 shared shape** (`ErrDiagnosisRequired` + `DiagnosedDestruction` + `DestructiveAck`) — reusable across destructive tools, not bespoke to import
- **1.3 helper formalization** — `LatestFailedAppVersionContext` as named ops helper, not buried inline in workflow_checks
- **Explicit out-of-scope rationale** — what we deliberately did NOT do, with investigation citations

The investigations contributed:
- **Atom corpus survey** (Explore): read-before-act is tactical not foundational; 12 atoms missing prerequisites — but Codex correctly identified that prose can't override instruction-following at destructive surfaces. Atom polish is **separate plan, not v4 scope**.
- **Envelope failure-context audit** (Explore): failure context belongs in lazy Recovery hints + zerops_events, NOT envelope plumbing. Drove the cut of "failureClass routing axis" from v4.
- **Codex PAO architectural review**: keep narrow + add helper; structural gate not prose. Drove the 3.1 shared shape as the future-proofing mechanism.
- **Codex destructive-ops audit**: catalog of always-dangerous surfaces (`env set/delete/generate-dotenv` + `import override`); recommended shared `DIAGNOSIS_REQUIRED` + `confirmDestructive` shape that v4 implements.

The plan is bullet-proof against scope creep (explicit out-of-scope), future-proof against new destructive tools (shared shape), and grounded in empirical evidence (only adds gates where eval shows damage). It is NOT generalized PAO — that would be over-engineering. It is the smallest invariant that closes F1+F4+F6 from the empirical baseline while leaving a contract that future destructive tools plug into in 30 LOC each.
