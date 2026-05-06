# Plan: Local-Flow Fundamentals — Tactical Bug-Fix Wave (Phase 5–12)

> **Status**: Proposed. Replaces the over-engineered v1 of this plan.
> **Date**: 2026-05-06
> **Predecessor**: Phase 1–4 already shipped (commits `390b6082`,
> `472f5162`, `e9c2be9c`, `9a90de1b`, `7697c9a1`, `b6d97418`).
> **Scope**: Eight small bug-fix commits targeting specific cracks in
> local-mode flow surfaced across six behavioral eval runs + Codex
> foundation map + live e2e verification.
> **Scope OUT**: No new types. No new state files. No new atom axes.
> No migration. **This is a bug-fix wave, NOT an architectural redesign.**

## Why this plan exists

Eval runs surfaced friction; first-pass plan responded by proposing a
new `LocalProject` type + `localTopology` atom axis + envelope
restructure. User pushed back: **adding architecture without a
corresponding new capability is a red flag**. Code review confirmed
every "bend" can be fixed in-place with small, targeted changes. The
new axis would have been pure renaming with no behavioral win.

This plan is the honest minimum: each phase fixes a verified bug,
none introduces new abstractions.

## Foundation: what we verified before writing this plan

1. **Recursive expansion works** (e2e test pass, suite log
   `e2e/env_generate_test.go::TestE2E_EnvGenerateDotenv_RecursiveConnectionString`):
   ```
   DATABASE_URL=postgresql://db:6laACJzc.4Pr5Qg.MThjnnyO@zcpdbc6dbbf1f:5432
   ```
   Phase 4's recursive expander correctly resolves `${db_connectionString}`
   against db's sibling envs (`user`, `password`, `hostname`, `port`)
   via real Zerops API. Doctrine validated.

2. **`.env` security leak confirmed live** (same e2e):
   `.env` includes `ZCP_API_KEY` (project-scoped deploy token) +
   `staticCdnUrl`/`apiCdnUrl`/`storageCdnUrl` + `envIsolation` +
   `sshIsolation` + `zeropsSubdomainHost`/`zeropsSubdomainString`.
   Real bug — addressed in Phase 6.

3. **Container "auto first then change" pattern**: bootstrap writes
   `CloseDeployMode=unset` for all targets; `develop-strategy-review`
   atom fires post-deploy on `closeDeployModes:[unset]`; agent then
   sets close-mode explicitly. Local should match this pattern (Phase 9
   simplification).

4. **Atom axis squeeze is NOT a bug**: `modes:` axis legitimately
   namespaces both container service roles and local topologies. Bug
   is **only** the projection: `resolveEnvelopeMode` projects
   local-stage runtime as `ModeStage` instead of `ModeLocalStage`,
   so atoms with `modes: [..., local-stage]` silently miss. Fix is
   3-line projection switch + ~10-line synthetic snapshot.
   Verified: zero atoms have `modes: [stage]` standalone, so
   changing local-stage projection cannot strand any container atom.

5. **`ServiceMeta.Hostname` dual-meaning is acceptable**: It's the
   only place semantics differ; consumers either iterate live
   services (no project-name hit) or check meta.Mode explicitly
   (Phase 1.5 + Phase 2 already addressed prune + scope listing).
   Adding a separate `LocalProject` type would relocate cognitive
   cost without eliminating it.

## How an LLM implementer should approach this plan

1. Read top-to-bottom before starting.
2. Order is strict: 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12.
3. TDD: every code change is RED → GREEN. Pure refactor (no behavior
   change) skips RED but still verifies all layers green.
4. Container regression is non-negotiable: every phase has explicit
   container test coverage or the change is provably local-only.
5. Live verify is batched: single `make flow-eval-local` after batch
   D (Phase 11+12), plus the existing e2e env test catches Phase 5+6
   regressions.
6. Single commit per phase.

## Reference: actual types this plan touches

These were wrong in v1; verified against current code:

```go
// internal/topology/recovery.go
type Recovery struct {
    Tool   string            `json:"tool"`
    Action string            `json:"action"`
    Args   map[string]string `json:"args,omitempty"`  // string values only
}

// internal/tools/destructive_ack.go
type DiagnosedDestruction struct {
    Operation string          `json:"operation"`
    Targets   []string        `json:"targets"`
    Loss      DestructionLoss `json:"wouldDestroy"`  // wire path: wouldDestroy.wouldDestroy.envVars
}

// internal/tools/workflow.go
CloseModes  map[string]string `json:"closeMode,omitempty"`  // exists; do NOT add string CloseMode field

// internal/ops/env_generate.go (after Phase 4)
var refPatternAny = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)
type refExpander struct {
    client    platform.Client
    projectID string
    cache     map[string][]platform.EnvVar
}

// internal/ops/deploy_validate.go (sibling parser — same first-underscore bug)
func ValidateEnvReferences(envVars, discoveredEnvVars map[string][]string, liveHostnames []string) []EnvRefError
func parseEnvRefs(s string) []envRef  // ← same bug as env_generate before Phase 5
```

## Phase 5 — Shared env-ref classifier with live-hostname matching

**Why**: `expandRefs` (env_generate.go) splits on first underscore, so
`${STAGE_API_URL}` becomes `host=STAGE, var=API_URL` → "service not
found" error at top level. Per `internal/knowledge/guides/environment-variables.md:57`,
service `my-db` ref is `${my_db_port}` — `strings.Cut` reads
`host=my, var=db_port` → wrong. Same bug in `parseEnvRefs`
(deploy_validate.go) hits container deploy preflight too.

**What**: Extract `internal/ops/env_refs.go` with
`EnvRefClassifier` keyed off live hostnames (canonicalized
dash→underscore). Used by both call sites. `expandRefs` no longer
calls `ListServices` itself — receives pre-built classifier +
service index from `EnvGenerateDotenv`.

```go
type EnvRefClassifier struct {
    hostUnderscoreNames map[string]string  // wire-form host → canonical
}

// Classify finds longest live-hostname prefix in body. Returns
// (host, var, true) for cross-service refs; (zero, zero, false) for
// lone refs. Caller decides literal-vs-sibling based on context.
func (c *EnvRefClassifier) Classify(body string) (string, string, bool)
```

**Tests**:
- `TestEnvGenerateDotenv_TopLevelLoneRefStaysLiteral` — `${SOME_PROJECT_VAR}` literal.
- `TestEnvGenerateDotenv_DashHostnameLongestPrefix` — `${my_db_hostname}` resolves via "my-db".
- `TestValidateEnvReferences_LoneRefIgnored`.
- `TestValidateEnvReferences_DashHostnameLongestMatch`.
- `TestExpandRefs_ListServices_CalledOncePerBatch` (call-count assertion).
- Container regression: `TestEnvGenerateDotenv_SingleWordHostnameUnchanged`.

**Container risk**: ZERO. Single-word hostnames classify identically
under longest-prefix and first-underscore. Both bend sites already
deal with both modes via shared parsers — same fix applies to both.

**Size**: ~120 LOC.

## Phase 6 — `.env` denylist for platform internals

**Why** (verified live): `.env` contains `ZCP_API_KEY` (deploy
token), `*CdnUrl`, `envIsolation`, `sshIsolation`,
`zeropsSubdomainHost`, `zeropsSubdomainString`. User fat-finger
`git add -A` despite `.gitignore` would publish the deploy token.

**What**: Constant `platformInternalKeys` map in `env_generate.go`.
Filter applied to project-env append loop. Yaml-defined refs are
exempt (user wrote them deliberately). Result struct gains
`OmittedPlatformKeys []string` for transparency.

```go
var platformInternalKeys = map[string]bool{
    "ZCP_API_KEY":           true,
    "envIsolation":          true,
    "sshIsolation":          true,
    "apiCdnUrl":             true,
    "staticCdnUrl":          true,
    "storageCdnUrl":         true,
    "zeropsSubdomainHost":   true,
    "zeropsSubdomainString": true,
}
```

**NOT denylisted**: `APP_KEY`, `APP_SECRET`, framework auto-secrets.
Local app reads these. `.env` remains secret-bearing.

**Tests**:
- `TestEnvGenerateDotenv_PlatformInternalsFiltered` — `wantNotContainsLines: []string{"ZCP_API_KEY=", "zeropsSubdomainHost="}` (per-line assertion, not substring — header may mention key names).
- `TestEnvGenerateDotenv_YamlRefOverridesDenylist` — explicit user ref wins.
- Container regression: container-context generate-dotenv applies same denylist.

**Container risk**: ZERO. Denylist is symmetric (these keys are
platform-internal in any env).

**Size**: ~30 LOC.

## Phase 7 — Failed-deploy recovery: populate envVars + better suggestion text

**Why**: `gateOverrideOnFailedHistory` declares
`envVarsByService := make(map[string][]string)` then never writes to
it. `collectEnvVarKeys` always returns empty. `wouldDestroy.envVars`
always empty. Agent sees "no env loss" when override may delete
real keys.

Plus: `Recovery.Args` is `map[string]string` — too narrow for the
nested `confirmDestructive` payload. Currently agents have to
hand-construct it from the wouldDestroy shape.

**What** (NO new wire type — keep simple):
1. Populate `envVarsByService` via `ops.LookupService` + `ops.FetchServiceEnv`.
2. Improve `PlatformError.Suggestion` field text to include a
   copy-pasteable JSON snippet of the next call:
   ```
   Suggestion: After reading logs, retry with:
     zerops_import override=true confirmDestructive={"operation":"import-override","acknowledgedTargets":["app"]}
   ```
3. Drop `facility` arg from RecoveryHint args (covered in Phase 8).

**Tests**:
- `TestGateOverrideOnFailedHistory_PopulatesEnvVarLoss`.
- `TestGateOverrideOnFailedHistory_SuggestionIncludesRetryShape`.
- Existing `TestImport_OverrideOnFailedRequiresAck` updated for new
  envVars population.

**Container risk**: ZERO. Same gate runs in container. `envVarsByService`
fix is strictly additive (was always empty; now has real keys).
Suggestion text change is doc-only.

**Size**: ~40 LOC.

## Phase 8 — Doc-only fixes

**Why**:
1. `internal/tools/import.go:159-166` — recovery hint args include
   `"facility": "application"`. `LogsInput` has no Facility field;
   MCP rejects.
2. `internal/tools/env.go::envInputSchema` — schema description for
   `serviceHostname` says "Ignored by generate-dotenv". Implementation
   requires it.

**What**:
1. Drop `"facility"` from RecoveryHint args.
2. Rewrite `serviceHostname` description: "Required by generate-dotenv
   to identify which setup block in zerops.yaml's run.envVariables
   to resolve."

**Tests**:
- `TestImport_RecoveryHint_NoFacilityArg`.
- `TestEnvSchema_ServiceHostname_RequiredDescription`.

**Container risk**: ZERO. Pure doc/data fixes.

**Size**: ~5 LOC.

## Phase 9 — Close-mode parity with container's "unset → review → pick"

**Why**: Container bootstrap writes `CloseDeployMode=unset`; agent
calls `develop-strategy-review` atom post-deploy and picks then.
Local mode currently:
- Zero-runtime branch → `CloseModeManual` + `Confirmed=true`
- Multi-runtime branch → unset (silent inconsistency)
- `handleAdoptLocal` → unset on upgrade local-only → local-stage

**What** (simpler than v1 plan — NO new field, NO LinkedCloseMode):
1. `LocalAutoAdopt` zero-runtime branch: change to `CloseDeployMode=unset`
   + `Confirmed=false`. Match multi-runtime branch.
2. `handleAdoptLocal`: keep `CloseDeployMode=unset` on upgrade
   (unchanged, but now consistent with the unified path).
3. Spec amendment: `docs/spec-local-dev.md §4` — update local-only
   description to say close-mode stays unset until strategy-review.

**Tests**:
- `TestLocalAutoAdopt_ZeroRuntime_LeavesCloseModeUnset` (new).
- `TestLocalAutoAdopt_MultiRuntime_LeavesCloseModeUnset` (existing,
  may need update).
- Local strategy-review atom fires for both cases (envelope-level
  test).

**Container risk**: ZERO. `LocalAutoAdopt` and `handleAdoptLocal` are
local-only (gated by `!rtInfo.InContainer`).

**Size**: ~10 LOC.

## Phase 10 — Adoption note durability + CLAUDE.md refresh symmetry

**Why**:
1. `runLocalAutoAdopt` returns `""` once any meta exists. Agents
   joining adopted local project (second+ server start) see no
   adoption guidance in MCP instructions.
2. `RefreshClaudeMD` is gated `if rtInfo.InContainer` — local users
   with stale CLAUDE.md template silently use outdated guidance.

**What**:
1. New helper `workflow.FormatLocalStateNote(metas, services, projectName)`:
   Always emits a current-state note. For local-only with multiple
   runtimes, leads with `BEFORE running develop, link a runtime via
   adopt-local...` (actionable recovery up-front, not buried).
2. `runLocalAutoAdopt` always calls FormatLocalStateNote (not just
   on first call).
3. Drop `rtInfo.InContainer` gate from `RefreshClaudeMD` call in
   `server.New`. Update `BuildClaudeMD` doc-comment to reflect
   serve-time refresh now happens in both envs.

```go
func FormatLocalStateNote(metas []*ServiceMeta, services []platform.ServiceStack, projectName string) string {
    local := findLocalMeta(metas)
    if local == nil { return "" }
    if projectName == "" { projectName = local.Hostname }  // fallback
    runtimes, managed := classifyServices(services)
    switch {
    case local.Mode == topology.PlanModeLocalStage:
        return formatLocalStageNote(local, runtimes, managed, projectName)
    case len(runtimes) > 1, len(runtimes) == 1:
        return formatLocalOnlyMultiRuntimeNote(local, runtimes, managed, projectName)
    default:
        return formatLocalOnlyZeroRuntimeNote(local, managed, projectName)
    }
}
```

**Tests**:
- `TestRunLocalAutoAdopt_SecondCall_StillSurfacesNote`.
- `TestFormatLocalStateNote_LocalOnlyMultiRuntime_LeadsWithAdoptLocal`.
- `TestServerNew_LocalEnv_RefreshesClaudeMD`.
- Container regression: `TestServerNew_ContainerEnv_StillRefreshesClaudeMD`.

**Container risk**: ZERO. Adoption note re-emission is local-only
(`runLocalAutoAdopt` is gated). CLAUDE.md refresh extension is
symmetric (container behavior unchanged).

**Size**: ~80 LOC.

## Phase 11 — Envelope projection: ModeLocalStage + synthetic local-only

**Why**: `resolveEnvelopeMode` projects local-stage's stageHostname as
`ModeStage`. Atoms with `modes: [..., local-stage]` (no `stage`)
silently miss. Affected atoms (verified by grep):
- `develop-build-observe.md` `[standard, simple, local-stage, local-only]`
- `develop-first-deploy-execute.md` `[dev, simple, standard, local-stage]`
- `develop-ready-to-deploy.md` `[dev, simple, standard, local-stage]`
- `develop-dynamic-runtime-start-local.md` `[dev, standard, local-stage, local-only]` + `runtimes: [dynamic]`
- `develop-close-mode-git-push.md` `[standard, simple, local-stage, local-only]`
- `develop-close-mode-git-push-needs-setup.md` same.

**Container regression check** (re-verified):
```
grep -rE "modes:.*\bstage\b" internal/content/atoms/*.md | grep -v "local-stage"
# (empty — zero atoms have stage standalone)
```

So changing local-stage projection cannot strand any container atom.

**What** (no new axis, no new types):
1. `resolveEnvelopeMode`: when role is `DeployRoleStage` AND `meta.Mode
   == PlanModeLocalStage`, return `ModeLocalStage` (not `ModeStage`).
2. `buildServiceSnapshots`: append synthetic snapshot for each
   `PlanModeLocalOnly` meta (Mode + close/git fields, no
   RuntimeClass/Status — local-only has no linked runtime).

```go
// resolveEnvelopeMode case for stage role
case topology.DeployRoleStage:
    if meta.Mode == topology.PlanModeLocalStage {
        return topology.ModeLocalStage
    }
    return topology.ModeStage
```

```go
// buildServiceSnapshots tail addition
for _, m := range metas {
    if m == nil || m.Mode != topology.PlanModeLocalOnly { continue }
    out = append(out, ServiceSnapshot{
        Hostname:        m.Hostname,
        Mode:            topology.Mode(m.Mode),
        Bootstrapped:    m.IsComplete(),
        CloseDeployMode: m.CloseDeployMode,
        GitPushState:    m.GitPushState,
    })
}
```

**Atom coverage table** (post-fix):

| Atom | local-stage (linked dynamic runtime) | local-only |
|---|---|---|
| `develop-build-observe.md` | ✓ via live runtime snapshot | ✓ via synthetic |
| `develop-first-deploy-execute.md` | ✓ | — by design (no `local-only` in filter) |
| `develop-ready-to-deploy.md` | ✓ | — by design |
| `develop-close-mode-git-push.md` | ✓ | ✓ |
| `develop-close-mode-git-push-needs-setup.md` | ✓ | ✓ |
| `develop-dynamic-runtime-start-local.md` | ✓ | — by design (synthetic has no RuntimeClass; runtime-gated guidance is premature for unlinked project) |

**Tests**:
- `TestResolveEnvelopeMode_LocalStage_ProjectsAsModeLocalStage`.
- `TestResolveEnvelopeMode_ContainerStandard_StillProjectsAsModeStage` (regression).
- `TestBuildServiceSnapshots_LocalOnly_AppendsSyntheticSnapshot`.
- `TestBuildServiceSnapshots_ContainerStandard_NoSynthetic` (regression).
- Atom-render assertions: `TestAtomMatching_BuildObserve_FiresForLocalStage`.

**Container risk**: ZERO. Verified by grep — no container atom
depends on the misprojection.

**Size**: ~15 LOC + tests.

## Phase 12 — Env-aware adopt recovery + git-push gate carve-out

**Why**:
1. `requireAdoption` blocks deploys to non-meta hostnames. For
   local-only project, agent calls `deploy targetService=app` →
   "Service `app` not adopted; run bootstrap" — wrong recovery (right
   action is `adopt-local`).
2. `PushSourceCheckFor` rejects local-stage targeting stage hostname
   as `PushSourceIsStageHalf` ("use dev hostname instead") — but
   in local mode, "dev hostname" is the project name (local CWD).
   Hits all three sites: `gitPushMetaPreflight` (shared by container
   `handleGitPush` + local `handleLocalGitPush`) AND `handleGitPushSetup`.

**What**:
1. `requireAdoption` accepts `runtime.Info` parameter; in local env,
   recovery points at `adopt-local` (with structured Recovery hint),
   in container at `bootstrap`.
2. `PushSourceCheckFor` short-circuits local-stage + stage-hostname
   target → return `PushSourceIsValidSource` (existing constant). One
   change in shared classifier covers all three callers.

```go
// PushSourceCheckFor (in service_meta.go)
func (m *ServiceMeta) PushSourceCheckFor(hostname string) topology.PushSourceResult {
    if m == nil || hostname == "" {
        return topology.PushSourceUnknownHost
    }
    // Local-stage + targeting linked stage = legitimate (project name
    // is the source; stage hostname is the deploy target).
    if m.Mode == topology.PlanModeLocalStage && hostname == m.StageHostname {
        return topology.PushSourceIsValidSource
    }
    // ... existing checks unchanged ...
}
```

**Tests**:
- `TestRequireAdoption_LocalEnv_RecoveryPointsAtAdoptLocal`.
- `TestRequireAdoption_ContainerEnv_StillPointsAtBootstrap` (regression).
- `TestPushSourceCheckFor_LocalStage_StageHostnameAccepted`.
- `TestPushSourceCheckFor_ContainerStandard_StageHostnameStillRejected` (regression).
- Three-caller integration: `TestHandleGitPush_LocalStage_StageHostnameTarget_Proceeds`,
  `TestHandleLocalGitPush_LocalStage_StageHostnameTarget_Proceeds`,
  `TestHandleGitPushSetup_LocalStage_StageHostnameTarget_Proceeds`.

**Container risk**: ZERO. Container path branches preserved
(`InContainer=true` keeps bootstrap message; `PlanModeStandard` keeps
StageHalf rejection).

**Size**: ~30 LOC.

## End-of-plan verification

After all 8 phases land:
1. Full test sweep: `go test ./... -count=1 -race`.
2. Lint clean: `make lint-local`.
3. Live local: `make flow-eval-local ID=local-auto-adopt-node-postgres-first-deploy`.
   Read self-review; confirm zero remaining critical friction.
4. Live container regression: pick `existing-simple-mode-add-endpoint` from
   `eval/behavioral/scenarios/`; run `flow-eval`; confirm container
   behavior unchanged.
5. E2E env still passes:
   `go test ./e2e/ -tags e2e -run TestE2E_EnvGenerateDotenv -v -timeout 900s`.
6. Archive plan: `git mv plans/local-flow-fundamentals-2026-05-06.md plans/archive/`.

## Out of scope — separate plans if/when

- **`wouldDestroy.envVars` JSON path rename**: cleanup the
  `wouldDestroy.wouldDestroy.envVars` double-nesting. Wire shape
  break; needs separate audit of consumers.
- **Recipe-knowledge edits**: `npm ci` lockfile precondition,
  `connectionString` cross-service guidance. Recipe-team scope;
  sync-push amplification. Backlog
  `plans/backlog/recipe-knowledge-context-bleed-adopt-scenarios.md`.
- **Build-failure classifier coverage**: `npm ci` no-lockfile
  pattern, empty-logs case. Needs live failure-pattern collection.
- **Spec stale decision-log entries**: `docs/spec-local-dev.md §13`
  D4-D6 reference pre-project-keyed design. Doc-only refactor.

## Risks for implementer

1. Phase 7 wire path: `wouldDestroy.envVars` is actually nested at
   `wouldDestroy.wouldDestroy.envVars` (Go field `Loss` has JSON tag
   `"wouldDestroy"`). Tests assert actual path; rename is separate
   future plan.
2. Phase 11 atom axes beyond Mode: synthetic local-only snapshot
   carries no RuntimeClass; atoms with `runtimes:` filter
   intentionally don't match for local-only. Coverage table above
   documents which atoms gain firing for which mode. Future atoms
   adding `runtimes:` axis to local targets need explicit testing.
3. Phase 5 nil-classifier: empty `liveHostnames` → all refs become
   "lone refs" → loosens shim-mode validation in
   `zcp check env-refs`. Accepted trade-off (live mode is primary).
   Re-evaluate if real bugs slip through shim mode.

## Hand-off checklist

When starting work in a fresh session:
- Read this plan top-to-bottom.
- Confirm preconditions (`git status` clean, tests green, lint clean).
- Begin Phase 5. Each phase: RED → GREEN → tests + lint + race → commit.
- Live verify after Phase 6 (run e2e env test) + after Phase 12 (run flow-eval-local).
- If phase reveals dependency on a deferred phase, STOP + surface to user.
