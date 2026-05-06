# Plan: Local-Flow Fundamentals — Tactical Wave (Phase 5–12)

> **Status**: Proposed. Pending second Codex review + user approval.
> **Date**: 2026-05-06
> **Predecessor**: Phase 1–4 already shipped (commits `390b6082`,
> `472f5162`, `e9c2be9c`, `9a90de1b`, `7697c9a1`, `b6d97418`). Findings
> in this plan surfaced via:
>   - Six behavioral eval runs on `local-auto-adopt-node-postgres-first-deploy`
>     scenario, suite IDs `20260506-{133416,141525,144837,145922,150933,152718}`
>     under `eval/behavioral/runs-local/`.
>   - Codex audit + own re-verification documented in chat (this document).
>   - First-pass plan + Codex review surfaced 12 issues that this revised
>     plan addresses.
> **Scope IN**: Eight tactical commits (Phase 5–12) that close specific
> local-mode correctness gaps. Each is independently verifiable, has
> explicit container-regression coverage, and is gated by a live
> `flow-eval-local` re-run.
> **Scope OUT**: Full deploy-typed-intent split (Codex's C3 — refactor
> every deploy gate to consume a typed `LocalDeployIntent` value). The
> narrow C3 fix in Phase 12 addresses the user-visible friction; the
> typed-intent redesign waits for `plans/deploy-typed-intent-202X.md`.

---

## How an LLM implementer should approach this plan

1. **Read top-to-bottom before starting any phase.** The phases are
   independent enough to commit separately, but the rationale chain
   only makes sense end-to-end.
2. **Order is strict.** Phase 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12. Each
   commits green before the next starts.
   - **Behavioral dependency call-out**: Phase 11 exposes atoms with
     `modes: [..., local-stage]` filter (e.g. `develop-close-mode-git-push.md`)
     that emit `zerops_deploy strategy="git-push"` commands referencing
     stage hostnames. Phase 12's shared-gate fix is what protects those
     newly-exposed commands from spurious "use the dev hostname"
     rejections in `gitPushMetaPreflight`. **Phase 11 MUST land paired
     with Phase 12** — never ship 11 alone. The same restriction does
     NOT apply to phases 5-10; they're independently safe.
3. **TDD per `CLAUDE.md`.** Every code change is `RED` (failing test
   first), `GREEN` (implementation makes test pass). Pure refactors
   skip RED — verify all layers stay green. Phases 11 + 12 are bug
   fixes, not refactors — RED + GREEN required.
4. **Container regression is non-negotiable.** Every phase has explicit
   container test coverage. Skip-conditions in code MUST be narrow to
   the local-mode case; any change that could affect a container
   metadata path is rejected.
5. **Live verify is batched.** Live `flow-eval-local` is ~5 min wall +
   session-bridge state; per-phase verification is too expensive +
   brittle. Run live eval after each batch and read self-review.md
   between batches:
   - Batch A: Phase 5 + Phase 6 (env-ref classifier + .env denylist).
   - Batch B: Phase 7 + Phase 8 (recovery enrichment + doc fixes).
   - Batch C: Phase 9 + Phase 10 (close-mode + adoption note durability).
   - Batch D: Phase 11 + Phase 12 (envelope projection + adopt recovery).
   Per-phase: keep unit/integration tests + race + lint as the gate. If
   a batch's live eval surfaces unexpected friction, bisect within the
   batch (the two phases are small and individually revertable).
6. **No silent skips.** If a phase reveals an unexpected dependency on
   a deferred phase (e.g. full typed-intent split for a gate that
   Phase 12 narrow fix doesn't cover), STOP and surface to the user
   — do not implement the deferred work as a side-quest.
7. **Single commit per phase.** No "WIP" intermediates. Each phase
   head commit must compile, tests must pass, lint must be 0 issues,
   and the live eval must reproduce the targeted fix.

---

## Why these eight phases (and not others)

The audit surfaced 14 issues classified CRITICAL / MEDIUM / MINOR. This
plan picks eight that satisfy:

| Criterion | Why |
|---|---|
| Independently verifiable | Each phase's regression test stands alone |
| Narrow blast radius | Skip-conditions are local-mode-only or platform-internal-only |
| Container-safe by construction | Either touches local-only code paths OR uses additive enrichment OR has zero-atom-overlap (Phase 11) |
| High user-visible win | Each was raised by an agent retrospective in 6 eval runs |
| No multi-system reshape needed | Defers full typed-intent split (Codex C3) to dedicated plan |

The deferred item is real but architectural and would conflate "fix
obvious bugs" with "redesign axis projection". Implementing it under
the same plan would violate the engineering priority in
`CLAUDE.local.md` ("structural correction vs. minimal patch").

---

## Preconditions

```bash
# Build + auth (operator setup)
make build
export ZCP_API_KEY=$(jq -r '.mcpServers.zcp.env.ZCP_API_KEY' .mcp.json)
export PATH="$PWD/bin:$PATH"

# Verify clean state
git status                              # must be clean
go test ./... -count=1 -short           # must be green
./bin/golangci-lint run ./internal/...  # must be 0 issues
```

VPN must be up to eval-zcp (`zcli vpn up <projectId>`). `ZCP_API_KEY`
must be the eval-zcp project-scoped token (same one in `.mcp.json`).

`runtime.Info` for spawned `zcp` is local — eval runner unsets
`serviceId` (`cmd/zcp/eval_behavioral_local.go::runBehavioralRunLocal`).

---

## Reference: types this plan touches

Pin these signatures before implementing — they were wrong in the
first-pass plan and Codex review caught the drift.

```go
// internal/topology/recovery.go
type Recovery struct {
    Tool   string            `json:"tool"`
    Action string            `json:"action"`
    Args   map[string]string `json:"args,omitempty"` // string values only
}

// internal/tools/errwire.go
type RecoveryHint = topology.Recovery   // alias

// internal/tools/destructive_ack.go
type DiagnosedDestruction struct {
    Operation string          `json:"operation"`
    Targets   []string        `json:"targets"`
    Loss      DestructionLoss `json:"wouldDestroy"`  // ⚠ field is "Loss" but JSON tag is "wouldDestroy"
}
type DestructionLoss struct {
    ServiceStacks   []string `json:"serviceStacks,omitempty"`
    EnvVars         []string `json:"envVars,omitempty"`
    LocalFiles      []string `json:"localFiles,omitempty"`
    UncommittedCode bool     `json:"uncommittedCode,omitempty"`
}
// Wire path is therefore wouldDestroy.wouldDestroy.envVars (yes, doubly nested).

// internal/tools/workflow.go
type WorkflowInput struct {
    ...
    CloseModes  map[string]string `json:"closeMode,omitempty"`  // ⚠ field plural, JSON tag singular
    ...
}
// Adding a separate string field "closeMode" would clash on JSON tag.
// Phase 9 introduces a NEW field name (e.g. LinkedCloseMode / json:"linkedCloseMode").

// internal/ops/env_generate.go (current after Phase 4)
var refPatternAny = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)
type refExpander struct {
    client    platform.Client
    projectID string
    cache     map[string][]platform.EnvVar
    // hostUnderscoreNames map[string]string  // ADDED in Phase 5
}

// internal/ops/deploy_validate.go (sibling parser — same first-underscore bug)
func ValidateEnvReferences(envVars, discoveredEnvVars map[string][]string, liveHostnames []string) []EnvRefError
type envRef struct { raw, hostname, varName string }
func parseEnvRefs(s string) []envRef
// Phase 5 extracts a SHARED classifier used by both env_generate.go and deploy_validate.go.

// internal/workflow/compute_envelope.go (Phase 11 touches)
func resolveEnvelopeMode(meta *ServiceMeta, hostname string) topology.Mode

// internal/tools/workflow_adopt_local.go
func handleAdoptLocal(ctx, client, projectID, stateDir string, input WorkflowInput, rt runtime.Info)
// Phase 9 adds optional LinkedCloseMode handling.

// internal/tools/guard.go (Phase 12 touches)
func requireAdoption(stateDir string, recipeProbe RecipeSessionProbe, hostnames ...string) *mcp.CallToolResult

// internal/content/build_claude.go (Phase 10 touches)
func BuildClaudeMD(rt runtime.Info) (string, error)
func RefreshClaudeMD(path string, rt runtime.Info) (refreshed bool, err error)
```

---

## Phase 5 — Shared env-ref classifier with live-hostname matching

### Why

`internal/ops/env_generate.go::expandRefs` (Phase 4) classifies every
`${X_Y}` body as cross-service via first-underscore split — but two
patterns break:

1. **Top-level lone refs that happen to contain underscores** —
   `${STAGE_API_URL}` becomes `host=STAGE, var=API_URL`. If no service
   "STAGE" exists, top-level `FindService` errors with "service not
   found". Agent's actual intent was a project-level placeholder which
   should stay literal.
2. **Dash-bearing hostnames** — Zerops convention (per
   `internal/knowledge/guides/environment-variables.md:57`):
   "Hostname transformation: dashes become underscores. Service
   `my-db` variable `port` is `${my_db_port}`." `strings.Cut("my_db_port", "_")`
   reads `host=my, var=db_port` — wrong service, wrong var.

**Parallel path**: `internal/ops/deploy_validate.go::parseEnvRefs` has
the same broken split (`underIdx := strings.Index(inner, "_")`). It
backs `ValidateEnvReferences` which `internal/tools/deploy_preflight.go`
calls in BOTH container and local modes. Container deploy preflight is
also buggy for dash hostnames. Fix must cover both call sites.

### What

Extract a shared classifier into `internal/ops/env_refs.go` (new file)
keyed off live hostnames:

```go
// internal/ops/env_refs.go (new)

// EnvRefClassifier resolves ${name} bodies against a live-hostname
// set with Zerops's dash→underscore canonicalization. Constructed once
// per resolution batch so the live hostname list is fetched at most
// once.
type EnvRefClassifier struct {
    // hostUnderscoreNames maps the wire-form hostname (dashes replaced
    // with underscores) to the canonical hostname. Built from
    // ListProjectServices output.
    hostUnderscoreNames map[string]string
}

// NewEnvRefClassifier returns a classifier seeded with live hostnames.
// nil-safe: a nil classifier classifies every body as "lone" (returns
// false from Classify), letting callers degrade to literal handling.
func NewEnvRefClassifier(liveHostnames []string) *EnvRefClassifier {
    if len(liveHostnames) == 0 {
        return &EnvRefClassifier{}
    }
    m := make(map[string]string, len(liveHostnames))
    for _, h := range liveHostnames {
        m[strings.ReplaceAll(h, "-", "_")] = h
    }
    return &EnvRefClassifier{hostUnderscoreNames: m}
}

// Classify finds the longest live-hostname prefix in body. Returns
// (hostname, varName, true) when a cross-service ref is recognized;
// (zero, zero, false) when body is a lone ref. Longest-prefix avoids
// the dash-host bug (`my_db_port` returns "my-db", "port" if "my-db"
// is live; falls through if not).
func (c *EnvRefClassifier) Classify(body string) (hostname, varName string, ok bool) {
    if c == nil || len(c.hostUnderscoreNames) == 0 {
        return "", "", false
    }
    // Walk underscore boundaries from longest to shortest.
    for end := strings.LastIndex(body, "_"); end > 0; end = strings.LastIndex(body[:end], "_") {
        // candidate is body[:end] — try as hostname.
        if canonical, hit := c.hostUnderscoreNames[body[:end]]; hit && end < len(body)-1 {
            return canonical, body[end+1:], true
        }
        if end == 0 { break }
    }
    return "", "", false
}
```

Use the classifier in two places:

1. **`internal/ops/env_generate.go::expandRefs`**:
   - At start of `EnvGenerateDotenv`, call `client.ListServices` ONCE
     and build a `*EnvRefClassifier`. Also build a `servicesByHost`
     index `map[string]platform.ServiceStack` from the same response.
     Stash both on `refExpander`.
   - **`expandRefs` no longer calls `ListServices`.** It consults
     `servicesByHost` for cross-service ID lookups (lazy
     `GetServiceEnv` per host on first miss, cached afterwards).
     Single ListServices call per resolution batch.
   - Replace `strings.Cut(refBody, "_")` with
     `classifier.Classify(refBody)`. When `ok=false`:
     - If `sourceService != ""` (recursive): treat as sibling lookup
       against `sourceService` (current behavior).
     - If `sourceService == ""` (top level): leave literal (NEW
       behavior — was: split on first underscore + try resolution).
   - Pin via test asserting `client.ListServices` is called exactly
     once for a yaml with N cross-service refs to M distinct hosts:

   ```go
   func TestExpandRefs_ListServices_CalledOncePerResolutionBatch(t *testing.T) {
       // mock with WithCallCount("ListServices") wrapper
       // yaml with refs to db + cache + redis
       // assert call count == 1
   }
   ```

2. **`internal/ops/deploy_validate.go::parseEnvRefs`**:
   - Refactor `parseEnvRefs` to take a `*EnvRefClassifier` parameter.
   - Caller `ValidateEnvReferences` builds the classifier from its
     `liveHostnames` parameter.
   - Same Classify-or-skip logic. Lone refs are NOT recorded as
     `envRef` (no validation needed).
   - **Caveat for `zcp check env-refs`**: when called with empty
     `liveHostnames` (the shim-mode CLI path used by recipe linting),
     the classifier returns `ok=false` for every ref → all become
     "lone refs" → no validation errors raised. This loosens the
     shim's catch surface for malformed cross-service refs (e.g.
     `${db_password}` against an empty topology becomes a pass).
     **Decision** for this plan: accept the loosening. Document in
     `cmd/zcp/check/env-refs.go` help text that empty-topology mode
     skips host-shape validation. Rationale: live-topology mode (the
     primary use case) is strictly stronger; shim mode was an edge
     case that surfaces false positives anyway. If evidence later
     shows shim mode catching real bugs that live-topology misses, a
     separate `--strict-shape` flag can be added.

### RED tests

Add to `internal/ops/env_generate_test.go::TestEnvGenerateDotenv_ResolvesRefs`:

```go
{
    name: "regression: top-level lone ref with underscore stays literal when not a live host",
    zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        FOO: prefix-${SOME_PROJECT_VAR}-suffix
`,
    hostname:     "app",
    serviceEnvs:  map[string][]platform.EnvVar{}, // no service "SOME"
    wantVars:     1,
    wantServices: 0,
    wantContains: []string{"FOO=prefix-${SOME_PROJECT_VAR}-suffix"},
},
{
    name: "regression: dash-bearing hostname resolves via longest-prefix match",
    zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${my_db_hostname}
`,
    hostname: "app",
    serviceEnvs: map[string][]platform.EnvVar{
        // mock keys by canonical name (with dash); classifier maps
        // wire form "my_db" → canonical "my-db" before lookup.
        "my-db": {{ID: "e1", Key: "hostname", Content: "my-db"}},
    },
    wantVars:     1,
    wantServices: 1,
    wantContains: []string{"DB_HOST=my-db"},
},
```

Add to `internal/ops/deploy_validate_test.go`:

```go
func TestValidateEnvReferences_LonRefIgnored(t *testing.T) {
    envVars := map[string]string{
        "FOO": "${SOME_PROJECT_VAR}",  // not a live host; should not error
    }
    errs := ValidateEnvReferences(envVars, nil, []string{"db", "appdev"})
    if len(errs) != 0 {
        t.Fatalf("expected no errors for lone ref; got %v", errs)
    }
}

func TestValidateEnvReferences_DashHostnameLongestMatch(t *testing.T) {
    envVars := map[string]string{
        "FOO": "${my_db_port}",
    }
    discovered := map[string][]string{"my-db": {"port"}}
    errs := ValidateEnvReferences(envVars, discovered, []string{"my-db"})
    if len(errs) != 0 {
        t.Fatalf("expected no errors for valid dash-host ref; got %v", errs)
    }
}
```

### GREEN

Implement `EnvRefClassifier` + plumbing. Existing tests (single-word
hostnames) stay green: longest-prefix on `db_user` finds `db` as
hostname; `db` is in liveHostnames; returns `(db, user, true)` —
identical to first-underscore split outcome.

### Container regression

Add explicit container case to `TestEnvGenerateDotenv_ResolvesRefs`:

```go
{
    name: "container regression: typical container yaml with single-word hostname",
    zeropsYml: `zerops:
  - setup: apidev
    run:
      envVariables:
        DB_PASS: ${db_password}
        DB_HOST: ${db_hostname}
`,
    hostname: "apidev",
    serviceEnvs: map[string][]platform.EnvVar{
        "db": {
            {ID: "e1", Key: "password", Content: "secret"},
            {ID: "e2", Key: "hostname", Content: "db"},
        },
    },
    wantVars:     2,
    wantServices: 1,
    wantContains: []string{"DB_PASS=secret", "DB_HOST=db"},
},
```

`TestValidateEnvReferences_*` existing tests in deploy_validate_test.go
stay green — they all use single-word hostnames.

### Live verify

```bash
make build && ./bin/zcp eval behavioral run-local --id local-auto-adopt-node-postgres-first-deploy
```

Read `eval/behavioral/runs-local/<suite>/.../self-review.md`.
Targeted friction: agent yaml with `${STAGE_API_URL}` (or any
underscore-bearing top-level ref to a non-existent service) no longer
hard-errors at top level.

### Commit message hint

```
fix(ops): shared env-ref classifier with live-hostname matching

Phase 4's expandRefs split on first underscore unconditionally. Two
latent bugs the eval scenario didn't hit because eval-zcp uses
single-word hostnames:

  - Top-level ${SOME_PROJECT_VAR} (no live service "SOME") errored at
    top level instead of staying literal. Agents reaching for
    project-level vars in run.envVariables (anti-pattern but real)
    saw cryptic "service not found" errors.
  - Per Zerops env-vars guide §57: "dashes become underscores. Service
    `my-db` variable `port` is ${my_db_port}." First-underscore split
    reads host=my, var=db_port — wrong service, wrong var.

Same broken split lived in ops.parseEnvRefs (deploy_validate.go),
hitting both container and local deploy preflight. Now extracted as
ops.EnvRefClassifier with longest-prefix match over live hostnames
(canonicalized dash→underscore). Both call sites use the shared
classifier. Single ListServices call per resolution batch.

Container regression: single-word hostnames classify identically
under longest-prefix and first-underscore (longest match IS the only
match). All existing tests stay green; explicit container-shape test
case added to env_generate_test.go.
```

---

## Phase 6 — `.env` denylist for platform internals

### Why

`internal/ops/env_generate.go::EnvGenerateDotenv` appends every key from
`client.GetProjectEnv` into `.env` unless yaml-defined. Eval evidence
(suite `20260506-152718` etc.) shows `.env` containing:

- `ZCP_API_KEY` — the project-scoped DEPLOY TOKEN. Full project access.
- `apiCdnUrl`, `staticCdnUrl`, `storageCdnUrl` — Zerops CDN endpoints.
- `envIsolation`, `sshIsolation` — platform mode flags.
- `zeropsSubdomainHost`, `zeropsSubdomainString` — subdomain template
  fragments containing literal `${hostname}` / `${port}` placeholders
  (resolved at deploy time inside the container, not on local laptop).

User who fat-fingers `git add -A` lands `ZCP_API_KEY` in their git
history. `.gitignore` defends; defense-in-depth at generation is
correct.

`APP_KEY`, `APP_SECRET`, framework auto-secret vars: NOT denylisted.
These are application secrets the user's local app NEEDS to read. They
remain in `.env` (gitignored). The generated `.env` is still
secret-bearing — denylist filters platform/operator credentials only.

### What

Add a denylist in `internal/ops/env_generate.go`:

```go
// platformInternalKeys is the set of project-level env keys the .env
// generator refuses to copy into local .env files. They serve only the
// platform/operator surface; surfacing them locally risks accidental
// commit of the deploy token or leaking platform CDN endpoints into
// app config. Yaml-defined refs are exempt — the user wrote that
// reference deliberately.
//
// NOT in this list: APP_KEY, APP_SECRET, framework auto-secrets. Those
// are application secrets the user's local app reads; .env stays
// secret-bearing and must remain in .gitignore.
var platformInternalKeys = map[string]bool{
    "ZCP_API_KEY":           true, // project-scoped deploy token
    "envIsolation":          true, // platform mode flag
    "sshIsolation":          true, // platform mode flag
    "apiCdnUrl":             true, // CDN endpoint
    "staticCdnUrl":          true, // CDN endpoint
    "storageCdnUrl":         true, // CDN endpoint
    "zeropsSubdomainHost":   true, // platform subdomain prefix
    "zeropsSubdomainString": true, // template (`${hostname}-${port}.zerops.app`)
}
```

Apply in the project-env append loop. Surface omitted keys on the
result + as a comment in the generated `.env`:

```go
type EnvDotenvResult struct {
    Path                string   `json:"path"`
    Services            int      `json:"services"`
    Variables           int      `json:"variables"`
    VPNHint             string   `json:"vpnHint,omitempty"`
    OmittedPlatformKeys []string `json:"omittedPlatformKeys,omitempty"`
}
```

`.env` header comment lists what was filtered (so the agent can choose
to fetch them explicitly via `zerops_env action="get"` if a specific
internal key is genuinely needed).

### RED tests

```go
{
    name: "platform internal vars are filtered from .env",
    zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
`,
    hostname: "app",
    serviceEnvs: map[string][]platform.EnvVar{
        "db": {{ID: "e1", Key: "hostname", Content: "db"}},
    },
    projectEnvs: []platform.EnvVar{
        {ID: "p1", Key: "ZCP_API_KEY", Content: "secret-token"},
        {ID: "p2", Key: "APP_NAME", Content: "myapp"},
        {ID: "p3", Key: "zeropsSubdomainHost", Content: "abc1"},
    },
    wantVars:     2, // DB_HOST + APP_NAME (ZCP_API_KEY + zeropsSubdomainHost denied)
    wantServices: 1,
    wantContains: []string{"DB_HOST=db", "APP_NAME=myapp"},
    // Assert assignment lines are absent (header may mention key names
    // for discoverability — assertion targets `KEY=VALUE` lines only).
    wantNotContainsLines: []string{"ZCP_API_KEY=", "zeropsSubdomainHost="},
},
{
    name: "yaml-defined ref overrides denylist (user explicit)",
    zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        ZCP_API_KEY: explicit-user-value
`,
    hostname:     "app",
    serviceEnvs:  map[string][]platform.EnvVar{},
    projectEnvs:  []platform.EnvVar{{Key: "ZCP_API_KEY", Content: "would-be-denied"}},
    wantVars:     1,
    wantServices: 0,
    wantContains: []string{"ZCP_API_KEY=explicit-user-value"},
},
```

The `wantNotContainsLines` test field is new — extend the test runner
to assert per-line absence (regex over whole `.env` content matching
`^KEY=`). Header text may legitimately reference omitted key names so
substring assertion is too coarse.

### GREEN

Implement filter + result field + `.env` header. Update existing test
runner with `wantNotContainsLines` assertion.

### Container regression

`generate-dotenv` is registered in container too (`internal/server/server.go::registerTools`
calls `RegisterEnv` unconditionally). Container `.env` flow is rare in
practice (env vars are auto-injected into container env), but the
denylist applies symmetrically — same internals are platform-internal
in both modes.

Add explicit container case:

```go
{
    name: "container regression: managed-service refs survive denylist",
    // setup: apidev with run.envVariables: DB_HOST: ${db_hostname}
    // projectEnvs without platform internals
    // Assert .env content matches pre-denylist behavior
},
```

### Live verify

After `flow-eval-local`, parse the agent's transcript for the
generate-dotenv result, assert `OmittedPlatformKeys` is non-empty
when project has platform internals (eval-zcp does), and `ZCP_API_KEY`
is NOT in the resulting `.env` body.

Wording in the agent-facing description: ".env still contains app
secrets like APP_KEY and DB passwords — keep .env in .gitignore."

### Commit message hint

```
fix(env_generate): deny platform-internal keys from .env output

GetProjectEnv returns ALL project-level env vars including
ZCP_API_KEY (the project-scoped deploy token), envIsolation, *CdnUrl,
and zeropsSubdomain* — none of which belong in a local .env. A user
who fat-fingers `git add -A` despite .gitignore would publish the
deploy token.

Adds a denylist applied to the project-env append loop. Yaml-defined
refs are exempt (user wrote them deliberately). The .env header lists
omitted keys for discoverability; result.omittedPlatformKeys carries
the same list to the wire.

NOT denied: APP_KEY, APP_SECRET, framework auto-secrets. Those are
application secrets local apps read; .env remains secret-bearing
and must stay in .gitignore.

Container generate-dotenv path uses the same denylist (symmetric).
Existing test runner gains wantNotContainsLines assertion so header
text mentioning omitted key names doesn't false-positive.
```

---

## Phase 7 — Failed-deploy recovery: structured retry hint + populate envVars

### Why

`internal/tools/import.go::gateOverrideOnFailedHistory` correctly
enforces diagnose-before-destruct (CLAUDE.md invariant). Two issues:

1. **`envVarsByService` declared but never populated** (line 129):
   `envVarsByService := make(map[string][]string)` is created, the
   loop iterates `failedTargets` but only writes to `failedTargets`
   slice, never to `envVarsByService`. Then
   `collectEnvVarKeys(envVarsByService, failedTargets)` reads it,
   always returns empty. `wouldDestroy.wouldDestroy.envVars` is
   ALWAYS empty in the wire response. Agents see "env var loss: none"
   when override may delete real keys.

2. **Recovery has no schema scaffold for the retry**: agent gets
   `wouldDestroy` payload, reads logs (per recovery hint), THEN has
   to hand-construct `confirmDestructive` from the wouldDestroy
   shape. `topology.Recovery.Args` is `map[string]string` — can't
   nest `confirmDestructive: {operation, acknowledgedTargets}`
   directly.

### What

Two changes:

1. **Populate `envVarsByService`** before constructing `wouldDestroy`:

```go
for _, hostname := range failedTargets {
    svc, err := ops.LookupService(ctx, client, projectID, hostname)
    if err != nil || svc == nil {
        continue // best-effort — empty envVars list for this target is
                  // better than failing the gate over a transient API hiccup
    }
    envs, err := ops.FetchServiceEnv(ctx, client, svc.ID)
    if err != nil {
        continue
    }
    keys := make([]string, 0, len(envs))
    for _, e := range envs {
        keys = append(keys, e.Key)
    }
    envVarsByService[hostname] = keys
}
```

(`ops.LookupService`, NOT `ops.LookupServiceID` — the latter doesn't
exist. Lookup returns `*platform.ServiceStack`, get `.ID`.)

2. **Add structured retry hint to topology.Recovery** (additive, old
   wire stays valid):

```go
// internal/topology/recovery.go

// SuggestedRetry describes the next call after a recovery action
// completes. Used when the natural next step is a retry of the same
// or different tool with structured arguments — typical for
// diagnose-before-destruct gates where the second call needs an
// acknowledgment payload too rich to fit in Recovery.Args (which is
// map[string]string only).
type SuggestedRetry struct {
    Tool string         `json:"tool"`
    Args map[string]any `json:"args"` // structured (any JSON value)
    Note string         `json:"note,omitempty"`
}

type Recovery struct {
    Tool           string            `json:"tool"`
    Action         string            `json:"action"`
    Args           map[string]string `json:"args,omitempty"`
    SuggestedRetry *SuggestedRetry   `json:"suggestedRetry,omitempty"` // NEW
}
```

Augment the import gate's recovery to attach a suggestedRetry:

```go
return convertError(validateErr,
    WithWouldDestroy(&expected),
    WithRecovery(&topology.Recovery{
        Tool:   "zerops_logs",
        Action: "fetch",
        Args:   map[string]string{
            "serviceHostname": failedTargets[0],
            "since":           "15m",
        },
        SuggestedRetry: &topology.SuggestedRetry{
            Tool: "zerops_import",
            Args: map[string]interface{}{
                "filePath":  input.FilePath,  // (or content if filePath empty)
                "content":   input.Content,
                "override":  true,
                "confirmDestructive": map[string]interface{}{
                    "operation":           expected.Operation,
                    "acknowledgedTargets": expected.Targets,
                },
            },
            Note: "After reading logs, retry import with this payload to acknowledge destruction.",
        },
    }),
), nil
```

(The `facility` arg is dropped — see Phase 8.)

### RED tests

```go
func TestGateOverrideOnFailedHistory_PopulatesEnvVarLoss(t *testing.T) {
    // Mock platform: one failed target with env vars (e.g. APP_KEY, DB_URL).
    // Call gateOverrideOnFailedHistory.
    // Assert wouldDestroy.wouldDestroy.envVars contains expected keys.
    //
    // Note the wire path is wouldDestroy.wouldDestroy.envVars — outer
    // is the field tag, inner is the embedded loss struct's tag (also
    // "wouldDestroy" by design — refactor candidate but out of scope here).
}

func TestGateOverrideOnFailedHistory_RecoveryIncludesSuggestedRetry(t *testing.T) {
    // Call the gate with no confirmDestructive — assert the error
    // wire's recovery.suggestedRetry block exists with:
    //   - tool=zerops_import
    //   - args.confirmDestructive.operation == expected operation
    //   - args.confirmDestructive.acknowledgedTargets == expected targets
}

func TestSuggestedRetry_OmitsWhenNoRetryHint(t *testing.T) {
    // Other recovery paths (non-import gates) MUST NOT carry a
    // SuggestedRetry — assert default Recovery still serializes
    // without the new field.
}
```

### GREEN

Implement type + populate + retry-hint construction. Existing
`TestImport_OverrideOnFailedRequiresAck` updated (envVars now
populated; retry hint asserted).

### Container regression

The import gate runs in container too. Behavior changes are:
- `wouldDestroy.envVars` is now non-empty when target services have
  env vars (was always empty).
- Recovery carries a new optional field.

Both are additive enrichment. Existing container imports keep
working; the new fields just give container agents more guidance too.

Add explicit container test:

```go
func TestGateOverrideOnFailedHistory_ContainerEnv_SameEnrichment(t *testing.T) {
    // runtime.Info{InContainer: true}
    // Assert envVars populated + suggestedRetry present (same wire shape)
}
```

### Live verify

`flow-eval-local` will reproduce the npm-ci-no-lockfile failure path
(out of scope for fixing — recipe-knowledge concern). Confirm:
1. Agent's `zerops_import override:true` rejection error has
   `wouldDestroy.wouldDestroy.envVars` populated with actual keys.
2. Recovery has `suggestedRetry` block.
3. If agent calls the suggested retry with the suggested args, gate
   passes.

### Commit message hint

```
fix(import): populate wouldDestroy.envVars + add structured retry hint

gateOverrideOnFailedHistory had a latent bug and a UX gap:

  - envVarsByService was declared as an empty map and never written
    to. collectEnvVarKeys always returned []. wouldDestroy reported
    env var loss as empty when override could delete real keys.
    Now populated via ops.LookupService + ops.FetchServiceEnv per
    failed target (best-effort: an API hiccup leaves that target's
    list empty rather than failing the gate).

  - Recovery hint pointed at zerops_logs only. After reading logs
    agents had to reconstruct confirmDestructive's nested shape
    from the wouldDestroy payload. topology.Recovery.Args is
    map[string]string — too narrow for the structured ack.

    Added topology.SuggestedRetry (optional, additive). The import
    gate now attaches a suggestedRetry pointing at the next
    zerops_import call with confirmDestructive pre-shaped from the
    wouldDestroy payload. Agent still passes the value explicitly
    (gate intent preserved); no schema-guessing.

Diagnose-before-destruct contract intact (acknowledgment still
required; no auto-bypass). Wire path note: wouldDestroy.envVars is
actually nested at wouldDestroy.wouldDestroy.envVars due to existing
JSON tags (both outer field and inner loss struct tagged
"wouldDestroy"). Test assertions target the actual wire path. A
rename to wouldDestroy.loss is filed as a separate follow-up
(recovery contract test changes).
```

---

## Phase 8 — Doc-only: drop facility hint + correct generate-dotenv schema

### Why

Two pre-existing inaccuracies that surfaced repeatedly in eval
retrospectives:

1. **`internal/tools/import.go:159-166`** — recovery hint args include
   `"facility": "application"`. But `LogsInput`
   (`internal/tools/logs.go`) has no `Facility` field. Agent passes it
   → MCP rejects. Plus `ops.FetchLogs` already hardcodes
   `Facility: "application"` (`internal/ops/logs.go`), so the param
   is doubly redundant.

2. **`internal/tools/env.go::envInputSchema`** — schema description
   for `serviceHostname` says "Ignored by generate-dotenv (which reads
   zerops.yaml instead)." Implementation requires `serviceHostname`
   and rejects empty with `INVALID_PARAMETER`.

### What

1. Drop `"facility"` from RecoveryHint args in `import.go` (Phase 7's
   recovery construction already omits it; this phase removes any
   remaining call site that adds it).
2. Rewrite `serviceHostname` schema description: "Required by
   generate-dotenv to identify which setup block in zerops.yaml's
   run.envVariables to resolve."

### RED tests

```go
func TestImport_RecoveryHint_NoFacilityArg(t *testing.T) {
    // Trigger DIAGNOSIS_REQUIRED gate; assert recovery.args has no "facility" key
}

func TestEnvSchema_ServiceHostname_MatchesRequiredness(t *testing.T) {
    // Load envInputSchema; for action="generate-dotenv" the description
    // for serviceHostname does NOT contain "Ignored" and DOES contain
    // language matching its required-ness.
}
```

### GREEN

Trivial doc/data fixes.

### Container regression

Pure data fixes; no behavior change. Existing `TestEnvToolDescription*`
(if present) still passes.

### Live verify

After `flow-eval-local`, confirm:
1. Agents calling generate-dotenv without serviceHostname see a
   description that signals required-ness; they're more likely to
   pass it on first call.
2. Agents reading import recovery don't try to pass a `facility` arg
   to zerops_logs.

### Commit message hint

```
docs(tools): drop facility from import recovery + correct generate-dotenv schema

Two pre-existing inaccuracies that surfaced in flow-eval-local
retrospectives 4 and 6:

  - import.go's diagnose-required recovery hint passed
    args:{facility:"application"}, but zerops_logs has no Facility
    field. Agents tried to pass it, got "unknown property"
    rejection, lost the actionable hint. Drop facility (FetchLogs
    already hardcodes application server-side).

  - env.go's serviceHostname schema description said "Ignored by
    generate-dotenv (reads zerops.yaml)". Implementation requires
    it. Description now matches behavior.

Pinned by lint-style assertion tests so future drift is caught.
```

---

## Phase 9 — adopt-local: explicit close-mode semantics

### Why

Two related defects in `LocalAutoAdopt` and `handleAdoptLocal`:

1. **Inconsistent CloseDeployMode across LocalAutoAdopt branches**:
   - Zero-runtime branch (`internal/workflow/adopt_local.go:89-110`)
     writes `CloseDeployMode: CloseModeManual` + `CloseDeployModeConfirmed: true`.
   - Multi-runtime branch (line 135-153) writes neither field
     (CloseDeployMode unset).

2. **`handleAdoptLocal` doesn't set CloseDeployMode** when upgrading
   `local-only` → `local-stage`. Auto-close gate
   (`internal/workflow/work_session.go::autoCloseGateOpen`) reads
   `CloseDeployMode`; unset blocks auto-close. Agent has to call
   `action="close-mode"` separately. Run #6 retrospective:
   "Bootstrap leaves close-mode as `unset`, which silently blocks
   auto-close — easy to miss."

### What

**Decision** (resolving Codex review item #9):

- **`local-only` always = manual.** Both LocalAutoAdopt branches set
  `CloseDeployMode: CloseModeManual` + `Confirmed: true`. Spec §4
  promise honored consistently. Deploy gate
  (`checkLocalOnlyGate`) already refuses default-deploy on
  local-only (no stage to push to); manual is the only consistent
  choice.

- **`adopt-local` upgrade defaults to `auto`.** When `handleAdoptLocal`
  upgrades to local-stage with linked stage hostname, set
  `CloseDeployMode: CloseModeAuto` + `Confirmed: true`. Auto-close
  fires on the typical happy path (link → develop → deploy → verify
  → close). Agent can override via the new optional input field.

- **Optional override input field**: add `LinkedCloseMode string`
  (NOT `CloseMode` — clashes with existing
  `CloseModes map[string]string json:"closeMode"`). JSON tag
  `linkedCloseMode`, applies only to `action="adopt-local"`.

```go
// internal/tools/workflow.go::WorkflowInput
LinkedCloseMode string `json:"linkedCloseMode,omitempty" jsonschema:"Optional close-deploy-mode for action=adopt-local. Defaults to 'auto' when upgrading local-only → local-stage. Valid values: auto, git-push, manual."`

// internal/tools/workflow_adopt_local.go::handleAdoptLocal
// Validation matches the existing pattern in handleCloseMode — there
// is no shared parseCloseMode helper today. If you prefer, extract
// one as a refactor step in this same commit; otherwise inline.
linkedMode := topology.CloseModeAuto
if input.LinkedCloseMode != "" {
    candidate := topology.CloseDeployMode(input.LinkedCloseMode)
    if !validCloseModes[candidate] {  // validCloseModes is the map already declared in workflow_close_mode.go
        return convertError(platform.NewPlatformError(
            platform.ErrInvalidParameter,
            fmt.Sprintf("Invalid linkedCloseMode %q", input.LinkedCloseMode),
            "Valid values: auto, git-push, manual"), WithRecoveryStatus()), nil, nil
    }
    linkedMode = candidate
}
local.Mode = topology.PlanModeLocalStage
local.StageHostname = input.TargetService
local.CloseDeployMode = linkedMode
local.CloseDeployModeConfirmed = true
```

### RED tests

```go
func TestLocalAutoAdopt_MultiRuntime_WritesManualCloseMode(t *testing.T) {
    // mock 2 runtimes; call LocalAutoAdopt; assert resulting meta has
    // CloseDeployMode == CloseModeManual + Confirmed == true.
}

func TestHandleAdoptLocal_DefaultClosesModeAuto(t *testing.T) {
    // Pre-existing local-only meta; call handleAdoptLocal with no
    // LinkedCloseMode; assert resulting meta has CloseDeployMode == CloseModeAuto.
}

func TestHandleAdoptLocal_ExplicitGitPush(t *testing.T) {
    // Call with LinkedCloseMode = "git-push"; assert persisted.
}

func TestHandleAdoptLocal_InvalidLinkedCloseMode_Rejected(t *testing.T) {
    // Call with LinkedCloseMode = "bogus"; assert INVALID_PARAMETER.
}
```

### GREEN

Implement the changes. Update the existing `TestLocalAutoAdopt_*` tests
(they may currently assert unset close-mode for multi-runtime branch).

### Container regression

`handleAdoptLocal` refuses in container env (early-return at
`internal/tools/workflow_adopt_local.go:28-34`). `LocalAutoAdopt` is
gated by `!rtInfo.InContainer` in `internal/server/server.go:78`.
Container path does not exercise these functions. Test pin:

```go
func TestHandleAdoptLocal_ContainerRefusalUnchanged(t *testing.T) {
    // runtime.Info{InContainer: true}; call handleAdoptLocal; assert
    // refusal with INVALID_PARAMETER.
}
```

### Live verify

`flow-eval-local` reproduces the multi-runtime adopt scenario.
Confirm:
1. After auto-adopt, on-disk meta has `closeDeployMode: "manual"`.
2. After agent calls `adopt-local`, meta has
   `closeDeployMode: "auto"` (defaulted).
3. Auto-close fires on deploy + verify success without separate
   close-mode call (collapses 2-step dance into 1).

### Commit message hint

```
fix(adopt_local): unified close-mode semantics + adopt-local default auto

Three related issues:

  - LocalAutoAdopt's zero-runtime branch wrote CloseDeployMode=manual
    consistently; multi-runtime branch silently left it unset. Spec
    §4 says local-only forces manual; both branches now match.

  - handleAdoptLocal upgraded local-only → local-stage but left
    CloseDeployMode unset, blocking the auto-close gate. Agents had
    to call action=close-mode separately (2-step dance). Now defaults
    to CloseModeAuto when upgrading.

  - Optional input field LinkedCloseMode (JSON: linkedCloseMode) lets
    the agent override the default for git-push or manual flows. Field
    name avoids clash with existing WorkflowInput.CloseModes
    (map[string]string with json:"closeMode").

Container path untouched: handleAdoptLocal refuses in container env
explicitly; LocalAutoAdopt is gated on !rtInfo.InContainer at server
startup. Both gates pinned by existing tests.
```

---

## Phase 10 — Adoption note durability + CLAUDE.md refresh symmetry

### Why

Two parallel asymmetries in `internal/server/server.go`:

1. **Adoption note disappears on second+ server start.**
   `runLocalAutoAdopt` calls `LocalAutoAdopt`, which returns
   `(nil, nil)` (no-op sentinel) when any meta exists. The wrapper
   returns "" (empty note). MCP instructions block has no adoption
   appendix. Agents joining an already-adopted local project get NO
   guidance about topology / unlinked runtimes / `adopt-local`
   availability.

2. **CLAUDE.md auto-refresh is container-only.**
   Lines 89-96: `if rtInfo.InContainer && stateDir != "" { content.RefreshClaudeMD(...) }`.
   Local users with stale templates from an earlier `zcp` install use
   outdated guidance silently. Container has the symmetric benefit.

`internal/content/build_claude.go::BuildClaudeMD` doc-comment says env
is stable per install and re-render is via `zcp init`. The container
exception was added because long-lived containers couldn't easily
re-run `zcp init`. Same logic applies to local: users start `zcp`
from `.mcp.json` config without re-running `zcp init`. Extending the
refresh is symmetric, not a doctrine break.

### What

1. **`runLocalAutoAdopt` always emits a current-state note**:
   - On first call (no metas): existing auto-adopt path returns
     `FormatAdoptionNote(result)` (current behavior).
   - On subsequent calls (meta exists): build a current-state note
     from the existing meta + live services.

```go
// internal/workflow/adopt_local.go (new helper)

// FormatLocalStateNote builds a deterministic current-state note for
// MCP instructions. Always non-empty when called with a local meta.
// Surfaces adopt-local prereq prominently when local-only with
// candidate runtimes.
func FormatLocalStateNote(metas []*ServiceMeta, liveServices []platform.ServiceStack, projectName string) string {
    local := findLocalMeta(metas)
    if local == nil {
        return ""
    }
    // Project-name fallback: server.go's GetProject is best-effort and
    // can return empty string on transient failures. The local meta's
    // Hostname IS the project name (set by LocalAutoAdopt to
    // project.Name), so this is a safe fallback that keeps the note
    // readable when the live API call missed.
    if projectName == "" {
        projectName = local.Hostname
    }
    runtimes, managed := classifyServices(liveServices)
    
    switch {
    case local.Mode == topology.PlanModeLocalStage:
        return formatLocalStageNote(local, runtimes, managed, projectName)
    case len(runtimes) > 1:
        return formatLocalOnlyMultiRuntimeNote(local, runtimes, managed, projectName)
    case len(runtimes) == 0:
        return formatLocalOnlyZeroRuntimeNote(local, managed, projectName)
    default:
        // Exactly 1 runtime + still local-only: ambiguous state
        // (LocalAutoAdopt wrote local-only when 2+ existed, but one
        // was deleted). Treat as candidate-for-adoption.
        return formatLocalOnlyMultiRuntimeNote(local, runtimes, managed, projectName)
    }
}
```

The local-only multi-runtime note phrasing leads with the actionable
recovery (Codex review item #10):

```
Adopted project "<project>" as local-only. Multiple Zerops runtimes exist
(<runtime list>). BEFORE running `develop`, link one as the local stage:
  zerops_workflow action="adopt-local" targetService="<runtime>"
This sets the project's stage target so develop + auto-close work.
Managed services: <managed list>. Run `zcli vpn up <projectId>` for
local dev-time access.
```

Update `runLocalAutoAdopt`:

```go
func runLocalAutoAdopt(ctx, client, projectID, stateDir string, logger *slog.Logger) string {
    services, _ := client.ListServices(ctx, projectID)  // best-effort; nil OK
    project, _ := client.GetProject(ctx, projectID)     // best-effort
    projectName := ""
    if project != nil { projectName = project.Name }

    existing, err := workflow.ListServiceMetas(stateDir)
    if err != nil {
        logger.Warn("auto-adopt: list metas failed", "err", err)
        return ""
    }

    if len(existing) == 0 {
        // First call — perform adoption + format the result note.
        result, err := workflow.LocalAutoAdopt(ctx, client, projectID, stateDir)
        if err != nil {
            logger.Warn("auto-adopt: failed", "err", err)
            return ""
        }
        return workflow.FormatAdoptionNote(result)
    }

    // Subsequent call — re-render current state.
    return workflow.FormatLocalStateNote(existing, services, projectName)
}
```

2. **Extend `RefreshClaudeMD` call to local** (drop the
   `rtInfo.InContainer` gate):

```go
if stateDir != "" {
    claudemdPath := filepath.Join(filepath.Dir(filepath.Dir(stateDir)), "CLAUDE.md")
    if refreshed, err := content.RefreshClaudeMD(claudemdPath, rtInfo); err != nil {
        logger.Warn("CLAUDE.md refresh failed", "path", claudemdPath, "err", err)
    } else if refreshed {
        logger.Info("CLAUDE.md refreshed from embedded template", "path", claudemdPath)
    }
}
```

Update `internal/content/build_claude.go` doc-comment to reflect that
serve-time refresh is now done in BOTH envs (idempotent: no-op when
disk content matches embedded template).

### RED tests

```go
func TestRunLocalAutoAdopt_SecondCall_StillSurfacesNote(t *testing.T) {
    // First call: write meta. Second call: assert note is non-empty
    // and contains expected current-state phrasing
    // (e.g. "Adopted project ... as local-only").
}

func TestFormatLocalStateNote_LocalOnlyMultiRuntime_LeadsWithAdoptLocal(t *testing.T) {
    // Build metas + live services with multiple runtimes; call helper.
    // Assert "BEFORE running `develop`" + "adopt-local" appear before
    // any close-mode discussion.
}

func TestServerNew_LocalEnv_RefreshesClaudeMD(t *testing.T) {
    // Mock: write old CLAUDE.md template, call server.New with
    // InContainer=false; assert CLAUDE.md was refreshed (matches
    // embedded template).
}

func TestServerNew_ContainerEnv_StillRefreshesClaudeMD(t *testing.T) {
    // Container regression: same path with InContainer=true keeps
    // working as before.
}
```

### GREEN

Implement `FormatLocalStateNote` + restructure `runLocalAutoAdopt` +
drop container-gate from `RefreshClaudeMD` call. Update
`build_claude.go` doc-comment.

### Container regression

CLAUDE.md refresh in container was already happening; extending to
local doesn't change container behavior. Adoption note re-emission
is local-only logic (`runLocalAutoAdopt` is gated by
`!rtInfo.InContainer` at server.go:78).

Pin via existing `TestRefreshClaudeMD_*` tests (expand if needed).

### Live verify

`flow-eval-local` second-run on a project that's already been adopted
(workdir reused via `--scenarios-dir` override or manual replay).
Confirm:
1. Adoption note is non-empty in MCP instructions.
2. `adopt-local` recovery is mentioned BEFORE close-mode options.
3. CLAUDE.md content matches current template.

### Commit message hint

```
fix(server): durable adoption note + symmetric CLAUDE.md refresh

Two parallel asymmetries:

  - runLocalAutoAdopt returned "" once any meta existed. Agents
    joining an already-adopted local project saw no MCP-instruction
    guidance about topology / unlinked runtimes / adopt-local
    availability. Note now re-renders current state on every server
    start via FormatLocalStateNote(metas, services, projectName).
    Local-only multi-runtime phrasing leads with the actionable
    "BEFORE running develop" recovery (closes the gap evals 4-6
    flagged: "adopt-local prereq surfaces too late").

  - RefreshClaudeMD was gated on rtInfo.InContainer; local users
    with stale CLAUDE.md from an earlier zcp install used outdated
    guidance silently. The container-only gate was an artifact of
    long-lived containers being hard to manually re-init; local
    users have the same gap (zcp starts from .mcp.json without
    re-running init). Now extended to local. build_claude.go
    doc-comment updated to match.

Container behavior identical: refresh logic was already running
there. Adoption note remains local-only.
```

---

## Phase 11 — Local-mode envelope projection (C1)

### Why

Atoms with `modes: [..., local-stage]` or `[..., local-only]`
**silently never match** for local runtimes. Six atoms affected:

| Atom | Filter |
|---|---|
| `develop-build-observe.md` | `[standard, simple, local-stage, local-only]` |
| `develop-first-deploy-execute.md` | `[dev, simple, standard, local-stage]` |
| `develop-ready-to-deploy.md` | `[dev, simple, standard, local-stage]` |
| `develop-dynamic-runtime-start-local.md` | `[dev, standard, local-stage, local-only]` |
| `develop-close-mode-git-push.md` | `[standard, simple, local-stage, local-only]` |
| `develop-close-mode-git-push-needs-setup.md` | `[standard, simple, local-stage, local-only]` |

Cause: `internal/workflow/compute_envelope.go::resolveEnvelopeMode`
maps a local-stage runtime's stageHostname through `RoleFor` →
`DeployRoleStage` → `topology.ModeStage`. The snapshot `Mode` field
becomes `"stage"`, not `"local-stage"`. Atoms filtered to
`[..., local-stage]` (without including `stage`) don't match.

For local-only: no live service maps to the project meta's hostname
(project name is not a service name). `metaByHost[svc.Name]` returns
nil for every live service; no snapshot gets `Mode = "local-only"`.
Atoms with `[..., local-only]` never match because no snapshot
projects as that mode.

**Container regression check**: `grep` confirmed ZERO atoms have
`modes: [stage]` standalone or any combination including `stage` but
NOT `local-stage`. So changing local-stage's projection from
`ModeStage` to `ModeLocalStage` doesn't strand any container-stage
atom (none exist that depend on the mis-projection).

### What

Two changes:

1. **Projection of stage hostname respects local meta mode**:

```go
// internal/workflow/compute_envelope.go::resolveEnvelopeMode

func resolveEnvelopeMode(meta *ServiceMeta, hostname string) topology.Mode {
    if meta == nil {
        return ""
    }
    role := meta.RoleFor(hostname)
    switch role {
    case topology.DeployRoleStage:
        // For local-stage meta, project the stage hostname as
        // ModeLocalStage. Container-standard's stage half stays
        // ModeStage. No atom with `modes: [stage]` exists without
        // `local-stage` companion, so the swap doesn't strand any
        // existing axis filter (verified via grep at plan time).
        if meta.Mode == topology.PlanModeLocalStage {
            return topology.ModeLocalStage
        }
        return topology.ModeStage
    case topology.DeployRoleSimple:
        return topology.ModeSimple
    case topology.DeployRoleDev, topology.PlanModeStandard, topology.PlanModeLocalStage, topology.PlanModeLocalOnly:
        if meta.Mode == topology.PlanModeStandard {
            return topology.ModeStandard
        }
        return topology.ModeDev
    }
    return ""
}
```

2. **Synthetic snapshot for local-only project meta**:

For local-only metas, no live service projects as ModeLocalOnly. Add
a synthetic snapshot in `buildServiceSnapshots` so atoms with
`modes: [..., local-only]` AND no `runtimes:` axis filter can match:

```go
// internal/workflow/compute_envelope.go::buildServiceSnapshots

// After the live-services loop, append synthetic snapshots for
// local-only project metas. The synthetic carries Mode + close/push
// state so mode-only and close-mode-only atom axes can match. It
// does NOT carry RuntimeClass or live status — local-only by
// definition has no linked runtime to bind those to. Atoms with
// `runtimes:` axis filter intentionally do NOT match local-only;
// runtime-specific guidance is premature until the user runs
// adopt-local (which transitions to local-stage and the live
// runtime then projects with full axis coverage).
for _, m := range metas {
    if m == nil { continue }
    if m.Mode != topology.PlanModeLocalOnly { continue }
    out = append(out, ServiceSnapshot{
        Hostname:        m.Hostname,
        Mode:            topology.Mode(m.Mode),  // "local-only"
        Bootstrapped:    m.IsComplete(),
        CloseDeployMode: m.CloseDeployMode,
        GitPushState:    m.GitPushState,
        // No RuntimeClass / TypeVersion / Status — local-only project
        // hasn't picked a runtime yet.
    })
}
sort.Slice(out, func(i, j int) bool { return out[i].Hostname < out[j].Hostname })
```

Local-stage gets nothing synthetic — its linked stageHostname IS a
live service, the existing `buildServiceSnapshots` loop builds its
snapshot, and Phase 11's projection change makes that snapshot's
`Mode = ModeLocalStage`. RuntimeClass + Status come from the live
ServiceStack normally.

### Atom coverage table (post-Phase 11)

Documenting which atoms fire for which local mode after the fix:

| Atom | Filter | local-stage (linked dynamic runtime) | local-only (no linked runtime) |
|---|---|---|---|
| `develop-build-observe.md` | `modes: [..., local-stage, local-only]` (no runtimes filter) | ✓ via live runtime snapshot | ✓ via synthetic |
| `develop-first-deploy-execute.md` | `modes: [dev, simple, standard, local-stage]` (no `local-only`, no runtimes filter) | ✓ via live runtime snapshot | — by design (no `local-only` in filter) |
| `develop-ready-to-deploy.md` | same as above | ✓ | — by design |
| `develop-close-mode-git-push.md` | `modes: [..., local-stage, local-only]` (no runtimes filter) | ✓ | ✓ |
| `develop-close-mode-git-push-needs-setup.md` | same | ✓ | ✓ |
| `develop-dynamic-runtime-start-local.md` | `modes: [..., local-stage, local-only]` + `runtimes: [dynamic]` | ✓ for dynamic-class linked runtime | — by design (no live runtime to bind RuntimeClass to; user runs adopt-local first to transition to local-stage with classified runtime) |

**Phase 11's claim is narrowed accordingly**: 5 of 6 atoms gain
correct firing for local-stage; 4 of 6 gain correct firing for
local-only. The `develop-dynamic-runtime-start-local` atom for
local-only project remains intentionally inert because the project
has no linked runtime to classify.

### Atom audit reproducibility

Verify the container-safety claim ("zero atoms have `modes: [stage]`
without `local-stage`") with this command:

```bash
grep -rE "modes:.*\bstage\b" internal/content/atoms/*.md | grep -v "local-stage" || echo "none — Phase 11 safe"
```

### RED tests

```go
func TestResolveEnvelopeMode_LocalStage_ProjectsAsModeLocalStage(t *testing.T) {
    meta := &ServiceMeta{Hostname: "myproject", StageHostname: "app", Mode: topology.PlanModeLocalStage}
    if got := resolveEnvelopeMode(meta, "app"); got != topology.ModeLocalStage {
        t.Errorf("expected ModeLocalStage, got %q", got)
    }
}

func TestResolveEnvelopeMode_ContainerStandard_StillProjectsAsModeStage(t *testing.T) {
    meta := &ServiceMeta{Hostname: "appdev", StageHostname: "appstage", Mode: topology.PlanModeStandard}
    if got := resolveEnvelopeMode(meta, "appstage"); got != topology.ModeStage {
        t.Errorf("expected ModeStage (container regression), got %q", got)
    }
}

func TestBuildServiceSnapshots_LocalOnly_AppendsSyntheticSnapshot(t *testing.T) {
    metas := []*ServiceMeta{{Hostname: "myproject", Mode: topology.PlanModeLocalOnly, BootstrappedAt: "2026-05-06"}}
    services := []platform.ServiceStack{
        {Name: "db", ServiceStackTypeInfo: platform.ServiceTypeInfo{ServiceStackTypeVersionName: "postgresql@16"}},
    }
    snaps := buildServiceSnapshots(services, metas, nil, "")
    var foundProject bool
    for _, s := range snaps {
        if s.Hostname == "myproject" && s.Mode == topology.ModeLocalOnly {
            foundProject = true
        }
    }
    if !foundProject {
        t.Errorf("expected synthetic local-only snapshot for project; got %+v", snaps)
    }
}

func TestAtomMatching_BuildObserve_FiresForLocalStage(t *testing.T) {
    // Build envelope for local-stage meta; run atom Synthesize;
    // assert develop-build-observe.md atom is in the rendered set.
}

func TestAtomMatching_FirstDeployExecute_FiresForLocalStage(t *testing.T) {
    // Same shape — assert develop-first-deploy-execute.md fires.
}
```

### GREEN

Implement projection switch + synthetic snapshot. Existing tests
should still pass (single-projection cases for container modes
unchanged).

### Container regression

Critical regression coverage. Add explicit per-mode tests:

```go
func TestBuildServiceSnapshots_ContainerStandard_NoSynthetic(t *testing.T) {
    metas := []*ServiceMeta{{Hostname: "appdev", StageHostname: "appstage", Mode: topology.PlanModeStandard}}
    services := []platform.ServiceStack{{Name: "appdev"}, {Name: "appstage"}}
    snaps := buildServiceSnapshots(services, metas, nil, "")
    // Expect 2 snapshots only (live appdev + live appstage), no
    // synthetic — local-mode-only logic must not stamp synthetics
    // for container metas.
    if len(snaps) != 2 { t.Errorf("expected 2 snapshots, got %d: %+v", len(snaps), snaps) }
}

func TestAtomMatching_ContainerStandardStage_StillFiresForStageAtoms(t *testing.T) {
    // Verify any atom with modes: [..., stage, ...] still fires for
    // a container ModeStandard pair's stage half (snap.Mode = ModeStage).
}
```

### Live verify

`flow-eval-local`. Inspect the workflow_status response: assert
multiple snapshots include `mode: "local-stage"` for the linked
stage hostname AND a synthetic project entry. Verify previously-
broken atoms (`develop-first-deploy-execute`, `develop-ready-to-deploy`)
appear in rendered guidance.

### Commit message hint

```
fix(envelope): project local-stage as ModeLocalStage + synthesize local-only

Six atoms with modes: [..., local-stage] or [..., local-only] silently
never matched because resolveEnvelopeMode mapped local-stage's stage
hostname through DeployRoleStage → ModeStage, not ModeLocalStage. And
no live service maps to a local-only project meta's hostname (project
name ≠ service name), so no snapshot ever projected as ModeLocalOnly.

Same shape as the Phase 1.5 PruneServiceMetas bug: the local-mode
invariant ("project-keyed metas, project name in Hostname field") was
documented in spec-local-dev.md §6 but a sibling code path didn't
implement it. Phase 1.5 fixed PruneServiceMetas; Phase 11 fixes
envelope projection.

Two changes:

  1. resolveEnvelopeMode treats DeployRoleStage of a local-stage meta
     as ModeLocalStage (was ModeStage).
  2. buildServiceSnapshots appends a synthetic snapshot for each local
     meta keyed on the project name, so atoms with [local-only] /
     [local-stage] axis filters can match.

Container regression verified: zero atoms have modes: [stage] standalone
or any combination including stage but not local-stage. The projection
swap doesn't strand any existing container axis filter. ModeStandard
pairs project unchanged (dev half ModeDev, stage half ModeStage).

Affected atoms (now correctly fire for local-mode projects):
  - develop-build-observe.md
  - develop-first-deploy-execute.md
  - develop-ready-to-deploy.md
  - develop-dynamic-runtime-start-local.md
  - develop-close-mode-git-push.md
  - develop-close-mode-git-push-needs-setup.md

Pinned by TestResolveEnvelopeMode_LocalStage_*, container
regression by TestResolveEnvelopeMode_ContainerStandard_* and
TestBuildServiceSnapshots_ContainerStandard_NoSynthetic.
```

---

## Phase 12 — Env-aware adopt recovery in requireAdoption + git-push gate

### Why

`internal/tools/guard.go::requireAdoption` blocks deploys when the
target hostname is not in any meta's `Hostnames()`. For local-only
projects, the project meta's only hostname is the project name; live
runtime services (e.g. `app`, `zcp`) are NOT in `Hostnames()` because
they're not yet linked. Recovery message says:

> Adopt it first: zerops_workflow action="start" workflow="bootstrap"
> (with isExisting=true for app)

Wrong recovery for local mode — the right action is `adopt-local`, not
`bootstrap`. This compounds with adoption-note discoverability
(Phase 10): the recovery hint is the FIRST place a confused agent
looks.

Same shape in `internal/tools/deploy_git_push.go::handleGitPush` /
`PushSourceCheckFor`: for local-stage targeting the stage hostname,
the gate classifies as `PushSourceIsStageHalf` and tells the agent to
push from the dev hostname — but in local mode, the "dev hostname" is
the project name (the local working directory).

### What

1. **`requireAdoption` env-aware recovery** — pass `runtime.Info` (or
   environment) into `requireAdoption` and tailor the recovery message:

```go
func requireAdoption(stateDir string, recipeProbe RecipeSessionProbe, rt runtime.Info, hostnames ...string) *mcp.CallToolResult {
    // ... existing checks unchanged ...
    for _, h := range hostnames {
        if h == "" { continue }
        if workflow.IsKnownService(stateDir, h) { continue }
        if recipeProbe != nil && recipeProbe.CoversHost(h) { continue }
        
        if rt.InContainer {
            return convertError(platform.NewPlatformError(
                platform.ErrServiceNotFound,
                fmt.Sprintf("Service %q is not adopted by ZCP — deploy blocked", h),
                fmt.Sprintf("Adopt it first: zerops_workflow action=\"start\" workflow=\"bootstrap\" (with isExisting=true for %s)", h),
            ))
        }
        // Local env: prefer adopt-local for transitioning local-only → local-stage.
        return convertError(platform.NewPlatformError(
            platform.ErrServiceNotFound,
            fmt.Sprintf("Service %q is not linked as the local stage — deploy blocked", h),
            fmt.Sprintf(
                "Link it via:\n"+
                    "  zerops_workflow action=\"adopt-local\" targetService=%q\n"+
                    "Or for a fresh adoption flow:\n"+
                    "  zerops_workflow action=\"start\" workflow=\"bootstrap\" (with isExisting=true for %s)",
                h, h,
            ),
        ), WithRecovery(&topology.Recovery{
            Tool:   "zerops_workflow",
            Action: "adopt-local",
            Args:   map[string]string{"targetService": h},
        }))
    }
    return nil
}
```

Update all call sites of `requireAdoption` in `internal/tools/` to
pass `rt`.

2. **Shared git-push gate carve-out** for local-stage stage hostname:

The stage-half rejection lives in `gitPushMetaPreflight` (called by
BOTH `handleGitPush` for container AND `handleLocalGitPush` for
local), AND in `handleGitPushSetup` independently. Patching only one
site leaves the other broken. Fix at the shared classifier:

```go
// internal/workflow/service_meta.go::PushSourceCheckFor (or sibling)
//
// For local-stage meta, the linked stage hostname is a legitimate
// push target — the "dev half" is the local CWD (project name set
// by LocalAutoAdopt). The platform-side push from local CWD →
// linked stage is the canonical local-stage deploy path; rejecting
// it as "stage half — push from dev hostname instead" tells the
// agent to use the project name (which is NOT a deployable hostname
// from any deploy gate's perspective).
func (m *ServiceMeta) PushSourceCheckFor(hostname string) topology.PushSourceResult {
    if m == nil || hostname == "" {
        return topology.PushSourceUnknownHost
    }
    // NEW: local-stage + targeting the linked stage hostname =
    // legitimate push (treat as same-as-dev for push-source classification).
    if m.Mode == topology.PlanModeLocalStage && hostname == m.StageHostname {
        return topology.PushSourceIsValidSource  // existing constant
    }
    // ... existing checks (unchanged) ...
}
```

This single change covers all three callers automatically:
- `handleGitPush` (container) — its stage-half rejection logic now
  sees `PushSourceIsValidSource` for local-stage's stageHostname and
  proceeds. Container-standard pairs are unaffected (their meta.Mode
  is `PlanModeStandard`, not `PlanModeLocalStage`).
- `handleLocalGitPush` (local) — same classifier returns the same
  result; local push proceeds.
- `handleGitPushSetup` — same classifier; setup gate accepts the
  local-stage stageHostname target.

Read `PushSourceCheckFor` first to confirm constant names + check
that `PushSourceIsValidSource` is the right return shape (or pick
the appropriate non-rejection result).

### Phase 12 RED tests — all three callers

```go
func TestPushSourceCheckFor_LocalStage_StageHostname_AcceptedAsValidSource(t *testing.T) {
    meta := &ServiceMeta{Hostname: "myproject", StageHostname: "app", Mode: topology.PlanModeLocalStage}
    if got := meta.PushSourceCheckFor("app"); got != topology.PushSourceIsValidSource {
        t.Errorf("expected ValidSource, got %v", got)
    }
}

func TestPushSourceCheckFor_ContainerStandard_StageHostname_StillRejected(t *testing.T) {
    // CONTAINER REGRESSION
    meta := &ServiceMeta{Hostname: "appdev", StageHostname: "appstage", Mode: topology.PlanModeStandard}
    if got := meta.PushSourceCheckFor("appstage"); got != topology.PushSourceIsStageHalf {
        t.Errorf("expected IsStageHalf preserved for container-standard, got %v", got)
    }
}

func TestHandleLocalGitPush_LocalStage_StageHostnameTarget_Proceeds(t *testing.T) {
    // local-stage meta with StageHostname=app; agent calls
    // zerops_deploy strategy=git-push targetService=app via local handler.
    // Assert no PushSourceIsStageHalf rejection — push proceeds with
    // local CWD as source.
}

func TestHandleGitPush_LocalStage_StageHostnameTarget_Proceeds(t *testing.T) {
    // Container-side handler when an SSH-deploying ZCP somehow has a
    // local-stage meta in scope (rare; defensive — same classifier
    // means same behavior).
}

func TestHandleGitPushSetup_LocalStage_StageHostnameTarget_Proceeds(t *testing.T) {
    // adopt-local has linked stage; agent calls action=git-push-setup
    // service=app. Assert setup proceeds (existing rejection gone).
}
```

### RED tests

```go
func TestRequireAdoption_LocalEnv_RecoveryPointsAtAdoptLocal(t *testing.T) {
    // Mock metas: empty (or local-only with project name).
    // Call requireAdoption(stateDir, nil, runtime.Info{InContainer: false}, "app").
    // Assert error message mentions "adopt-local" and Recovery is
    //   {Tool: "zerops_workflow", Action: "adopt-local", Args: {"targetService": "app"}}.
}

func TestRequireAdoption_ContainerEnv_RecoveryStillPointsAtBootstrap(t *testing.T) {
    // Container regression: same call with InContainer=true points at
    // bootstrap (existing message preserved).
}

func TestHandleGitPush_LocalStage_StageHostnameTarget_ProceedsAsLocalCWDPush(t *testing.T) {
    // local-stage meta with StageHostname=app; agent calls
    // zerops_deploy strategy=git-push targetService=app.
    // Assert no PushSourceIsStageHalf rejection — push proceeds with
    // local CWD as source.
}
```

### GREEN

Plumb `runtime.Info` to `requireAdoption`. Implement env-aware error
+ Recovery hint. Update git-push gate for local-stage case.

### Container regression

Container path explicitly tested by the bootstrap-recovery test
above. Existing tests of `requireAdoption` in container scenarios
should still pass (bootstrap recovery message unchanged in container
env).

### Live verify

`flow-eval-local`. After auto-adopt (local-only state), agent calls
`zerops_deploy targetService=app`. Confirm:
1. Rejection message names `adopt-local` (not `bootstrap`) as the
   primary recovery.
2. Recovery hint args has `targetService=app`.

### Commit message hint

```
fix(deploy): env-aware adopt recovery in requireAdoption + git-push gate

requireAdoption blocked deploys to local-mode runtimes that haven't
been linked yet (local-only meta has only project name in
Hostnames()). The recovery message said "run bootstrap with
isExisting=true" — wrong action for local mode, where adopt-local is
the correct primitive.

Now passes runtime.Info into requireAdoption. Container env keeps
the bootstrap recovery message; local env points at adopt-local with
a structured Recovery hint args:{targetService}.

deploy_git_push gate had a parallel issue: for local-stage targeting
the stage hostname via strategy=git-push, PushSourceCheckFor
classified it as "push from stage half", suggesting "use the dev
hostname instead" — but in local mode the dev half is the project
name (local CWD). Now short-circuits the check for local-stage
+ stage-hostname-target and proceeds.

Container path unchanged: bootstrap-recovery wire shape preserved
for InContainer=true. Existing deploy_git_push tests for
container-standard pairs stay green.

Narrow fix only — full deploy-typed-intent split (Codex C3) deferred
to plans/deploy-typed-intent-202X.md.
```

---

## End-of-plan verification

After all eight phases land:

1. **Full test sweep**: `go test ./... -count=1 -race`.
2. **Lint clean**: `make lint-local`.
3. **Live local verify**: `make flow-eval-local ID=local-auto-adopt-node-postgres-first-deploy`. Read self-review; confirm zero remaining critical friction.
4. **Live container regression**: Pick one container scenario from
   `eval/behavioral/scenarios/` exercising deploy + import + env
   (e.g. `existing-simple-mode-add-endpoint`). Run via `flow-eval`
   per `eval/behavioral/README.md`. Confirm container behavior
   unchanged.
5. **Atom rendering**: spot-check that atoms previously broken for
   local-stage (Phase 11 list) now render in the workflow_status
   response for a local-stage envelope.
6. **Backlog cleanup**:
   - `git mv plans/local-flow-fundamentals-2026-05-06.md plans/archive/` once all eight phases ship.
   - Update `plans/backlog/recipe-knowledge-context-bleed-adopt-scenarios.md` if Phase 9-10 adoption note changes obviated it (likely not — recipe-content scope stays separate).

---

## Out of scope — separate plans

These are real but require their own design pass:

- **Full deploy-typed-intent split** (Codex C3 full proposal):
  refactor every deploy gate to consume a typed `LocalDeployIntent
  { metaKey, runtimeTarget, stageHostname, pushSource }` value.
  Phase 12 narrows the user-visible friction; the typed redesign
  waits for `plans/deploy-typed-intent-202X.md`.

- **`wouldDestroy.envVars` JSON path rename** (`loss.envVars` instead
  of nested `wouldDestroy.wouldDestroy.envVars`): rename
  `DiagnosedDestruction.Loss` JSON tag from `wouldDestroy` to `loss`,
  update every test asserting the old path, audit downstream readers.
  Cosmetic but breaks wire shape — separate plan.

- **Recipe-knowledge edits**: `npm ci` lockfile precondition,
  `connectionString` cross-service template guidance. Recipe-team
  scope; sync-push amplification. Tracked in
  `plans/backlog/recipe-knowledge-context-bleed-adopt-scenarios.md`.

- **Build-failure classifier coverage**: `npm ci` no-lockfile pattern,
  empty-logs case where build container exits before any logs land.
  Separate plan, needs live failure-pattern collection.

---

## Hand-off checklist

When starting work in a fresh session:

- [ ] Read this plan top-to-bottom.
- [ ] Read `CLAUDE.md`, `CLAUDE.local.md`, `MEMORY.md` (auto-loaded
      by Claude Code; if not, the relevant context is the codebase
      itself + the Reference: types section above).
- [ ] Eval retrospectives are NOT in git (gitignored under
      `eval/behavioral/runs-local/`). They're available locally if
      this plan is being executed from the same workspace where the
      audit happened. If they're missing, the relevant friction is
      summarized inline in each phase's "Why" section — work from
      that.
- [ ] Confirm preconditions section above.
- [ ] Begin Phase 5. Each phase: RED → GREEN → tests pass → lint
      clean → race clean → live verify → commit.
- [ ] No Phase X+1 until Phase X is fully verified live.
- [ ] If a phase reveals dependency on a deferred phase, STOP +
      surface to user.

---

## Risks + open questions for the implementer

1. **Phase 7 wire path test assertions** — `wouldDestroy.envVars` is
   actually nested at `wouldDestroy.wouldDestroy.envVars` because
   `DiagnosedDestruction.Loss` has JSON tag `wouldDestroy` (not
   `loss`). When writing the test, assert the actual wire path. The
   nested name is ugly; clean rename is filed as separate plan
   (`plans/diagnosed-destruction-wire-rename.md` placeholder). NOT
   this plan's scope.

2. **Phase 9 default `auto` choice** — adopt-local upgrading to
   local-stage with default close-mode `auto` is the most common path,
   but it suppresses the `develop-strategy-review` atom (which fires
   only when CloseDeployMode is unset/manual). If user feedback shows
   they wanted to be ASKED about close-mode rather than have it
   defaulted, change the default to unset and keep the strategy
   review in the loop. Trade-off documented; default chosen for
   minimum friction in the eval-evidence path.

3. **Phase 11 synthetic snapshot interaction with PruneServiceMetas**
   — Phase 1.5 already kept local metas through prune. Phase 11
   builds snapshots from kept metas. Confirm no double-write or
   double-snapshot issue when both fire.

4. **Phase 11 atom axes beyond Mode** — Atom matching also evaluates
   `runtimes`, `serviceStatus`, `deployStates`, `buildIntegrations`.
   Synthetic local-only snapshot leaves `RuntimeClass` empty,
   `Status` empty, etc. Atoms with `runtimes:` filter intentionally
   don't match the synthetic — it has no linked runtime to classify.
   The atom-coverage table in Phase 11 documents which of the 6
   originally-broken atoms gain firing for which local mode. If
   future atoms add `runtimes:` filter to local-only-targeting
   shapes, they need to be tested explicitly — the synthetic doesn't
   help them.

5. **Phase 5 classifier nil-topology semantics** — when the
   classifier is built with empty `liveHostnames` (e.g. shim mode of
   `zcp check env-refs` with no live API access), every ref classifies
   as "lone ref". This is strictly more permissive than the current
   first-underscore-split behavior, which would have surfaced
   malformed cross-service refs as host-not-found errors. **Loosens
   the shim's catch surface** — accepted for this plan; live-topology
   mode (the primary path) gains correctness. Re-evaluate if real
   bugs slip through shim-mode validation.

6. **Phase 12 must accompany Phase 11** — Phase 11 exposes atoms
   that emit `zerops_deploy strategy="git-push"` commands referencing
   the linked stage hostname. Without Phase 12's shared-gate
   carve-out, those agent-facing commands hit the
   `PushSourceIsStageHalf` rejection. Sequence integrity matters:
   never ship Phase 11 alone.

7. **Live-eval batching trade-off** — running flow-eval-local after
   every commit is too expensive (~5min wall × 8 phases). The
   batched approach (5-6, 7-8, 9-10, 11-12) accepts that a regression
   surfaces only at batch boundaries. Bisect within the batch's two
   phases when needed. Container-side regression is verified by
   unit/integration tests; the live container scenario is end-of-plan
   only.

8. **MEMORY.md reference** — first-pass plan referenced `MEMORY.md`
   (auto-loaded by Claude Code) in the hand-off section. The actual
   `MEMORY.md` may not exist in some workspaces. The hand-off
   wording is now optional — relevant context is summarized inline
   in each phase's "Why" section. If `MEMORY.md` is present, read it;
   if not, work from the plan body alone.
