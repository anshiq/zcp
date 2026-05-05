# Per-tier authoring workflow

You author env-level surfaces (root + per-tier intros + import-comments)
across 6 tiers (0..5). The brief carries:

- Per-tier capability matrix (already computed)
- Cross-tier deltas from `tiers.go::Diff`
- Engine-emitted `tier_decision` facts (one per cross-tier whole-tier
  delta + one per per-service mode change)
- The plan snapshot (codebases + services)
- Parent-recipe pointer (when present)

## Workflow

1. **Author root/intro** — one sentence that frames what the recipe IS
   (stack + scenario), not what Zerops does with it. ≤ 500 chars, no
   markdown headings.
2. **For each tier 0..5**:
   - Author `env/<N>/intro` (1-2 sentences naming who this tier is for
     + the cost/availability tradeoff). ≤ 350 chars, no `## ` headings,
     no `<!-- #ZEROPS_EXTRACT_*` tokens (engine stamps those at stitch).
   - For each service block in the tier import.yaml that has a causal
     story, author `env/<N>/import-comments/<host>` (≤ 8 lines per block).
   - Author `env/<N>/import-comments/project` for the project-level
     block (secrets, corePackage).
3. **Cross-reference parent** when present — read parent's per-tier
   intros and dedup before authoring.

## Voice

Friendly authority, declarative, present-tense, porter-facing. State
the platform mechanism, name the tradeoff, end on the porter signal
that triggers a change. Never "we chose X", never "during scaffold".

PASS — production-tier APP_KEY block:

> *APP_KEY — production encryption key shared across all app and
> worker containers. Critical for session validity when the L7
> balancer distributes requests across multiple containers.*

Mechanism (encryption key shared) → consequence (session validity
under L7 balancing). Two sentences, no field narration.

PASS — tier-0 PostgreSQL block:

> *PostgreSQL — single instance with the smallest managed RAM.
> Snapshots run, but there is no replica — restoring means downtime,
> which is acceptable because tier-0 data is disposable. Priority 10
> ensures it accepts connections before any runtime initCommand fires
> migrations.*

Names the mode (single instance, NON_HA implied), the operational
consequence (manual-restore window), the audience (tier-0 data is
disposable), and the priority's effect (initCommand ordering).

PASS — tier-4 small-prod app block:

> *Small production — minContainers: 2 guarantees two app containers
> at all times, enabling rolling deploys with zero downtime (one
> container serves traffic while the other rebuilds). Zerops
> autoscales RAM within verticalAutoscaling bounds to absorb traffic
> spikes without manual intervention.*

Mechanism (two containers always running) → outcome (rolling deploys
without downtime) → operational reality (autoscaling absorbs spikes).

## Friendly-authority phrasing — the adapt-path contract

The refinement rubric grades these import-comments against Criterion 2
(Voice). Engineering-spec prose ("Single instance with the smallest
managed RAM. Snapshots run, but there is no replica…") describes the
shape correctly but doesn't tell the porter what to change. **Aim for
the 8.5 anchor at first write** — at least one friendly-authority
phrasing per service block, each tied to a concrete porter signal —
not the 7.0 engineering-spec floor that refinement then has to lift.
Voice rewrites across 50+ blocks at refinement time exceed the F-27
threshold; voice belongs in env-content authoring.

### Canonical phrasing patterns (count toward the score)

- *"Feel free to ..."*
- *"Configure this to ..."*
- *"Replace ... with ..."*
- *"Disabling ... is recommended ..."* / *"Enabling ... is recommended ..."*
- *"Adapt this ..."* / *"Adjust this ..."*
- *"Bump ... if ..."* / *"Switch ... when ..."*
- *"... once you ..."* (conditional adapt)

Each phrasing MUST name a concrete porter signal: a numeric threshold,
a configuration state, or a named external condition (custom domain
configured, monitoring shows X approaching ceiling, steady-state
traffic spikes saturate fan-out, dataset size crosses N GB). Without a
signal, it's a hedge — not friendly authority — and doesn't count.

**Forbidden hedge phrasings** — weasel words that name no signal:

- *"you might want to consider ..."*
- *"perhaps this could ..."*
- *"in some cases ..."*
- *"depending on your needs ..."* (without a named need)

The voice is *"this is the choice; here's the mechanism; you can
change it for your needs IF X"* — declarative + invitation, never
hedge.

### Worked example — runtime block (api/worker/app)

**BEFORE** — engineering-spec, 7.0 floor:

```yaml
# Two containers minimum on dedicated CPU — predictable latency
# under load, no shared-CPU jitter from neighbour services. The
# L7 balancer distributes requests across containers.
- hostname: api
  type: nodejs@22
  zeropsSetup: prod
  enableSubdomainAccess: true
  minContainers: 2
  verticalAutoscaling:
    minRam: 0.5
    minFreeRamGB: 0.25
```

The mechanism is named, but the porter doesn't know which knob to
turn when their own traffic shape diverges.

**AFTER** — friendly authority, 9.0 anchor:

```yaml
# Two containers minimum on dedicated CPU — predictable latency
# under load, no shared-CPU jitter from neighbour services. Bump
# minContainers to 3 if your steady-state traffic spikes saturate
# the two-replica fan-out; switch verticalAutoscaling.maxRam upward
# when monitoring shows containers approaching the current ceiling.
# Subdomain access is on by default — disable it once you have a
# custom domain configured.
- hostname: api
  type: nodejs@22
  zeropsSetup: prod
  enableSubdomainAccess: true
  minContainers: 2
  verticalAutoscaling:
    minRam: 0.5
    minFreeRamGB: 0.25
```

Three friendly-authority phrasings (*"Bump … if"*, *"switch … when"*,
*"… once you"*); each tied to a concrete porter signal (steady-state
saturation; monitoring shows ceiling approach; custom domain
configured). The mechanism still leads.

### Worked example — managed-service block (db/cache/broker/search)

**BEFORE** — engineering-spec, 7.0 floor:

```yaml
# Single instance with the smallest managed RAM. Snapshots run,
# but there is no replica — restoring means downtime, which is
# acceptable because tier-3 stage data is rehearsal-grade.
- hostname: db
  type: postgresql@18
  priority: 10
  mode: NON_HA
  verticalAutoscaling:
    minRam: 0.25
```

**AFTER** — friendly authority, 8.5 anchor:

```yaml
# Single instance with the smallest managed RAM. Snapshots run,
# but there is no replica — restoring means downtime, which is
# acceptable because tier-3 stage data is rehearsal-grade. Switch
# mode to HA once you need failover without a manual-restore
# window; bump verticalAutoscaling.minRam if working-set growth
# pushes query latency past your stage SLO.
- hostname: db
  type: postgresql@18
  priority: 10
  mode: NON_HA
  verticalAutoscaling:
    minRam: 0.25
```

Two friendly-authority phrasings (*"Switch … once you"*, *"bump …
if"*); each names the porter signal that triggers the adapt
(failover-without-downtime requirement; working-set-vs-SLO).

### Worked example — project-level block (secrets, URL constants)

**BEFORE** — engineering-spec, 7.0 floor:

```yaml
# APP_SECRET is generated once at import and shared across api +
# worker so JWT verification holds across the L7 balancer.
# STAGE_API_URL and STAGE_FRONTEND_URL are the single-slot stage
# URLs that frontends and CORS allow-lists consume.
project:
  name: <recipe-slug>-stage
  envVariables:
    APP_SECRET: <@generateRandomString(<32>)>
    STAGE_API_URL: https://api-stage.example.com
    STAGE_FRONTEND_URL: https://app-stage.example.com
```

**AFTER** — friendly authority, 8.5 anchor:

```yaml
# APP_SECRET is generated once at import and shared across api +
# worker so JWT verification holds across the L7 balancer. Rotate
# this via the Zerops UI's project envs once you suspect leakage
# — every container picks up the new value on next restart.
# Replace STAGE_API_URL and STAGE_FRONTEND_URL with your own
# stage hostnames once subdomain access is swapped for a custom
# domain.
project:
  name: <recipe-slug>-stage
  envVariables:
    APP_SECRET: <@generateRandomString(<32>)>
    STAGE_API_URL: https://api-stage.example.com
    STAGE_FRONTEND_URL: https://app-stage.example.com
```

Two friendly-authority phrasings (*"Rotate … once you"*, *"Replace
… once"*); each tied to a porter signal (suspected leakage;
custom-domain swap).

### Tier-specific carve-outs (HOLDs)

Some tier × service combinations don't carry a clean adapt path.
Be explicit rather than forcing a phrasing that strains:

- **Tier 0 / tier 1 (workspace tiers)** — the porter is iterating,
  not adapting. Friendly-authority phrasings strain because there's
  no production-shaped knob to turn yet. Use **orientation framing**
  ("This single instance with smallest managed RAM is enough for
  agent / human-porter iteration; production scale arrives at
  tier 4") rather than adapt-path framing. The voice score softens
  here by design.
- **`object-storage`** — no `mode` field, no replica option, no
  `verticalAutoscaling`. The only adapt path is `objectStorageSize`.
  **One adapt phrasing is fine** ("Bump objectStorageSize when
  uploads outgrow the current quota").
- **`meilisearch`** — no HA mode at any tier (platform contract).
  Adapt path is `verticalAutoscaling.minRam` if index size grows.
  Don't force a `mode`-flip phrasing; it doesn't exist.
- **Single-valued mode fields** — at tier 5 db, mode is `HA` and
  there is no further mode upgrade. **HOLD on the mode-flip
  phrasing** for that field; pivot to `verticalAutoscaling`
  adapt paths or backup retention adapt paths in the same block.

When all adapt paths in a block fall under a HOLD, the block can
land at the 7.0 voice floor without penalty — orientation framing
is the correct shape.

## One causal teaching per block, deduped across services + tiers

Each block teaches a non-obvious choice. Two different dedup rules
operate at two different scopes — confusing them strips per-tier
flavor and leaves the porter without operational framing.

### Cross-tier dedup is for the canonical-set teaching, NOT for per-tier flavor

The **canonical service set** (which services exist, what each one
is for, the `${db_*}`/`${queue_*}` env-injection contract) is
authored once at tier 0 and not re-explained at every subsequent
tier. That's the canonical-set dedup — it strips repetition.

The **per-tier flavor** (what the runtime / mode / minContainers
shape MEANS for THIS service at THIS tier — single instance vs
NON_HA-with-snapshots vs HA-with-failover; what the operational
reality is for the porter at this scale) is per-tier teaching.
**Keep at least 1-2 lines of flavor framing per service block at
every tier**, even when no field changes from the previous tier.

Concretely: at tier 1 (Remote CDE), the postgres block looks
field-identical to tier 0 (same NON_HA mode, same priority). But
the FLAVOR is different — tier 0 is the agent's workspace where
data is replaceable; tier 1 is the human-porter dev workspace
where data is also replaceable but the porter expectation (single
instance for CDE iteration, snapshot for safety net) is worth
naming. Keep that 1-2 line framing on the postgres block at tier 1.
Drop the canonical-set "Postgres stores the items table" prose
(already in tier 0).

Example tier-1 postgres block flavor (1-2 lines, kept):

```yaml
# Same NON_HA shape as tier 0 — single instance for the human-
# porter dev workspace; snapshots cover restoration if needed.
- hostname: db
  ...
```

Tier 0 ships full per-block teaching (canonical-set + flavor)
because there's no prior tier to inherit from. Tiers 1-5 inherit
the canonical-set silently AND ship per-tier flavor comments on
every block. A bump (mode flip, corePackage upgrade, minContainers
bump) gets fuller teaching (3-5 lines naming the change + its
operational consequence); identical-shape blocks still get the
1-2 line flavor framing.

The run-17 baseline (100-135 indented `#` lines per tier) was
canonical-set repetition — the same NON_HA-with-snapshots paragraph
on db, cache, broker, search at every tier. The run-22 baseline
(6 lines per tier at tiers 1-3) over-corrected — every service
lost its per-tier flavor. The target is ~15-25 lines per tier:
canonical-set stripped, per-tier flavor preserved.

### Cross-service dedup within a tier

If all four managed services share the same NON_HA-with-snapshots-
and-no-replica story at tier 0, the project-level and per-service
blocks each carry the part the porter needs at THAT scope — the
project block names project-wide invariants (secret scope,
corePackage), and each service block names ONLY what's specific
to that service (RAM target, priority, peer dependency).

## Anti-patterns

The run-17 baseline shipped 100-135 indented `#` lines per tier. Every
class of waste shows up in the diff against the reference (~22 lines
per tier). Avoid:

**Field narration** — restating the directive name in prose:

```
# minContainers: 2
# This sets the minimum number of containers to 2.
```

The directive value already says "2"; the comment says nothing the
yaml doesn't. Replace with mechanism + reason:

```
# Two containers always running enables rolling deploys —
# one serves traffic while the other rebuilds, no downtime.
```

**Repetition across services** — copy-pasting the same NON_HA-with-
snapshots paragraph onto db, cache, broker, and search blocks at tier
0. Write it once at the project level (or on the first service block),
and the per-service blocks carry only what's specific to that service.

**Repetition across tiers** — explaining `verticalAutoscaling` on every
single tier. Explain it once, where it first appears or where it
changes. Subsequent tiers' service blocks carry no comment when the
configuration is identical to the prior tier's.

**Tier-promotion narratives** — *"Promote to tier 1 once a human porter
takes over"*, *"Outgrow this when..."*, *"Upgrade from tier N to N+1
when..."*. The contrast between tier yamls is the promotion signal —
the porter scrolling through tiers sees what changes. Don't narrate
the promotion path in prose.

**Authoring voice** — *"we chose X over Y"*, *"during scaffold"*,
*"recipe author decision"*. These are sub-agent process language; the
porter doesn't operate as part of the authoring run.

**"See the X guide" trailing slugs** — *"See: env-var-model guide."*,
*"see `init-commands`"*. The agent's `zerops_knowledge` tool slugs
don't resolve as docs URLs for porters. Cite by inline prose mention
or omit the citation.

**Platform-internal claims that exceed the corpus** — *"already
redundant at the platform layer"*, *"the bucket lives on a replicated
backend"*, *"Zerops automatically synchronizes…"*. The corpus
(`themes/services.md` + `themes/core.md`) is the authoritative source
for what the platform does and does not promise. If a service has no
HA mode, name that fact as the porter sees it (single instance, no
replica option, recovery via X) — do NOT extrapolate into hidden
platform behaviour the corpus does not document.

**FAIL** (run-21 tier-5 storage block):

```yaml
# Object storage is already redundant at the platform layer — the
# bucket lives on a replicated backend regardless of mode, so no
# service-mode upgrade exists to flip.
```

The "already redundant at the platform layer" + "replicated backend"
claims are stronger than `services.md` supports — the corpus only
states "no HA mode" and "S3-compatible MinIO backend".

**PASS** (corpus-grounded shape):

```yaml
# Object storage stays on the same shape every tier — there is no
# `mode` field on this service type (NON_HA only) and no replica
# option to flip. Production scaling here means raising
# objectStorageSize; durability and availability are whatever the
# managed S3-compatible backend provides, not a recipe knob.
```

The mechanism (no mode field, no replica option) is named without
inventing platform-internal redundancy; the porter knob (objectStorageSize)
is the correct adapt path.

**Authoring-tool names in published comments** — `zerops_*`,
`zsc <subcommand>`, `zcli`, `zcp` — the porter operates with framework-
canonical commands, not the agent's tool inventory.

## Per-block density target

3-5 lines per block, every line carries mechanism or reason. Skip
"# Base image" / "# Run command" labels — those say nothing the yaml
doesn't. The 8-line per-block cap is the validator's hard limit;
3-5 is the quality target.

A block under the cap is not the goal. The goal is: every line teaches
a non-obvious choice. If you cannot say something non-obvious about a
block, skip the comment.

## Self-validate

`zerops_recipe` is an **MCP tool** — invoke it as a JSON tool call,
not a shell command. Invoke `zerops_recipe` with
`action: complete-phase` and `phase: env-content` to run EnvGates()
validators. Fix violations by re-invoking `zerops_recipe` with
`action: record-fragment` and `mode: replace` until the gate passes.
