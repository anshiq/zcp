# Plan: Local-flow recipes + env sync — Recipes-Option-1 + two-region `.env`

> **Status**: Proposed.
> **Date**: 2026-05-07
> **Predecessor**: `plans/archive/local-flow-fundamentals-2026-05-06.md`
> (Phases 5-12 shipped; this plan picks up two architectural concerns
> the retrospective + post-ship review surfaced).
> **Scope**: Two related local-flow architecture additions.
> **Scope OUT**: Recipe engine internals (`internal/recipe/`,
> `workflow_recipe.go`) — Aleš's scope; coordinate, do not edit.

## Why this plan exists

After the local-flow-fundamentals wave (Phase 5-12) shipped, the user
surfaced two architectural concerns that the bug-fix wave didn't reach:

### Concern 1 — Recipes in local-flow

Recipe import YAMLs (`internal/knowledge/recipes/<slug>.import.yml`)
provision both `<name>dev` (Zerops dev runtime, `zeropsSetup: dev`) and
`<name>stage` (Zerops stage runtime, `zeropsSetup: prod`). Both come
with `buildFromGit` so Zerops auto-builds initial code from upstream at
provision.

In container-mode the agent SSHs into `appdev`, edits the source, and
cross-deploys to `appstage`. **In local-mode the user's CWD replaces
`appdev`** — the agent runs `npm run dev` directly on the user's
machine; no SSH-in dev runtime exists.

The atom contract `bootstrap-discover-local.md` already says
"no `{name}dev` service on Zerops in local mode", but the recipe-route
import path consumes the YAML verbatim. So today, a local-mode recipe
route either provisions `appdev` anyway (defeating local-mode purpose)
or fails the bootstrap check (`appdev_exists=fail`).

**User decision (2026-05-07)**: keep stage exactly as in container-mode
(with `buildFromGit` + `zeropsSetup: prod`). Just drop the `appdev`
service. The user clones the recipe repo locally for editing; Zerops
stage gets initial code from upstream `buildFromGit`; subsequent local
edits redeploy to stage via `zerops_deploy targetService=appstage
workingDir=<cwd>`.

This is **Option 1** from the Codex `/tmp/codex-recipes-local-flow.md`
analysis (minimum-risk, stage stays container-shaped).

### Concern 2 — Env update mechanism for local

In local-mode the user's app reads a generated `.env` file. ZCP's
`generate-dotenv` resolves `zerops.yaml run.envVariables` + project env
+ cross-service `${host_var}` refs and writes the file. Phase 6 added
the platform-internals denylist.

But there's no contract for **what does the LLM do when it wants to
update an env var**. Three channels exist (yaml, project env, service
env), each with different timing and propagation. And there's no
mechanism for **user-static values that ZCP must not clobber on regen**.

**User clarification (2026-05-07)** is more nuanced than my initial
two-region scheme captured:

> "Třeba envu APP_ENV chceme mít v `.env` nastavenou na 'local', ale
> třeba v `zerops.yaml` nebo project env na 'production'. Na druhou
> stranu třeba envy pro db budeme chtít umět synchronizovat."

Translation: APP_ENV should be 'local' in `.env` but 'production' in
zerops.yaml/project env. DB envs should auto-sync from Zerops. So the
override semantics are **per-key**: some keys are user-locked, some are
ZCP-managed-always.

The basic two-region mechanism (one ZCP-managed block + one user
add-only block) doesn't cover this — it handles ADDING new user vars
but not OVERRIDING ZCP-managed values selectively. Per-key opt-in/opt-out
needs design.

**User instruction**: ship the two-region basic now, defer the per-key
override design to the next session.

## Foundation: what we verified before writing this plan

1. **Container stage-shape today** — `nodejs-hello-world.import.yml`
   confirms both appdev and appstage carry `buildFromGit` + `zeropsSetup`.
   So the simplest local transform is "drop services with
   `zeropsSetup: dev`" — stage stays untouched.

2. **No env-aware recipe filtering exists** — Codex traced the
   recipe-route end-to-end. `RewriteRecipeImportYAML`
   (`internal/workflow/recipe_override.go`) only does hostname mapping
   and EXISTS managed-service drops. The function is the right
   insertion point for env-aware logic.

3. **bootstrap-provision-local.md is `routes: [classic]`** — pinned by
   `synthesize_test.go::TestSynthesize_LocalProvisionAtomIsClassicOnly`
   as intentional. So no local-specific atom fires for recipe-route
   provision today; that's the gap.

4. **`workflow_checks.go::checkServiceRunning` validates DevHostname
   unconditionally** — even in local-mode it expects appdev to exist.
   Loosening this is required when local-mode-recipe lands.

5. **`generate-dotenv` indexes by serviceHostname matching `setup:`
   name** — recipes use `setup: dev`/`prod`, not hostnames. Atom
   guidance is the lighter fix; explicit `setup` parameter is heavier
   but more explicit.

6. **Recipe markdown frontmatter has `repo:`** but `RecipeMatch`
   (`internal/workflow/route.go`) doesn't surface it. Need to plumb
   through for atom-driven `git clone`.

## How an LLM implementer should approach this plan

1. Read top-to-bottom before starting.
2. Order: Theme 2 first (smaller, more discrete), then Theme 1.
3. TDD per ZCP convention: RED → GREEN → tests + lint + race → commit.
4. Container regression non-negotiable: every phase has explicit
   container-mode test coverage OR provably local-only paths.
5. Aleš coordination triggers (any of):
   - Recipe corpus shape changes (parallel `<slug>.local.import.yml`).
   - Recipe synthesizer integration with bootstrap-consumable variants.
   - Recipe knowledge markdown edits beyond minor cross-references.
6. **Per-key env override semantics is DEFERRED** — see Theme 2
   "Deferred design question". Two-region basic ships first.

---

## Theme 1 — Recipe-route in local-flow (Option 1)

### Architecture

```
Local-mode recipe-route bootstrap, end-to-end:

  1. ZCP detects EnvLocal + route=recipe → uses local-aware path
  2. ZCP transforms import.yml programmatically:
     • Drop services with zeropsSetup: dev (appdev)
     • Keep stage runtime AS IS (zeropsSetup: prod, buildFromGit, all)
     • Keep all managed services
  3. ZCP submits transformed YAML to zerops_import
     → Zerops creates {appstage, db}, builds appstage from buildFromGit
        (initial code from upstream)
  4. Atom guides agent: empty-CWD check → git clone <recipe-repo> .
     Repo URL comes from RecipeMatch.Repo (added in phase 1C)
  5. Atom guides agent: zerops_env action="generate-dotenv"
        serviceHostname=<recipe-setup-name>  (e.g. "prod")
     → .env lands in CWD
  6. Atom guides agent: develop locally (npm run dev etc.) +
     redeploy to stage via zerops_deploy targetService=appstage
        workingDir=<cwd>
     → first local-driven build replaces upstream-seeded version
```

**Key invariants:**

- Stage container-shape **unchanged** — buildFromGit retained, zeropsSetup
  retained, no `startWithoutCode` set. Stage immediately works after
  provision (initial code from upstream).
- Local CWD has independent git checkout — user iterates without round-
  tripping through upstream.
- zerops.yaml in CWD: as-is from the cloned repo (multi-setup syntax
  preserved for future monorepo support per user decision). No yaml
  mutation by ZCP. Atom MAY suggest dropping `setup: dev` as cosmetic
  cleanup but doesn't enforce.

### Phase 1A — Local recipe import transform

**Why**: Today's recipe import.yml goes to `zerops_import` verbatim.
Local-mode needs `appdev` stripped before submission.

**What**:

```go
// internal/workflow/recipe_import_local.go (new)

// LocalizeRecipeImportYAML drops services that exist solely for the
// container-mode SSH-in dev workspace. In local mode the user's CWD
// replaces these; provisioning them on Zerops would defeat local-mode
// purpose.
//
// Container-mode shape (preserved): every service stays.
// Local-mode shape: services with zeropsSetup: dev are dropped;
// stage runtime + managed services pass through unchanged.
func LocalizeRecipeImportYAML(yamlContent string) (string, error)
```

**Wire-in**:
- `internal/workflow/bootstrap_guide_assembly.go::formatRecipeImportYAMLForGuide`:
  when `EnvLocal`, route through `LocalizeRecipeImportYAML` after
  `RewriteRecipeImportYAML`.
- `internal/workflow/engine.go::BootstrapCompletePlan`: same.

**Tests**:
- `TestLocalizeRecipeImportYAML_DropsZeropsSetupDev`.
- `TestLocalizeRecipeImportYAML_PreservesStageAndManaged`.
- `TestLocalizeRecipeImportYAML_NoOpForRecipesWithoutDev` (e.g.
  `nextjs-ssr-hello-world` — no dev block to drop).
- `TestLocalizeRecipeImportYAML_PreservesYAMLNodeOrdering` (YAML
  comments + ordering matter for the agent's downstream review).

**Container regression**:
- `TestRewriteRecipeImportYAML` stays green (non-local paths preserve
  `zeropsSetup` + `buildFromGit`).
- `TestBuildGuide_Recipe_ProvisionRewritesYAMLWithPlanHostnames` stays
  green.

**Size**: ~80 LOC + tests.

### Phase 1B — Loosen workflow_checks + bootstrap_outputs

**Why**: `checkServiceRunning` expects `DevHostname` to exist; after
1A it doesn't in local mode. `bootstrap_outputs` writes the plan's
mode verbatim; local-mode-recipe should write `Mode=local-stage`.

**What**:
- `internal/tools/workflow_checks.go::checkServiceRunning`: when
  `EnvLocal` + recipe route, skip DevHostname existence check; only
  validate stage runtime + managed deps.
- `internal/workflow/bootstrap_outputs.go`: when `EnvLocal` + the
  plan has a stage runtime, write `Mode=PlanModeLocalStage` with
  `StageHostname=<created-runtime>` (mirroring `LocalAutoAdopt` case 1
  semantics from Phase 9).

**Tests**:
- `TestCheckServiceRunning_LocalRecipe_SkipsDevHostname`.
- `TestCheckServiceRunning_ContainerRecipe_StillRequiresDev`
  (regression).
- `TestBootstrapOutputs_LocalRecipe_WritesLocalStageMode`.
- `TestBootstrapOutputs_ContainerStandard_StillWritesStandardMode`
  (regression).

**Size**: ~40 LOC + tests.

### Phase 1C — Plumb RecipeMatch.Repo

**Why**: Recipe markdown frontmatter has `repo:` but
`internal/workflow/route.go::RecipeMatch` drops it. Atom needs the URL
to interpolate into a `git clone` command.

**What**:
- `internal/workflow/route.go`: add `Repo string` field to `RecipeMatch`.
- `internal/workflow/recipe_corpus_store.go`: read frontmatter `repo:`
  during corpus load, populate the field on match construction.
- Bonus: also include `repo` in the `bootstrap-recipe-match.md` atom's
  template-vars surface so atom can render it.

**Tests**:
- `TestRecipeCorpusStore_LoadsRepoFromFrontmatter`.
- `TestBuildBootstrapRouteOptions_RecipeRouteCarriesRepoURL`.

**Size**: ~30 LOC + tests.

### Phase 1D — Atoms (clone + local-import + match modification)

**Why**: The local-mode-recipe path needs explicit atom guidance.
Without it the agent guesses (Codex confirmed this is exactly what
happens today).

**What** (three atoms, one modification):

1. **NEW** `internal/content/atoms/bootstrap-recipe-local-clone.md`:
   - Filters: `routes: [recipe]`, `environments: [local]`,
     `steps: [discover]`.
   - Shape:
     - "Local CWD replaces the recipe's appdev runtime."
     - "Before provisioning, verify CWD is empty
       (`ls -A` returns empty, or only contains ZCP state)."
     - "If non-empty: stop and ask. Do NOT clone over user files."
     - "Clone into CWD: `git clone <recipe.repo> .` (where
       <recipe.repo> is rendered from RecipeMatch.Repo)."
     - "Upstream remote stays connected — to use your own remote, run
       `git remote set-url origin <your-repo>`."

2. **NEW** `internal/content/atoms/bootstrap-recipe-import-local.md`:
   - Filters: `routes: [recipe]`, `environments: [local]`,
     `steps: [provision]`.
   - Shape:
     - "Submit the localized YAML (ZCP already dropped the dev runtime)
       via `zerops_import`. Stage will provision with code from upstream
       buildFromGit; you don't deploy it yourself this time."
     - "After services reach RUNNING:
       (a) `zerops_env action=\"get\" project=true` to surface project env keys.
       (b) `zerops_env action=\"generate-dotenv\" serviceHostname=\"prod\"` —
            uses the cloned zerops.yaml's `setup: prod` block.
       (c) Add `.env` to `.gitignore`.
       (d) Guide user to run `zcli vpn up <projectId>` for managed-service
            access from local."
     - "First app run: `npm install && npm run dev` (or framework
       equivalent). Subsequent stage deploys:
       `zerops_deploy targetService=<stage-hostname> workingDir=<cwd>`."

3. **MODIFY** `internal/content/atoms/bootstrap-recipe-match.md`:
   - Today says "Do not write code — `buildFromGit` pulls the app repo
     at import." This is container-only.
   - Add qualifier: "(container only; in local mode you'll clone the
     recipe repo locally — see `bootstrap-recipe-local-clone`)."

**Tests**:
- `TestSynthesize_LocalRecipeProvisionAtomFires`.
- `TestSynthesize_ContainerRecipeProvisionUnchanged` (regression).
- `TestSynthesize_LocalProvisionAtomIsClassicOnly` stays green
  (the existing local-classic atom is unaffected; the new one is
  recipe-route specific).

**Size**: ~150 LOC (mostly atom content + axis tests).

### Phase 1E — Live verification

**Why**: Codex flagged `Option 1` as needing live verification because
the recipe → local transform path is new.

**What**:
- New scenario `eval/behavioral/scenarios-local/recipe-nodejs-hello-world.md`:
  - Pre-seed: empty Zerops project (no services), empty CWD.
  - Prompt: "Use the nodejs-hello-world recipe to set up a Node + Postgres
    project."
  - Expected: agent picks recipe route, clones repo, .env generated,
    `npm install` works, stage deploys via redeploy from local.
  - Tags: `[local-mode, recipe-route, first-deploy, node, postgres]`.
- Run via `make flow-eval-local ID=recipe-nodejs-hello-world`.

**Tests**: scenario fixture + retrospective surfacing.

**Size**: ~100 LOC scenario + supporting fixtures.

### Phase 1 risk register

- **Stage waits for buildFromGit pull** — Zerops takes 1-2 minutes to
  clone + build initial code. atom-import-local.md should set agent
  expectations: provision-then-RUNNING is slower than classic-route's
  empty-shell provisioning.
- **Repo authentication** — recipe repos are public; private app repos
  added later need user-side `git` credential setup. ZCP doesn't own
  Git credentials.
- **`.git` retained after clone** — user's first commit will surprise
  upstream remote unless they `git remote set-url`. Atom warns.
- **Setup-name vs hostname mismatch in generate-dotenv** — recipe yamls
  use `setup: prod`/`dev`; atom guides `serviceHostname="prod"` for
  generate-dotenv. Cosmetic concern; explicit `setup` parameter on env
  tool is a future ergonomics win, deferred.

---

## Theme 2 — Two-region `.env` (with deferred per-key override)

### Architecture (basic — ships first)

`.env` has TWO regions delimited by comment markers:

```bash
# Generated by ZCP. Below the ZCP-MANAGED block is YOUR space.
# Values you put after the END marker survive every `generate-dotenv`.

# === ZCP-MANAGED BEGIN ===
# Auto-synced from zerops.yaml + project env + service refs.
# Edits inside this block are clobbered on every regen.
DATABASE_URL=postgresql://db:abc@zcpdb:5432/main
DB_HOST=zcpdb
APP_KEY=base64:xyz
# === ZCP-MANAGED END ===

# Your overrides — preserved across regenerations.
LOG_LEVEL=debug
FEATURE_FLAG_X=true
```

**Logic**:
- On `generate-dotenv` invocation:
  1. Read existing `.env` if present.
  2. Find `=== ZCP-MANAGED BEGIN ===` and `=== ZCP-MANAGED END ===`
     marker lines.
  3. Three branches:
     - **Both markers present**: extract content after end marker as
       user-block; replace managed block with fresh resolution.
     - **Neither marker** (legacy `.env` from Phase 5/6 era): treat
       entire content as user-block; prepend fresh managed block.
       Surface migration warning in result.
     - **One marker missing or malformed**: refuse + ask for cleanup.
  4. Compute new managed-block hash + user-block hash for manifest.
  5. Atomic write `.env.tmp` → rename.
  6. Update manifest at `.zcp/state/dotenv/<setup>.json`.

### Phase 2A — Spec + atom

**What**:
- Update `docs/spec-local-dev.md §7` with the two-region scheme.
- New atom `internal/content/atoms/develop-local-env-sync.md` with the
  surface→canonical table from Codex output, plus the two-region
  explanation and atom guidance "after editing zerops.yaml or
  project env, run `generate-dotenv` to refresh local `.env`".
- Modify `internal/content/atoms/develop-env-var-channels.md` with a
  local addendum.

### Phase 2B — Manifest + atomic write

**What**:
- New `internal/ops/env_dotenv_manifest.go` with manifest read/write.
- Manifest schema:
  ```json
  {
    "setup": "prod",
    "yamlSourceHash": "sha256:...",
    "projectEnvFingerprint": "sha256:...",
    "serviceEnvFingerprints": {"db": "sha256:..."},
    "managedBlockHash": "sha256:...",
    "userBlockHash": "sha256:...",
    "generated": "2026-05-07T10:00:00Z"
  }
  ```
- `env_generate.go`: switch to atomic write (`.env.tmp` → rename).

### Phase 2C — Two-region marker logic

**What**:
- `env_generate.go`: parse existing `.env` for markers, preserve user
  block, write managed block + user block in correct order.
- `EnvDotenvResult` extends with `ManifestPath`, `Migrated bool`,
  `Freshness string` (`"fresh"|"managed-block-edited"|"yaml-changed"`).

**Tests**:
- `TestEnvGenerateDotenv_NewFile_WritesBothMarkers`.
- `TestEnvGenerateDotenv_LegacyFile_WrapsExistingAsUser_Migrated`.
- `TestEnvGenerateDotenv_PreservesUserBlock`.
- `TestEnvGenerateDotenv_ClobbersManagedEditWithWarning`.
- `TestEnvGenerateDotenv_OneMarkerMissing_ReturnsError`.

### Phase 2D — Status check

**What**:
- New `internal/tools/workflow_checks_local_env.go::checkLocalDotenvFresh`.
- Wired into `zerops_workflow action="status"` lifecycle.
- Recovery hint: `tool=zerops_env`, `action=generate-dotenv`, args
  carry `serviceHostname`.
- Detect cases:
  - yaml mtime > .env mtime → stale → recover.
  - manifest absent → never generated → recover (Status="missing").
  - managed-block-hash on disk differs from manifest → drift inside
    managed (user edited there) → recover with warning.

### Phase 2E — Edge cases

- **VPN down during regen** — service-env fingerprints unavailable;
  `.env` still lands with Phase 5 vpnHint; manifest records empty
  service-fingerprints; status check tolerates this branch.
- **Multi-target local** (multiple runtime hostnames on Zerops, single
  CWD) — today `generate-dotenv` writes one `.env` per setup-block in
  the CWD's zerops.yaml. Multi-target use case is rare; document
  constraint, defer multi-output enhancement.
- **Race condition** — atomic rename plus manifest-after-write reduces
  risk; advisory lock under `.zcp/state/dotenv.lock` is overkill for
  single-user local dev.

### DEFERRED — Per-key override semantics (next session)

The basic two-region scheme handles "user adds vars ZCP doesn't know
about". It does NOT handle the user's clarification:

> "Třeba envu APP_ENV chceme mít v `.env` nastavenou na 'local', ale
> třeba v `zerops.yaml` nebo project env na 'production'. Na druhou
> stranu třeba envy pro db budeme chtít umět synchronizovat."

In this scenario:
- `APP_ENV` exists in canonical surface (yaml/project) with value
  `production`. User wants `.env` to show `local` instead.
- `DB_HOST` exists in canonical surface as cross-service ref. User
  wants `.env` to always show the live Zerops value.

The basic two-region writes APP_ENV into managed (=production), and
the user can't override it without entering shadow territory (most
dotenv loaders are first-wins, so user-block at bottom doesn't win).

**Open mechanism options for next session:**

1. **Per-key annotation in zerops.yaml** — `run.envVariables` entry
   carries a marker (yaml comment, custom key syntax) saying "local
   override allowed; emit-only-if-not-overridden".
   ```yaml
   run:
     envVariables:
       APP_ENV: production # zcp:local-override
       DATABASE_URL: ${db_connectionString}  # always managed
   ```
   At regen: ZCP reads the existing `.env` user-block; if a key is in
   user-block AND has `zcp:local-override` annotation in yaml, skip
   emitting it in managed-block (user value wins). DB_HOST without
   annotation: always emit fresh in managed.

2. **Per-key annotation in `.env` user-block** — user pins certain
   keys explicitly:
   ```bash
   # === ZCP-MANAGED END ===
   # zcp:lock APP_ENV
   APP_ENV=local
   ```
   At regen: ZCP scans user-block for `# zcp:lock <KEY>` directives;
   removes those keys from managed-block emission.

3. **Three-region scheme** — managed block + locked-by-user block
   (suppresses managed emission for these keys) + pure-user-add block.
   ```bash
   # === ZCP-MANAGED BEGIN ===
   DATABASE_URL=...
   # === ZCP-MANAGED END ===

   # === USER-LOCKED BEGIN === (these suppress same-key managed emission)
   APP_ENV=local
   # === USER-LOCKED END ===

   LOG_LEVEL=debug
   ```

4. **Layered files** — `.env.zerops` (always synced, ZCP-managed) +
   `.env.local` (user-only, ZCP never touches) + framework-side
   precedence chain. Requires framework support; not universal.

**Decision criteria for next session:**
- Discoverability: which mechanism is most obvious to LLM agents on
  first encounter?
- Robustness: which survives partial yaml/manifest corruption best?
- Aleš coordination: option 1 touches yaml syntax (recipe-author
  scope?) — confirm with Aleš before going there.
- Test surface: which option has the smallest test matrix?

**Trigger to promote**: any of —
- A flow-eval-local retrospective surfaces "I wanted `.env` value X but
  ZCP overwrote it with Y on next regen".
- User explicitly schedules the next-session improvement.
- A second concrete use case beyond APP_ENV / DB_HOST emerges.

**This deferral is intentional** per user instruction 2026-05-07:
"udělám vylepšení v další session". Theme 2 ships with the basic
two-region; per-key override design pass happens later.

---

## Out of scope — Aleš coordination items

- Recipe corpus shape changes (e.g. parallel `<slug>.local.import.yml`)
  — recipe-author scope. If `LocalizeRecipeImportYAML` programmatic
  transform proves brittle, escalate.
- Recipe markdown content rewrites beyond the `bootstrap-recipe-match`
  cross-reference. Recipe knowledge files are gitignored (Strapi-synced);
  `zcp sync push` amplification risk per CLAUDE.local.md.
- Recipe synthesizer integration with bootstrap-consumable variants.
  `internal/recipe/testdata/fixtures/synth-showcase/2 — Local.yaml`
  shows the desired shape but isn't on the bootstrap-recipe-route path.
  Long-term unification with `internal/knowledge/recipes/` is Aleš's
  call.
- Per-key env override mechanism (see Theme 2 deferred section). If
  option 1 (yaml annotation) is chosen, it's recipe-author-adjacent;
  options 2/3/4 are ZCP-only.

---

## Hand-off checklist

When starting work in a fresh session:
- Read this plan top-to-bottom.
- Confirm preconditions (`git status` clean, full test sweep + lint
  clean, race clean).
- **Order**: Theme 2 phases 2A-2E first (smaller, more discrete), then
  Theme 1 phases 1A-1E.
- TDD per CLAUDE.md: every phase is RED → GREEN → tests + lint + race
  → commit. Pure refactors skip RED.
- Container regression: every phase has explicit container-mode test
  coverage or provably-local-only paths.
- Live verify after Theme 1 Phase 1E
  (`make flow-eval-local ID=recipe-nodejs-hello-world`).
- Live verify after Theme 2 Phase 2C
  (`go test ./e2e/ -tags e2e -run TestE2E_EnvGenerateDotenv`
  to confirm two-region scheme works against real Zerops).
- Single commit per phase.
- **DO NOT** start the per-key override design pass — that's deferred
  to a separate session per user instruction.

## Risks for implementer

1. **Phase 1A YAML parsing fragility** — recipe YAMLs carry comments,
   ordering, conditionally-placed fields. Use `yaml.Node` round-trip
   (preserves structure) not `yaml.Unmarshal/Marshal` (drops comments,
   reorders). Mirror the approach in
   `internal/workflow/recipe_override.go::RewriteRecipeImportYAML`.

2. **Phase 1E live verification flakiness** — `flow-eval-local` against
   eval-zcp with real `git clone` of upstream-recipe repo introduces
   network dependency. Timeout the clone with reasonable bounds; on
   network failure surface a clear retry message.

3. **Phase 2A atom drift** — `develop-env-var-channels.md` is referenced
   from many other atoms via `references-atoms`. Local addendum must
   not break the existing reference graph. Run
   `tools/lint/atom_template_vars` after edits.

4. **Phase 2C marker collision** — if the user happens to have lines
   matching `=== ZCP-MANAGED BEGIN ===` literally in their existing
   `.env` (unlikely but possible), parse goes wrong. Use a robust
   marker pattern that won't collide with real env-file content.

5. **Theme 2 deferred design pressure** — basic two-region ships
   without solving APP_ENV-style override. Eval scenarios that test
   this case will surface friction; resist the urge to fix it inside
   this plan. Friction → backlog → next-session design pass.

## Estimated size

- Theme 1: ~400 LOC + ~6 atom files + 1 e2e scenario.
- Theme 2 basic: ~250 LOC + 3 atom files + spec doc edits.
- Total: ~650 LOC across ~20 files, 10 phases.

For comparison: Phase 5-12 wave shipped ~1100 LOC across 22 files in
8 phases. This plan is similar in size but split across two themes.
