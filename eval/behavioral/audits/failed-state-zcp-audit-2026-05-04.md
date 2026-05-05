# ZCP FAILED-state handling audit

**Date**: 2026-05-04
**Plan**: `plans/zcp-failed-state-recovery-2026-05-04.md` Phase B.
**Auditor**: Codex sub-agent (read-only audit, no code edits).

## Summary

Across `discover` / `verify` / `deploy` / `dev_server` / `workflow` / `logs`,
ZCP today has **no recovery surface for services already in terminal FAILED
status**. The only failure surface is DURING an active deploy this MCP call
initiated. Agents that encounter a pre-existing FAILED service are silently
routed through normal flow until something downstream errors out without
structured Recovery.

## (a) What works today

| Surface | Code path | Behaviour on FAILED |
|---|---|---|
| `pollDeployBuild` (during ZCP-initiated deploy) | `internal/tools/deploy_poll.go:76,97,113` + `internal/ops/deploy_failure.go:38` | Treats non-ACTIVE appVersion status as failure, fetches build/runtime logs, classifies (`BUILD_FAILED`/`PREPARING_RUNTIME_FAILED`/`DEPLOY_FAILED`), populates `failureClassification` with suggestedAction. **Coverage: failure during this deploy attempt only.** |
| `record-deploy` | `internal/tools/workflow_record_deploy.go:232` | Blocks on a failed latest appVersion with actionable text. |
| Reusable Recovery DTO | `internal/ops/verify.go:36` | Type exists, used by subdomain-enable verify check (`internal/ops/verify.go:185`). Mature shape, ready for re-use. |
| Atom routing axis | `internal/workflow/synthesize.go:398` (`ServiceStatuses` field) | **Already supports `serviceStatus:` filtering**. `develop-ready-to-deploy.md` is the precedent atom. Adding a FAILED-routed atom is content-only. |

## (b) What's missing

| Surface | Code path | Gap |
|---|---|---|
| `zerops_discover` / `ListProjectServices` | `internal/platform/zerops_mappers.go:112` | FAILED is copy-passed as a raw string from `ServiceStack.Status`. No flag, no annotation, no Recovery attached. Agents see "FAILED" in JSON but get no "this needs recovery" signal. |
| `zerops_verify` `service_running` check | `internal/ops/verify_checks.go:56,198` | Emits `service status: FAILED` and flips envelope to `unhealthy`, but attaches no Recovery. Subdomain Recovery path is **skipped** when service_running fails early (`internal/ops/verify.go:118`). |
| Atom corpus | `internal/content/atoms/` | No `serviceStatus: [FAILED]` atom exists. `ComputeEnvelope` / `Synthesize` renders `[FAILED]` in the status block (`internal/workflow/render.go:192`) but has nothing to route to. |
| Platform constants | `internal/platform/types.go:119` | Service-level constants stop at NEW/ACTIVE/READY_TO_DEPLOY/RUNNING. **No `ServiceStatusFailed` constant**. FAILED matched only as raw string at every call site. |

## (c) What's silently broken

| Surface | Code path | What happens on a pre-existing FAILED service |
|---|---|---|
| `zerops_deploy` (SSH path) | `internal/tools/deploy_ssh.go:139,207` → `internal/ops/deploy_ssh.go:87,143` | Proceeds normally through strategy / adoption / preflight, then SSHes and runs zcli. **Prior failure does NOT populate `failureClassification`** (per comment at `internal/ops/deploy_common.go:44`). Agent cannot distinguish "deploy succeeded on previously-FAILED service" from "service was already healthy". |
| `zerops_deploy` (local path) | `internal/ops/deploy_local.go:81,149` | Same as SSH — service-by-ID lookup, no FAILED check. |
| `zerops_dev_server` | `internal/tools/dev_server.go:104` → `internal/ops/dev_server.go:180,247` | Confirms hostname exists, then immediately starts SSH probing. FAILED runtime produces SSH errors or probe errors — unstructured, no Recovery payload. |
| `zerops_workflow` develop start | `internal/tools/workflow_develop.go:87,276` | `liveHostnames` built from names only; scope validation checks runtime metas not status. Workflow opens on FAILED-service topology. |
| `zerops_workflow` bootstrap (route discovery/adopt) | `internal/workflow/route.go:285` | Filters on system/managed/meta/self — not on status. Bootstrap proceeds; failure surfaces later. |
| Generic workflow check | `internal/tools/workflow_checks.go:179` | Late-stage failure surfaces as `expected one of [...], got FAILED` with no Recovery. |

## (d) Recommendation

Fix this **centrally**, not as scattered per-tool branches.

1. **Constant + predicate**: add `ServiceStatusFailed = "FAILED"` beside existing constants in `internal/platform/types.go:119`; add `IsFailed(s ServiceStack) bool` in `internal/topology/` (zero imports — foundational vocabulary per CLAUDE.md layer-2 rule).
2. **Reusable Recovery helper**: shared classifier returning `ops.Recovery` (existing shape, established by verify subdomain path). Inputs: service stack + recent logs/events. Output: tool/action/args/signals.
3. **Plumb through `discover` first** (earliest surface agents see, `internal/ops/discover.go:28`) — every FAILED service gets Recovery attached at list time. Single read, no follow-up call needed.
4. **Gate `zerops_deploy`, `zerops_dev_server`, and workflow-start** through the same helper before SSH/zcli/session creation — defence in depth, prevents silent normal flow.
5. **Verify extension**: `service_running` check attaches Recovery on FAILED, and stops short-circuiting the subdomain Recovery path when service_running fails (surface BOTH).
6. **Atom**: add a `serviceStatus: [FAILED]` routed recovery atom. Engine already supports this axis (`synthesize.go:398`); precedent at `develop-ready-to-deploy.md`. Content-only addition.
7. **Logs**: stay status-agnostic; add atom-level guidance note to widen `since=` window when the failure is not recent.

This avoids repeating FAILED-checks at each tool boundary while keeping the Recovery shape consistent with the verify contract already in use.
