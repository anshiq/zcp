# Plan: ZCP non-running-state recovery (supersedes 2026-05-04 FAILED-state plan)

**Status**: Phase A.3 + A.4 complete (`seed: settled` mode shipped; baseline scenario authored). Phase A baseline run captured. Phases B (audit) and prior-plan reasoning preserved. Phases C / D rewritten below to track empirical findings.

**Supersedes**: `plans/zcp-failed-state-recovery-2026-05-04.md` (the prior plan's framing was correct in spirit but wrong in scope — empirical baseline shows the agent's friction surface is broader than FAILED alone).

---

## What changed since 2026-05-04 — and why this plan exists

The 2026-05-04 plan was framed around "service is FAILED at MCP-call time". A baseline flow-eval run on 2026-05-05 (suite `20260505-115417`, scenario `recover-failed-buildfromgit-missing-dep`, retrospective at `eval/behavioral/runs/20260505-115417/recover-failed-buildfromgit-missing-dep/self-review.md`) reveals two corrections:

**1. The Tier-1 fixture does NOT produce FAILED**. `python-hello-world-app` via `buildFromGit` + missing `db` produces:
- Build phase: FINISHED (build doesn't reference `${db_*}`)
- Deploy/init phase: aborts when migrate.py reads unresolved `${db_hostname}`
- Service status: `READY_TO_DEPLOY` (built but never successfully deployed)
- Import parent process: CANCELED (parent aborted because child deploy failed)

**Service status FAILED is rare in practice**. The terminal "I can't progress" state agents actually encounter is `READY_TO_DEPLOY` with a failed appVersion event — built but never landed. Plain FAILED happens in narrower situations (e.g. STOPPED service that the platform downgraded, or post-stable runtime crash that exhausts restart budget).

**2. The agent's actual recovery friction is on a DIFFERENT axis** — not the FAILED-Recovery surface, but READY_TO_DEPLOY-in-bootstrap-adopt + diagnostic-instinct gaps:

| # | Friction | Surface | Severity |
|---|---|---|---|
| F1 | `bootstrap-adopt::provision` rejects READY_TO_DEPLOY with `expected one of [RUNNING, ACTIVE], got READY_TO_DEPLOY` and NO Recovery | `internal/tools/workflow_checks.go:179-184` | High — agent stalls until it digs through `zerops_import` tool description |
| F2 | Bootstrap-adopt provision-step guidance literally says *"Adopted services are typically already deployed, so this caveat doesn't apply"* — actively misleading for adopt-of-READY_TO_DEPLOY | `internal/content/atoms/bootstrap-provision-*.md` (the adopt-route provision atom) | High — text contradicts the case the agent is actually in |
| F3 | Existing atom `develop-ready-to-deploy.md` is keyed `phases: [develop-active]` only — never fires during `bootstrap-adopt` provision step | `internal/content/atoms/develop-ready-to-deploy.md:4` | Medium — recovery knowledge exists but not in the right phase |
| F4 | `verify`'s `service_running` check on READY_TO_DEPLOY emits raw `detail: "service status: READY_TO_DEPLOY"` with NO Recovery | `internal/ops/verify_checks.go:56-65` | Medium — first diagnostic surface is silent |
| F5 | `zerops_dev_server` `command: "PYTHONPATH=val cmd"` fails with confusing exec error because spawn uses `exec` directly, not a shell | `internal/ops/dev_server.go` (start helper) + tool description | Low — the agent recovered in one turn but it's a paper cut |
| F6 | Agent NEVER read runtime logs to diagnose root cause; instead scaffolded Flask hello-world from scratch and re-imported with `override=true`, destroying the original repo's code and yaml | systemic — multiple atoms guide toward "fix and redeploy" but on a non-running service the diagnostic instinct breaks | High — surface success, semantic failure |
| F7 | Bootstrap step responses dump huge `apiMeta` reference walls on every step (visible in transcript) | engine ergonomics — every `workflow action="complete"` response | Medium — noise crowds out the next-action signal |
| F8 | Adopt plan boilerplate (`isExisting: true`, `bootstrapMode: dev` for already-existing service) is undocumented | adopt-route plan schema | Low |
| F9 | `steps[close].status=skipped` reads as failure; it's normal for adopt | response wording | Low |

**The original plan's Phase C surface (Recovery on discover/deploy/dev_server/verify) covers F4 and is necessary but insufficient**. F1–F3 + F6 + F7 are the bigger wins and were not in the original plan's scope.

---

## What this plan does — three coordinated workstreams

### Workstream 1 — Recovery-on-non-running-terminal-status (extends prior Phase C)

**The structural fix.** The 2026-05-04 plan was right about the surface architecture — Recovery on every read-side surface where an agent first observes the broken state — but its category was too narrow. Generalize "service is FAILED" to "service is in a non-running terminal status" (`FAILED`, `READY_TO_DEPLOY`, `STOPPED`). Each has different recovery semantics; one classifier dispatches.

### Workstream 2 — Bootstrap-adopt phase coverage (closes F1–F3)

**The phase-routing fix.** `bootstrap-active::provision` is the phase where adopt-of-READY_TO_DEPLOY surfaces. Today the existing READY_TO_DEPLOY atom is develop-only, the adopt provision atom contradicts the user's situation, and `workflow_checks::checkServiceStatusAny` rejects without Recovery. Three fixes co-located on the bootstrap-adopt code path.

### Workstream 3 — Diagnostic-instinct + dev_server polish (mitigates F5–F9)

**The cognitive fix.** F6 is the most damaging — the agent's "I'll figure out what's broken" instinct is missing. Tier this carefully: not all of F6 is plan-scope (some is engine-level recipe-quality territory), but where atoms guide toward "fix and redeploy" they should also guide toward "read runtime logs FIRST when service is non-running". Plus narrow polish on F5/F7/F9.

---

## Workstream 1 — Recovery-on-non-running-terminal-status

### W1.1 — Topology vocabulary + classifier types (one commit)

Per `CLAUDE.md` layer-2 rule (`internal/topology/` is stdlib-only), the classification result struct lives in topology. Per Codex Q1, do **not** embed `*ops.Recovery` in topology — keep the wrapper at the ops layer.

**`internal/platform/types.go`**: add the missing service-status constants:
```go
ServiceStatusFailed  = "FAILED"
ServiceStatusStopped = "STOPPED"
```

**`internal/topology/predicates.go`**: add predicates
```go
// IsServiceTerminal returns true when the status names a state the service
// will not leave on its own (no in-flight platform operation).
func IsServiceTerminal(status string) bool

// IsServiceNonRunning returns true when status is terminal AND the service is
// not in a running state. Used by Recovery emitters to decide whether to
// classify the service as in need of agent attention.
func IsServiceNonRunning(status string) bool  // FAILED, READY_TO_DEPLOY, STOPPED
```

**`internal/topology/failure_class.go`**: add a sibling classification type for non-running services:
```go
type ServiceLifecycleClassification struct {
    Category        ServiceLifecycleCategory  // missing-dep / init-crash / build-failure / never-deployed / stopped-explicit / unknown
    LikelyCause     string
    SuggestedAction string
    Signals         []string
}
```

Pin: `TestIsServiceNonRunning_*`, `TestServiceLifecycleClassification_FieldShape`.

**Refactor target** (same commit): rename raw `"FAILED"` and `"STOPPED"` literals to constants. Sites:
- `internal/eval/cleanup.go:96-97` (terminalDeletableStatuses)
- `internal/eval/seed.go:89,103,104,105,106,107,109,110` (waitAllActive case + waitAllSettled isTerminalServiceStatus)
- `internal/ops/events.go:80` (statusFailed alias already exists; keep)
- `internal/tools/workflow_bootstrap.go:182` (status: "FAILED" literal)

RED: `grep -rn '"FAILED"' internal/ --include='*.go' | grep -v _test.go` returns zero hits. CANCELED is a process status, not service status — out of scope.

**Files**: 3 + ~5 refactor sites. ~80 LOC net.

### W1.2 — `Signals` field on Recovery types (one commit)

Per Codex missing #2: both `ops.Recovery` (`internal/ops/verify.go:36`) and `tools.RecoveryHint` (`internal/tools/errwire.go:65`) lack a `Signals` field. Add it to both — they remain independent types per the architecture rule (ops can't import tools), but their wire shapes converge.

```go
type Recovery struct {
    Tool    string            `json:"tool"`
    Action  string            `json:"action"`
    Args    map[string]string `json:"args,omitempty"`
    Signals []string          `json:"signals,omitempty"`  // new
}
```

Mirror in `tools.RecoveryHint`. The conversion path in `convertError` already iterates fields — extend.

Pin: `TestRecovery_SignalsRoundTrip`, existing `TestErrorWire_*` tests stay green.

**Files**: 2. ~10 LOC net.

### W1.3 — Lifecycle classifier (one commit)

**New file** `internal/ops/lifecycle_classifier.go`:

```go
// ClassifyServiceLifecycle inspects a non-running service and returns a
// ServiceLifecycleClassification + Recovery pointing at the next action.
// Mirrors the design of ClassifyDeployFailure in deploy_failure.go.
//
// Inputs:
//   - svc: the service stack with non-running status
//   - recentEvents: recent appVersion events (already-fetched from caller — see ops/events.go)
//   - logFetcher: optional; when non-nil, classifier may fetch a runtime-logs
//     tail to refine classification
//
// Output: classification + Recovery. Recovery is always non-nil — fallback
// points the agent at zerops_logs to investigate when no signal matches.
func ClassifyServiceLifecycle(
    ctx context.Context,
    svc *platform.ServiceStack,
    recentEvents []platform.AppVersionEvent,
    logFetcher platform.LogFetcher,
) (*topology.ServiceLifecycleClassification, *Recovery)
```

**Reuse, don't rebuild** (per Codex missing #3): when the most recent appVersion event has a failure status (`BUILD_FAILED` / `PREPARING_RUNTIME_FAILED` / `DEPLOY_FAILED`), call into the existing `ClassifyDeployFailure` (`internal/ops/deploy_failure.go`) and convert its output to `ServiceLifecycleClassification`. New regex pattern library only fires when there is NO recent failed appVersion (e.g. service was STOPPED manually, or was never deployed, or has a runtime crash post-stable).

Pattern library categories:
- `missing-dep`: log shows `connection refused.*5432|6379|3306`, env var keys reference unresolved `${dep_*}`. Recovery → `zerops_import` to add the dep.
- `init-crash`: appVersion `DEPLOY_FAILED` + runtime logs. Recovery → `zerops_logs facility=application since=15m` then `zerops_deploy` after fix.
- `build-failure`: appVersion `BUILD_FAILED`. Recovery → `zerops_logs facility=build since=15m`.
- `never-deployed`: status=READY_TO_DEPLOY + no appVersion events. Recovery → `zerops_import` with `startWithoutCode: true` + `override: true` (the documented escape hatch from F1).
- `stopped-explicit`: status=STOPPED + no recent failed events. Recovery → `zerops_manage action=start`.
- `unknown` (fallback): Recovery → `zerops_logs since=30m`.

**Fixture capture** (per Codex Q2 with note): the deploy_failure_test.go pattern uses INLINE samples, not testdata files. Mirror that — embed sanitized log tails directly in `lifecycle_classifier_test.go`. Capture the samples once via a one-shot manual probe against `python-simple-failed-no-db.yaml` in eval-zcp before writing the regex set; commit the resulting test cases inline.

Pin: `TestClassifyServiceLifecycle_MissingDep`, `_InitCrash`, `_NeverDeployed`, `_StoppedExplicit`, `_Unknown`, `_PassesThroughClassifyDeployFailureWhenAppVersionFailed`.

**Files**: 1 + tests. ~250 LOC.

### W1.4 — Plumb classifier through `discover` (one commit)

Per Codex missing #1: `Discover` and `RegisterDiscover` currently lack `platform.LogFetcher`. Either:
- (A) accept the signature ripple — extend both
- (B) defer log-tail enrichment to a follow-up tool call by emitting Recovery without log signals

**Decision: (A)**. The ripple is bounded (one signature change in `ops/discover.go`, one in `tools/discover.go`'s registration) and Recovery without log signals is significantly weaker (no `missing-dep` classification possible). Codex flagged this is a real ripple — accept it now or eat it later.

Changes:
1. `ops.Discover` signature gains `logFetcher platform.LogFetcher`.
2. `tools/discover.go::RegisterDiscover` gains a logFetcher parameter; `cmd/zcp/serve.go` and any test wiring updated.
3. `ops.Discover` for each non-system service in non-running terminal status: classify via W1.3, attach `Recovery` to the `ServiceInfo`.
4. New field on `ServiceInfo`:
```go
Recovery *Recovery `json:"recovery,omitempty"`
```
5. The list-mode and single-service-mode paths both populate.

Pin: `TestDiscover_PopulatesRecoveryOnFailedService`, `TestDiscover_PopulatesRecoveryOnReadyToDeploy`, `TestDiscover_NoRecoveryOnHealthy`.

**Files**: 3 + tests. ~80 LOC net (mostly signature plumbing).

### W1.5 — Plumb classifier through `verify::checkServiceRunning` (one commit)

`internal/ops/verify_checks.go:56-65`: when status is non-running terminal, call W1.3 classifier and attach Recovery to the CheckResult.

**Important** (audit's existing point at `verify.go:118`): the `if runningCheck.Status != CheckPass { ... return }` short-circuit silently discards subdomain Recovery. Two fixes are independent:
- attach Recovery to the failing service_running check (W1.5 scope)
- preserve subdomain Recovery emission even when service_running fails (separate, but ride along since the diff is small)

Pin: `TestCheckServiceRunning_FailedAttachesRecovery`, `TestCheckServiceRunning_ReadyToDeployAttachesRecovery`, `TestVerify_PreservesSubdomainRecoveryWhenServiceNotRunning`.

**Files**: 2. ~30 LOC net.

### W1.6 — Plumb classifier through `workflow_checks::checkServiceStatusAny` (one commit)

`internal/tools/workflow_checks.go:163-184`: the generic late-stage status check returns `expected one of [...], got X` with no Recovery. This is what the agent hit at F1 (provision step rejection).

When the failing status is non-running terminal AND the caller passed the LogFetcher (extend `checkServiceStatusAny` signature, ripple to all callers — it's currently called from a small set of places), classify and attach Recovery to the StepCheck.

The Recovery propagates to the agent via `tools.WithChecks` (already plumbed via `internal/tools/errwire.go::CheckWire` — note: CheckWire today does NOT carry Recovery; add `Recovery *RecoveryHint` field).

Pin: `TestCheckServiceStatusAny_ReadyToDeployAttachesRecovery`, `TestWorkflowChecks_RecoveryReachesErrorWire`.

**Files**: 3 + tests. ~50 LOC net.

### W1.7 — Defence-in-depth gates (one commit per gate, total 4)

Per Codex Q3 (qualified yes), fold `ops.Manage` into Workstream 1 here.

**W1.7a — `zerops_deploy` (SSH + local)**: pre-deploy check — fetch service status; if READY_TO_DEPLOY-with-failed-appVersion or FAILED, return ErrorWire with classification Recovery (preventing wasted deploy cycle). If READY_TO_DEPLOY-never-deployed, this is the *normal* first-deploy path; let it through. Discriminator: `len(failedAppVersionEvents) > 0`.

**W1.7b — `zerops_dev_server`**: pre-spawn check — same pattern. FAILED service rejects; READY_TO_DEPLOY-never-deployed rejects with Recovery pointing at `zerops_deploy`.

**W1.7c — `zerops_workflow develop start` + `zerops_workflow bootstrap start`** (per Codex Q6): gate at the **tool layer** (`internal/tools/workflow_route.go`, `internal/tools/workflow_develop.go`), NOT at `internal/workflow/route.go` (peer-layer rule — workflow can't import ops). When live topology contains non-running terminal services in the would-be scope, attach `nonRunningServices: [{hostname, recovery}]` to the start response.

**W1.7d — `ops.Manage` (per Codex Q3)**: 
- `Restart` and `Reload` ungated — restart can be the recovery itself
- `Scale`, `ConnectStorage`, `DisconnectStorage`: pre-mutate fetch + classify; FAILED/READY_TO_DEPLOY/STOPPED → Recovery, refuse to mutate
- `Stop` ungated — stopping a FAILED service is fine

Pin: per gate, e.g. `TestDeploySSH_FAILEDServiceReturnsRecovery`, `TestManageScale_NonRunningRefuses`, `TestWorkflowDevelop_AttachesNonRunningRecovery`.

**Files**: 6-8 across the four sub-commits. ~150 LOC.

---

## Workstream 2 — Bootstrap-adopt phase coverage

### W2.1 — `develop-ready-to-deploy.md` axis broadening (one commit, content-only)

The atom at `internal/content/atoms/develop-ready-to-deploy.md` already describes the exact recovery the agent eventually figured out manually (re-import with `startWithoutCode: true + override: true`). Broaden its phase axis:
```yaml
phases: [develop-active, bootstrap-active]
```

`Synthesize` already supports multi-phase atoms. The atom body already references `zerops_import override=true` — minor wording tweak so the bootstrap-adopt phase reads naturally.

Pin: re-render envelope tests for bootstrap-adopt-with-READY_TO_DEPLOY scenarios MUST include this atom in the rendered guidance. Add a new pinned-by-scenario test entry.

**Files**: 1 atom + 1 test. ~10 LOC.

### W2.2 — Bootstrap-adopt provision-step guidance correction (one commit, content-only)

The misleading text *"Adopted services are typically already deployed, so this caveat doesn't apply on the adopt route"* lives in (most likely) `internal/content/workflows/bootstrap/adopt-provision*.md` or the bootstrap-adopt provision atom. Replace with a status-aware fragment:

> Adopted services are usually ACTIVE; if `zerops_discover` shows status=READY_TO_DEPLOY, the service was created without `startWithoutCode: true`. Re-import with `startWithoutCode: true + override: true` — this REPLACES the service stack (back up code first). See `develop-ready-to-deploy` atom for the full procedure.

Pin: render-test asserting the new text fires in adopt-route provision phase.

**Files**: 1 content file + 1 test. ~15 LOC.

### W2.3 — Provision-step status-check Recovery (covered by W1.6)

Already in W1.6 — when `checkServiceStatusAny` rejects READY_TO_DEPLOY, Recovery points at `zerops_import` with `startWithoutCode + override`. The combination of W1.6 + W2.1 + W2.2 closes F1+F2+F3 fully.

---

## Workstream 3 — Diagnostic-instinct + dev_server polish

### W3.1 — Atom: read-runtime-logs-on-non-running (one commit, content-only)

The empirical baseline shows the agent NEVER read runtime logs to diagnose. Today the atom corpus has logs-related guidance only inside specific failure modes (build failure → buildLogs, deploy failure → runtimeLogs). There is no atom that says "service is non-running → read runtime logs FIRST".

New atom `internal/content/atoms/diagnose-non-running-read-logs.md`:
- `serviceStatus: [FAILED, READY_TO_DEPLOY, STOPPED]` (uses W1.1 constants)
- `phases: [bootstrap-active, develop-active]`
- Body: 3-5 lines. TRIGGER: service in non-running terminal status. ACTION: `zerops_logs serviceHostname=<host> facility=application since=30m severity=ERROR` BEFORE deploying or rebuilding. FAILURE MODE: scaffolding over the broken state without reading logs leads to lost original code (cite: empirical eval `recover-failed-buildfromgit-missing-dep` 2026-05-05).

Pin: scenario re-run shows logs called before `zerops_import override=true`.

**Files**: 1 atom + 1 scenarios_test entry. ~10 LOC + atom body.

### W3.2 — `zerops_dev_server` exec-not-shell guard (one commit)

`internal/ops/dev_server.go` (start helper): detect `command` strings starting with `KEY=VAL ...` (regex `^[A-Z_][A-Z0-9_]*=`) — these are shell prefix env-var assignments that fail under `exec` spawn. Either:
- (A) refuse with structured error pointing at `env KEY=VAL cmd` form
- (B) auto-rewrite (less explicit, more magic)

**Decision: (A)** — explicit refusal with Recovery `{tool: zerops_dev_server, action: start, args: {command: "env KEY=VAL <orig>"}}`. Mirrors the F5 fix path the agent eventually used.

Tool description gets a sentence: *"command runs via `exec` not a shell — for env-var prefixes use `env KEY=VAL cmd`, not `KEY=VAL cmd`."*

Pin: `TestDevServer_RejectsShellEnvPrefix_SuggestsEnvForm`.

**Files**: 2 + test. ~30 LOC.

### W3.3 — F7/F9 (apiMeta wall + skipped wording) — DEFERRED

These are engine-ergonomics issues with broad blast radius; the friction is real but bounded. Triage:
- **F7 (apiMeta walls)**: punt to a separate plan — the response-shape redesign affects every workflow tool, not just bootstrap-adopt. Backlog: `plans/backlog/apimeta-response-noise.md`.
- **F9 (skipped wording)**: rename `skipped` → `not-applicable` in close-step responses for adopt route. Tiny patch but touches snapshot tests across the workflow package — backlog as a one-bullet item.

### W3.4 — F8 (adopt boilerplate docs) — RECIPE SCOPE

`isExisting: true` + `bootstrapMode: dev` for adopt-of-existing belongs in adopt-route plan-shape documentation. This intersects with Aleš's recipe scope per `CLAUDE.local.md`. Surface as plain plan-doc note, do not act.

---

## Phase A — already shipped

`seed: settled` mode + `recover-failed-buildfromgit-missing-dep.md` scenario landed 2026-05-05 (commit pending). Both pollProcessSettled (FINISHED/FAILED/CANCELED tolerance) and waitAllSettled (terminal-status loop) work end-to-end. The scenario is reusable as the verification harness for Workstream 1+2+3.

---

## Out of scope (unchanged from prior plan, with two additions)

- Recovery for `BUILDING` / `DEPLOYING` / any transitional status.
- Auto-healing via timer-driven retries.
- Tier-3 mid-life crash coverage (no platform API to force healthy → FAILED).
- Recipe-as-scaffold tool (`plans/backlog/recipe-scaffold-tool.md`).
- **NEW: F7 apiMeta-wall response redesign** (`plans/backlog/apimeta-response-noise.md`, to be created).
- **NEW: `zerops_env` auto-restart on non-ACTIVE** (per Codex missing #4 — env-fix recovery dead-ends on FAILED/READY_TO_DEPLOY because `ops.Env::AutoRestart` only restarts ACTIVE runtimes; record as backlog `plans/backlog/env-auto-restart-non-running.md` and let agent's flow drive whether to lift).

---

## Cross-workstream verification

After Workstream 1 + 2 land:
- Re-run `recover-failed-buildfromgit-missing-dep` via `flow-eval.sh`. New self-review MUST show:
  - `zerops_discover` returned with `recovery` populated on the api service
  - `zerops_workflow start bootstrap route=adopt` provision-step rejection carried Recovery (not just `expected one of [...], got READY_TO_DEPLOY`)
  - Either the agent followed the Recovery directly OR (if it still scaffolds-over) reads runtime logs first per W3.1

After Workstream 3 lands:
- Same scenario re-run. Self-review MUST mention runtime logs BEFORE any `override=true` import.
- `zerops_dev_server` `PYTHONPATH=val cmd` invocation returns Recovery suggesting `env KEY=VAL` form.

Cross-suite regression: re-run a healthy scenario (`greenfield-node-postgres-dev-stage`). NO Recovery field on healthy services — confirms the populate-only-on-non-running rule.

---

## CLAUDE.md invariant pin (after both workstreams land)

Add to Conventions section:

> **Non-running terminal services carry structured Recovery on every read-side surface** — `zerops_discover` populates `recovery` on each service with `topology.IsServiceNonRunning(status)`, and `zerops_verify`'s `service_running` check + `zerops_workflow` provision-step status check + `zerops_deploy`/`zerops_dev_server`/`ops.Manage` mutations all gate through the same `ops.ClassifyServiceLifecycle` classifier. Lives on `ops.DiscoverResult.<Per-Service>.Recovery`, `ops.CheckResult.Recovery`, `tools.CheckWire.Recovery`, and the canonical `ops.Recovery` type (parallel `tools.RecoveryHint`). Reuses `ClassifyDeployFailure` when the most recent appVersion failed. Pinned by `TestDiscover_PopulatesRecoveryOnFailedService`, `TestCheckServiceStatusAny_ReadyToDeployAttachesRecovery`, `TestClassifyServiceLifecycle_*`, atom `diagnose-non-running-read-logs`.

---

## Sequencing — natural commit boundaries

Codex Q7 confirms: split into structural and defence pairs.

**Structural (Workstream 1.1–1.5)** — five commits, ~430 LOC:
1. W1.1 — topology constants + classification struct + literal refactor
2. W1.2 — Signals on Recovery types
3. W1.3 — ClassifyServiceLifecycle classifier
4. W1.4 — discover plumbing (signature ripple)
5. W1.5 — verify plumbing

**Defence (Workstream 1.6–1.7)** — five commits, ~200 LOC:
6. W1.6 — workflow_checks plumbing
7. W1.7a — deploy gating
8. W1.7b — dev_server gating
9. W1.7c — workflow start gating
10. W1.7d — manage gating

**Phase coverage (Workstream 2)** — two commits, ~25 LOC:
11. W2.1 — atom phase axis broadening
12. W2.2 — provision-step guidance correction

**Diagnostic + polish (Workstream 3)** — two commits, ~40 LOC:
13. W3.1 — diagnose-non-running atom
14. W3.2 — dev_server exec-not-shell guard

**Closing (one commit)**:
15. CLAUDE.md invariant pin + scenario re-run + plan move to archive.

Total: ~15 commits, ~700 LOC, 1.5–2 days of concentrated work + ~1 hour of flow-eval verification.

---

## Risks (delta from prior plan)

- **Discover signature ripple is real**. `ops.Discover` is called from MCP handler, CLI subcommands, eval scaffolding, and the workspace-manifest tool. Tests exist for each. Acceptable but plan ~30 min for adjusting test wirings.
- **`ClassifyDeployFailure` reuse loop**. `lifecycle_classifier` calls into `deploy_failure` when an appVersion is failed. Risk: pattern library divergence — same regex re-implemented for the lifecycle case. Mitigation: lifecycle-classifier tests assert that the deploy-failure-reuse path produces the same Category/Signals as direct `ClassifyDeployFailure` invocation.
- **Empirical baseline scenario may shift**. After W1+W2 land, the agent might still scaffold-over instead of reading logs (W3.1 atom may not be enough — agent's instinct is strong). If the second baseline run still shows the F6 pattern, escalate W3.1 to a higher-priority cross-atom edit (e.g. modifying the `bootstrap-adopt-route-overview` atom to gate on non-running status before suggesting any plan).
- **`develop-ready-to-deploy.md` was authored as a develop-phase recovery; broadening to bootstrap may surface assumption coupling**. The atom body references `develop` workflow concepts implicitly. W2.1 needs a careful re-read pass; if coupling is deep, fork into a second atom rather than over-broadening.

---

## Done definition

This plan is complete when:

1. `seed: settled` mode shipped (DONE 2026-05-05)
2. `recover-failed-buildfromgit-missing-dep` scenario authored + run baseline captured (DONE 2026-05-05)
3. Workstream 1 (W1.1–W1.7) shipped with all pinning tests green
4. Workstream 2 (W2.1–W2.2) shipped with re-render tests green
5. Workstream 3 (W3.1–W3.2) shipped
6. Re-run of `recover-failed-buildfromgit-missing-dep`'s self-review shows: Recovery field consumed, runtime logs read, no destructive override-rebuild
7. CLAUDE.md invariant bullet added
8. `plans/zcp-non-running-state-recovery-2026-05-05.md` moved to `plans/archive/`
9. Prior `plans/zcp-failed-state-recovery-2026-05-04.md` moved to `plans/archive/` referencing this plan as supersession

---

## Why this plan, not the prior one

The 2026-05-04 plan was correct in mechanism and wrong in scope. Its surface architecture (Recovery on read-side surfaces, classifier in ops, atom routing on serviceStatus) is preserved here verbatim — Workstream 1 is the prior Phase C with broadened category. Its analysis of audit gaps remains the foundation of W1.4–W1.7.

The corrections come from three sources:

1. **Empirical baseline** showed the FAILED frame was too narrow; READY_TO_DEPLOY with failed appVersion is the more common case, and bootstrap-adopt provision is the friction phase. → Workstreams 2 and 3.

2. **Codex independent review** caught three architectural details: layer rule (topology stdlib-only), `ops.Discover` lacks LogFetcher, `Signals` field missing on Recovery types, reuse of existing `ClassifyDeployFailure`, gate placement at tool-layer not workflow-layer. → W1.1, W1.2, W1.3 reuse loop, W1.4, W1.7c.

3. **My second-pass re-audit** caught `ops.Manage` gap, the verify→subdomain short-circuit gap, and the discover single-vs-list-mode parity gap. → W1.5, W1.7d.

The prior plan's three-phase A/B/C structure is preserved as Phase A (done) + Workstreams 1-3. Naming changed from "phases" to "workstreams" because the prior phase ordering implied A → B → C dependency; the workstreams here can ship in parallel after W1.1 lands (1.1 is the foundational vocabulary).
