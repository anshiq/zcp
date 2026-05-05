# Plan: ZCP FAILED-state recovery — taxonomy, eval coverage, recovery surface

**Status**: Phase B (audit) COMPLETE 2026-05-04. Phases A and C are open.

**Already in repo at this plan's commit time** (do not duplicate):
- `eval/behavioral/audits/failed-state-zcp-audit-2026-05-04.md` — Codex audit, the source of Phase B's verdict.
- `eval/behavioral/scenarios/fixtures/python-simple-failed-no-db.yaml` — Tier-1 broken fixture (buildFromGit + no db). Pairs with the as-yet-unauthored scenario `recover-failed-buildfromgit-missing-dep.md` once `seed: settled` ships.
- `eval/behavioral/scenarios/fixtures/python-simple-deployed.yaml` — sibling working fixture (same repo + db) for delta comparison.
- `eval/behavioral/scenarios/existing-simple-mode-add-endpoint.md` — DEFERRED scenario marker; un-defer once Phase C lands.
- `plans/backlog/recipe-scaffold-tool.md` — adjacent backlog from the same investigation; not part of this plan but cross-referenced.
**Surfaced**: 2026-05-04 — `existing-simple-mode-add-endpoint` flow-eval scenario reliably crashed at seed because its `buildFromGit` runtime hard-references `${db_hostname}` env vars and the fixture omitted the `db` service. The failure mode is generic — any path that produces a Zerops service in terminal FAILED status (failed provisioning, failed first deploy, runtime crash mid-life) puts the agent in a state the current ZCP recovery surface does not coherently address. `ops.DeployResult.FailureClassification` covers failures detected DURING an agent's deploy attempt; `ops.CheckResult.Recovery` covers actionable preconditions discovered DURING verify. Neither surface fires when the agent encounters a service that was already FAILED at MCP-call time.

This plan is self-contained — references `CLAUDE.md`, `CLAUDE.local.md`, `eval/behavioral/README.md`, `docs/spec-workflows.md`, `internal/platform/errors.go`, and the source tree.

---

## How an LLM implementer should approach this plan

1. **Read top-to-bottom before starting any phase.** Phases ARE NOT independent here: Phase A (taxonomy + eval coverage) anchors what Phase C (recovery surface design) is trying to address. Phase B (audit) feeds Phase C with the gap inventory.
2. **TDD per `CLAUDE.md`.** Each phase has a RED check (what should fail before), GREEN (after), slow-loop (re-run targeted scenarios).
3. **Phase order**: A → B → C. Phase A sets up observability (eval can produce FAILED state on demand); Phase B audits ZCP's existing handling; Phase C lands the recovery surface that closes the gaps.
4. **No premature plumbing.** If Phase B finds the existing surfaces (failureClassification / Recovery) already cover most of FAILED-state recovery and the gap is just routing/atom guidance, Phase C is a content-only plan, NOT a new tool/handler. Decide AFTER audit.
5. **Pause points**: end of Phase A (commit eval extension + scenarios), end of Phase B (commit audit notes), end of Phase C (recovery surface ships).

---

## Why now

The flow-eval suite `20260504-104436` exposed two artefacts pointing to the same underlying gap:

1. **`existing-simple-mode-add-endpoint` could not seed.** Its fixture (a single Python service with `buildFromGit` to `python-hello-world-app`) required a `db` service the fixture omitted; the runtime crashed in `initCommands` with `${db_hostname}` unresolved, the container ended in FAILED, and the eval runner aborted with `process … failed: unknown`. The fixture has been corrected to add `db: postgresql@18`, but the scenario remains DEFERRED because we don't yet have a generic story for "agent encounters a service already in FAILED state".
2. **Recovery friction in flow-eval suite was generic, not just about subdomains.** `verify-subdomain-recovery-before-browser` exercises one specific recovery (subdomain disabled). Extending coverage to FAILED state — the most common terminal failure mode in real Zerops projects — is the next step on the same axis.

This is also a real-user-impact issue, not just an eval-infra one: any user who imports a recipe, hits a `${dep_*}` mismatch, or pushes a broken zerops.yaml can produce a FAILED service. Today the agent's flow when it discovers one is undefined.

---

## What is NOT in scope

- Recovering `BUILDING`, `DEPLOYING`, or any other transitional state. This plan addresses the terminal FAILED case only.
- Auto-healing via timer-driven retries. Recovery is agent-driven, surfaced through tools/atoms.
- Mid-life failures from managed-dep crashes, OOMs, or platform incidents (failure modes Tier-3 below). Tier-1 + Tier-2 first.
- A separate failure surface for `STOPPED` services. STOPPED is intentional, FAILED is not — different semantic class.
- Recipe-as-scaffold tool. That's a separate backlog (`plans/backlog/recipe-scaffold-tool.md`).

---

## Phase A — failure-mode taxonomy + eval coverage

### A.1 — taxonomy

The "service is FAILED" state has two orthogonal axes:

**WHEN** failure happened:

| Tier | When | Notes |
|---|---|---|
| T0a | provisioning (import / buildFromGit clone-and-build) | Build container failed before any runtime container started. Surfaces as `process … failed: unknown` from `pollProcess` today. |
| T0b | first deploy after provisioning | Build OK, runtime container started, runtime exited non-zero or healthcheck failed. Service status reaches FAILED. |
| T1 | agent-driven first deploy | `zerops_deploy` at agent's request fails. Today this populates `ops.DeployResult.FailureClassification`. **Already covered**, included for taxonomy completeness. |
| T2 | agent-driven redeploy of healthy service | Service was ACTIVE; new deploy fails; rollback or stuck depending on platform. |
| T3 | post-stable mid-life crash | Service was ACTIVE for hours/days; runtime crashed for OOM, dep died, or platform incident. **Tier-3, deferred.** |

**WHY** failure happened:

| Code | Cause | Detection signal |
|---|---|---|
| W-A | Missing managed dep referenced by repo's zerops.yaml (`${db_hostname}` unresolved) | Runtime logs: connection-refused or env-var-empty |
| W-B | Invalid zerops.yaml syntax / schema | Build event: yaml parser error |
| W-C | Build commands failed (compile / npm-install error) | Build logs |
| W-D | Init command exited non-zero | `failureClassification.likelyCause = "init"` for fresh attempts; runtime logs for already-FAILED |
| W-E | Runtime start command failed | Runtime logs |
| W-F | Healthcheck timeout / port mismatch | Runtime logs + healthcheck event |

T0a × {W-B, W-C} = build-time failure (no runtime container ever ran).
T0b × {W-A, W-D, W-E, W-F} = post-build runtime failure (runtime container crashed or never went healthy).

### A.2 — eval scenarios (Tier 1 first)

| Scenario ID | Tier × Cause | Fixture requirement | Status |
|---|---|---|---|
| `recover-failed-buildfromgit-missing-dep` | T0b × W-A | `python-hello-world-app` buildFromGit + NO `db` service (deliberately broken) | **Author** |
| `recover-failed-init-command-crash` | T0b × W-D | Custom one-shot test repo with `initCommands: ["false"]` or similar deliberately-failing init; OR an existing recipe with an init that fails when env not present | **Defer until Phase B identifies whether this is needed** |
| `recover-failed-build-bad-yaml` | T0a × W-B | A custom repo with intentionally malformed zerops.yaml | **Defer** |

Tier-1 priority is **`recover-failed-buildfromgit-missing-dep`**: highest real-world frequency, simplest fixture (just the python-simple-deployed.yaml MINUS db), and exercises the full agent flow (discover sees FAILED → logs reveal cause → agent fixes by adding the dep OR re-importing without the broken reference OR redeploying with adjusted env).

### A.3 — eval-runner extension

Today's seed modes (`empty`, `imported`, `deployed`) cannot produce a deterministic FAILED-settled state:
- `imported` polls only the import processes; runtime crash AFTER provisioning is missed; eval starts before the FAILED status materialises.
- `deployed` aborts on FAILED via `waitAllActive`'s `case "FAILED"` arm.

Add a new seed mode:

```go
// internal/eval/scenario.go
const ModeSettled SeedMode = "settled"

// internal/eval/seed.go
// SeedSettled imports + waits for all services to reach a non-transitional
// status (ACTIVE, FAILED, STOPPED, READY_TO_DEPLOY). Unlike SeedDeployed it
// does NOT abort on FAILED — that's the explicit case behavioural failure
// scenarios want to seed.
func SeedSettled(ctx context.Context, client platform.Client, projectID, fixturePath, workDir, suiteID string) error
```

Implementation outline:

1. Reuse `SeedImported` for the import + import-process polling.
2. After import, poll `ListProjectServices` until every non-system service is in `terminalStatuses` (define as: ACTIVE, FAILED, READY_TO_DEPLOY, STOPPED — i.e. anything not BUILDING/DEPLOYING/CREATING/etc).
3. Return summary: `map[hostname]status` so the runner can decide whether the scenario's expected starting state was reached.
4. Wire into `RunBehavioralScenario` and `RunScenario` `seedScenario` switch.
5. Optional: in scenario YAML add `expectStatus: {api: FAILED, db: ACTIVE}` — runner asserts before agent spawn, fails fast if seed didn't produce expected state.

Files affected: `internal/eval/scenario.go`, `internal/eval/seed.go`, `internal/eval/scenario_run.go`, `internal/eval/behavioral_run.go`. ~50 lines net.

### A.4 — Phase A deliverables

1. **Already done**: fixture `eval/behavioral/scenarios/fixtures/python-simple-failed-no-db.yaml` (buildFromGit + no db, intentionally produces FAILED). Header doc explains the design.
2. **TODO**: new seed mode `settled` shipped + tested. See A.3 for sketch (~50 lines net across `internal/eval/scenario.go`, `internal/eval/seed.go`, `internal/eval/scenario_run.go`, `internal/eval/behavioral_run.go`).
3. **TODO**: scenario `eval/behavioral/scenarios/recover-failed-buildfromgit-missing-dep.md` authored. Seed `settled`, fixture pointer to the file in (1). Prompt: "The api service is failing. Figure out why and fix it." Persona: developer who knows the service should be running but doesn't yet know why it isn't.
4. **TODO**: scenario runs end-to-end via `flow-eval.sh`: seed produces FAILED service, agent gets prompt, retrospective captures lived friction.
5. **TODO**: run captured at `eval/behavioral/runs/<suiteId>/recover-failed-buildfromgit-missing-dep/self-review.md` — this becomes the BASELINE for Phase C verification.

Note on (3): `Scenario.validate()` at `internal/eval/scenario.go` will reject `seed: settled` until A.3 lands — author scenario in the same commit/PR as the runner extension, not before.

---

## Phase B — ZCP audit: what fires today on a FAILED-state service

**Status**: COMPLETE (2026-05-04). Full audit notes: `eval/behavioral/audits/failed-state-zcp-audit-2026-05-04.md`.

### Summary verdict

ZCP today has **no recovery surface for services already in terminal FAILED status**. The only failure surface is DURING an active deploy this MCP call initiated. Agents that encounter a pre-existing FAILED service are silently routed through normal flow until something downstream errors out without structured Recovery. Codex's recommendation is a centralised fix, not scattered per-tool branches — see Phase C below.

### What works today

- `pollDeployBuild` populates `failureClassification` for failures during an active deploy (`internal/tools/deploy_poll.go:76,97,113` + `internal/ops/deploy_failure.go:38`).
- `record-deploy` blocks on failed latest appVersion with actionable text (`internal/tools/workflow_record_deploy.go:232`).
- Reusable Recovery DTO exists at `internal/ops/verify.go:36`, used today only by subdomain-enable verify check (`internal/ops/verify.go:185`).
- **Atom routing already supports `serviceStatuses` axis** (`internal/workflow/synthesize.go:398`); precedent atom `develop-ready-to-deploy.md`. Adding a FAILED-routed atom is content-only.

### What's missing

- `zerops_discover` / `ListProjectServices` (`internal/platform/zerops_mappers.go:112`) — FAILED copy-passed as raw string, no Recovery, no flag.
- `zerops_verify` `service_running` check (`internal/ops/verify_checks.go:56,198`) — emits FAILED + unhealthy but no Recovery; subdomain Recovery skipped when service_running fails early (`internal/ops/verify.go:118`).
- No `serviceStatus: [FAILED]` atom in corpus.
- No `ServiceStatusFailed` constant in `internal/platform/types.go:119` — FAILED matched as raw string at every call site.

### What's silently broken

- `zerops_deploy` proceeds normally on a pre-existing FAILED service (both SSH `internal/tools/deploy_ssh.go:139,207` and local `internal/ops/deploy_local.go:81,149` paths). Prior failure does NOT populate `failureClassification` — comment at `internal/ops/deploy_common.go:44` confirms.
- `zerops_dev_server` (`internal/tools/dev_server.go:104` → `internal/ops/dev_server.go:180,247`) immediately SSH-probes a FAILED runtime, returns unstructured probe errors.
- Develop workflow opens on FAILED topology — `liveHostnames` built from names only (`internal/tools/workflow_develop.go:87`), scope validation checks runtime metas not status (`internal/tools/workflow_develop.go:276`).
- Bootstrap route discovery filters on system/managed/meta/self — not status (`internal/workflow/route.go:285`).
- Generic late surface at `internal/tools/workflow_checks.go:179` returns `expected one of [...], got FAILED` with no Recovery.

### Verdict on Phase C surface

Codex picks **Option 1 (Recovery on discover) PRIMARY + thin slice of Option 3 (deploy / dev_server / workflow gate) for defence in depth + new atom for engine routing**. This matches the weak prior in the original Phase C sketch and tightens the surface allocation. Phase C below is rewritten accordingly.

---

## Phase C — Recovery surface design (post-audit, locked)

Six sub-phases, ordered by dependency. C.1 is foundational; C.2 produces the artifact other sub-phases consume; C.3-C.6 plumb it through surfaces. Each sub-phase is its own commit.

### C.1 — Platform constant + topology predicate

- `internal/platform/types.go:119` — add `ServiceStatusFailed = "FAILED"` beside existing constants.
- `internal/topology/` — add `IsFailed(status string) bool` predicate (zero non-stdlib imports per layer-2 rule). Sibling: optionally `IsTerminal(status string) bool` returning true for `{ACTIVE, FAILED, READY_TO_DEPLOY, STOPPED}` — useful for the future `seed: settled` runner extension (Phase A.3) and for `cleanup.go`'s existing `terminalDeletableStatuses` map.
- Refactor existing FAILED string-literal sites to use the constant: `eval/cleanup.go:96,461`, `eval/seed.go:89`, `ops/events.go:80`, etc. Pure rename, no behaviour change.

**RED**: `grep -rn '"FAILED"' internal/ | grep -v '_test.go' | grep -v atoms_lint` should return zero hits after the refactor.

**Files**: 2 + N refactor (~6-8).

### C.2 — Reusable FAILED-state Recovery classifier

- New file `internal/ops/failed_recovery.go`. Function: `ClassifyFailedService(ctx, client, svc ServiceStack) (*Recovery, error)`.
- Inputs: service stack + a small recent-logs/events fetch (cap a few seconds; reuse `LogFetcher`).
- Output: `*ops.Recovery` (existing type at `internal/ops/verify.go:36`) with:
  - `Tool` — primary recommended action (e.g. `"zerops_logs"` to investigate, then `"zerops_deploy"` after fix)
  - `Action` — `"investigate"` / `"redeploy"` / `"recreate"` based on classification
  - `Args` — pre-filled (e.g. `serviceHostname=X facility=application since=15m`)
  - `Signals` — what to look for in the log tail (e.g. `${dep_*} unresolved`, `connect ECONNREFUSED`, `EACCES`)
- Classification heuristics mirror failureClassification's pattern library at `internal/ops/deploy_failure*.go`. Categories likely: `missing-dep` (env unresolved), `init-crash` (initCommands non-zero), `runtime-crash` (start exited), `port-mismatch` (healthcheck timeout), `unknown` (fallback).
- Pinned by `TestClassifyFailedService_*` table-driven tests against canned log fixtures.

**RED**: write tests first against the canned fixtures.

**Files**: 1 + tests (~2 net).

### C.3 — Plumb through `zerops_discover` (primary surface)

- `internal/ops/discover.go` (and its result DTO) — when a service in the result has `IsFailed(status)`, attach the Recovery from C.2.
- The list output gains a per-service `recovery` field, optional, only populated on FAILED.
- `internal/platform/zerops_mappers.go:112` — pure mapper, doesn't classify; classification stays in `ops/`.
- Snapshot tests + atom render-probe goldens may need refresh if any test embeds discover output.

**RED**: re-run `recover-failed-buildfromgit-missing-dep` (Phase A scenario), confirm new self-review shows agent reading `recovery` field from discover output rather than guessing.

**Files**: 2 + tests (~3 net).

### C.4 — Defence-in-depth: gate deploy / dev_server / workflow-start

- `internal/tools/deploy_ssh.go:139` and `internal/tools/deploy_local.go` (or a shared precheck): before any SSH/zcli call, fetch service status. If FAILED, return structured error envelope carrying the Recovery (per verify-error precedent).
- `internal/tools/dev_server.go:104` — same precheck.
- `internal/tools/workflow_develop.go:276` (and bootstrap-start equivalent) — when topology contains FAILED services in scope, attach `failedServices: [{hostname, recovery}]` block to the start response.
- All three sites use the same C.2 classifier — no duplicate logic.

**RED**: each precheck has its own Test (e.g. `TestDeploySSH_FAILEDServiceReturnsRecovery`).

**Files**: 3-4.

### C.5 — Verify check Recovery extension

- `internal/ops/verify_checks.go:56,198` — when `service_running` check observes status==FAILED, attach Recovery from C.2.
- Don't short-circuit subdomain Recovery on early service_running failure (`internal/ops/verify.go:118`) — surface both checks' Recoveries; agent gets full picture.

**RED**: extend existing verify tests with a FAILED-status fixture, assert Recovery shape.

**Files**: 2.

### C.6 — Atom routing for FAILED state

- New atom `internal/content/atoms/develop-recover-failed-service.md` (or similar slug; align with `develop-ready-to-deploy.md` precedent).
- Frontmatter includes `serviceStatus: [FAILED]`. Engine already routes on this axis per `internal/workflow/synthesize.go:398`.
- Body: TRIGGER + ACTION + FAILURE MODE in 3-5 lines; references the Recovery surface from C.3-C.5; does NOT duplicate the classifier text — atom guides, classifier classifies.
- Pinned by `internal/content/atoms_lint*.go` (Axes K/L/M/N) + scenarios test pinning the new atom routing.

**Files**: 1 atom + possibly 1 scenarios_test entry.

### C.7 — `CLAUDE.md` invariant pin

- Add bullet to project `CLAUDE.md` Conventions section: "**FAILED-state services carry structured Recovery on every primary surface** — `zerops_discover` populates `recovery` on each service with `IsFailed(status)`, and `zerops_deploy`/`zerops_dev_server`/workflow-start gate through the same classifier. Agents read this FIRST before treating a FAILED service as recoverable. Lives on `ops.DiscoverResult.<Per-Service>.Recovery` and `ops.Recovery` (single canonical type). Pinned by `TestDiscover_PopulatesFailedRecovery`, `TestDeploySSH_FAILEDServiceReturnsRecovery`, atom `develop-recover-failed-service`."

**Files**: 1 (CLAUDE.md).

### Phase C verification

- Fast: `make lint-local` + `go test ./internal/{platform,topology,ops,tools,workflow,content}/... -count=1` all green.
- Slow: re-run `recover-failed-buildfromgit-missing-dep` via `flow-eval.sh`. Compare new self-review to the Phase A baseline (captured pre-Phase-C). Recovery friction should be absent or strictly downgraded — agent reads `recovery` field, runs suggested action, redeploys cleanly.
- Cross: re-run any of the existing 19 scenarios that DON'T have FAILED services — recovery field must be absent on all healthy services, confirming the populate-only-on-FAILED rule.

---

## Cross-phase verification

After Phase A:
- `recover-failed-buildfromgit-missing-dep` runs end-to-end via `flow-eval.sh`. Seed produces FAILED state. Agent retrospective captures the friction (which tells us where today's gaps are — independent confirmation of Phase B's audit).

After Phase B:
- Audit notes committed under `eval/behavioral/audits/failed-state-zcp-audit-2026-05-04.md`.

After Phase C:
- `make lint-local` green.
- `go test ./internal/{ops,tools,workflow,content}/... -count=1` green.
- Targeted slow-loop: re-run the same FAILED-state scenario. New self-review shows agent reading recovery surface, not flailing.

---

## Risks

- **Audit surfaces a deeper structural gap than expected.** If FAILED-state requires a new platform-API call to recover (e.g. force-restart that doesn't exist as MCP tool), Phase C scope inflates. Mitigation: keep Phase C tight; if audit reveals platform-side gaps, file them as separate plan-doc against the platform team.
- **Eval seed-mode extension churns SeedImported callers**. `RunScenario` and `RunBehavioralScenario` both dispatch on `Scenario.Seed`; adding `settled` is purely additive — old modes unchanged. No backward compat work.
- **Tier-3 (mid-life crash) coverage NEEDS a way to put a healthy service into FAILED without a crash bomb.** Today there is no platform-API call that says "force this service into FAILED for testing". This is genuinely out-of-scope; document and skip.
- **Bundle.errors precedent**: `zerops_workflow workflow=export` already returns `status="validation-failed"` with structured `bundle.errors` per `spec-workflows.md §9`. The shape we choose for FAILED-state recovery should align with that precedent (status + structured details) for consistency.

---

## Out-of-scope items surfaced during plan authoring

- Failure-mode taxonomy could grow into a `topology.FailureMode` enum mirroring `topology.FailureClass`. Decide after Phase B.
- A "rebuild from last known good zerops.yaml" recovery mode for services that fail mid-life. Speculative — wait for evidence of need.
- Auto-creating missing managed deps when `${dep_hostname}` is unresolved. Tempting but not the right layer — the recovery surface should INFORM the agent so they decide; the agent then does the structural fix (add dep or change repo).

---

## Done definition

This plan is complete when:

- `seed: settled` mode shipped and tested.
- `recover-failed-buildfromgit-missing-dep` scenario authored and surfaces FAILED-state friction in retrospective.
- Phase B audit committed.
- Phase C surface design landed (whichever Option 1/2/3 audit picks); verified by re-running the FAILED-state scenario and seeing the agent navigate recovery cleanly.
- `plans/zcp-failed-state-recovery-2026-05-04.md` moved to `plans/archive/`.
