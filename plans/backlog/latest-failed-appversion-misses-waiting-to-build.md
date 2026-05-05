# LatestFailedAppVersionContext misses WAITING_TO_BUILD-stuck buildFromGit failures

**Surfaced**: 2026-05-05 (Phase 4 verification of plan v4 diagnose-before-destruct on `recover-failed-buildfromgit-missing-dep`, suite `20260505-145436`)

**Why deferred**: the empirical behavior the v4 plan was solving (agent destroying user code via blind `override=true`) didn't recur post-fix — the agent read `zerops_events` BEFORE attempting override, made an informed decision, and the override succeeded. The structural mechanism (Phase 1.4 Recovery hint + Phase 3.2 gate + shared shape) is in place. This entry tracks a detection gap that made the Recovery hint suboptimal but didn't break agent behavior.

**Trigger to promote**: any future eval where the agent destroys uncommitted user code via `override=true` because the diagnostic gate didn't fire AND the agent didn't independently read events. Or: an audit pass over scenarios where the helper-side `LatestFailedAppVersionContext` returns nil for services that empirically have failed deploys.

## Sketch

`internal/ops/failed_context.go::LatestFailedAppVersionContext` walks `SearchAppVersions` and matches `av.Status` against `BUILD_FAILED` / `PREPARING_RUNTIME_FAILED` / `DEPLOY_FAILED` via `FailurePhaseFromStatus`. Empirically observed in this run: a `buildFromGit` service whose initCommand crashed on missing `${db_hostname}` left:

- the latest **appVersion** stuck in `Status="WAITING_TO_BUILD"` (not `BUILD_FAILED`)
- a separate **process** event (`type="process" action="build" status="FAILED"`) recording the actual failure
- service stack itself in `READY_TO_DEPLOY`

The helper walks appVersion-level events only, doesn't see the WAITING_TO_BUILD entry as a failure, returns nil. Downstream consumers:

- `workflow_checks::checkServiceStatusAny` Recovery suboptimally points at `zerops_logs` (the "never deployed" branch) instead of `zerops_import override=true` (the "diagnosed-failure" branch).
- `Phase 3.2 gate in tools/import.go` doesn't fire (no failed history → gate bypassed) → un-acknowledged `override=true` succeeds.

**Fix sketch**: extend the helper to also walk `SearchProcesses` for action="build" status="FAILED" (or action="deploy" status="FAILED") tied to the target service-stack-id, since these are the canonical failure markers when appVersion never transitions out of `WAITING_TO_BUILD`. Map to `FailureClassBuild` + `LikelyCause="build process failed before completing"`. Keep the appVersion-status path as the primary signal — process-level scan is the fallback for the WAITING_TO_BUILD edge case.

Alternative: change `FailurePhaseFromStatus` to also recognize `WAITING_TO_BUILD` when paired with a failed sibling process — but that's API-shape coupling and harder to test.

## Risks

- Process scan adds one more API call per pre-flight gate (~50-200ms). Acceptable given the gate already calls `SearchAppVersions` once.
- Process-level FAILED includes non-build failures (subdomain-enable cancellation, scale rollback). Filter to `action="build"` / `action="deploy"` only.
- Need to map process → service-stack-id: `ProcessEvent.ServiceStacks[0].ID`.

## Refs

- Detected in `eval/behavioral/runs/20260505-145436/recover-failed-buildfromgit-missing-dep/self-review.md` (post-fix run, agent observed Recovery pointing at logs instead of import).
- Source: `internal/ops/failed_context.go::LatestFailedAppVersionContext`.
- Related: `internal/ops/deploy_failure.go::FailurePhaseFromStatus` would also need a sibling helper for process-level recognition.
- Plan v4 (shipped): `plans/archive/zcp-diagnose-before-destruct-2026-05-05.md`.
