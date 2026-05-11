# Cross-surface audit checklist

Each section names a defect class, the surfaces involved, the check
to run, and the suggested action. Walk them in order. Findings go in
the JSON list per `phase_entry.md`.

The seven content surfaces (anchored to
`docs/spec-content-surfaces.md`):

- S1 — Root README (`root/intro` fragment)
- S2 — Tier README (`env/<N>/intro` fragments)
- S3 — Tier `import.yaml` comments (typed `plan.EnvComments[<N>]`
  store; fragment IDs `env/<N>/import-comments/project` +
  `env/<N>/import-comments/<host>` are accepted by record-fragment but
  routed to the typed store, NOT to `plan.fragments`)
- S4 — Per-codebase Integration Guide
  (`codebase/<host>/integration-guide/<N>` fragments; IG #1 is
  ENGINE-EMITTED from the codebase's zerops.yaml — cite the underlying
  yaml fragment via `codebase/<host>/zerops-yaml` (S7) when flagging
  IG #1 content)
- S5 — Per-codebase Knowledge Base (`codebase/<host>/knowledge-base`)
- S6 — Per-codebase CLAUDE.md (`codebase/<host>/claude-md`)
- S7 — Per-codebase `zerops.yaml` comments
  (`codebase/<host>/zerops-yaml`) — the WHOLE yaml is one fragment;
  IG #1 on S4 is engine-rendered FROM this fragment, not authored
  separately

**fragmentId routing**: for codebase-bound surfaces (S4-S7) the
fragment IDs above are canonical keys in `plan.fragments`. For tier
surfaces S2 and S3, the engine uses a typed store
(`plan.EnvComments[<tier-index>]`); cite the conventional fragment
ID in findings (`env/<N>/import-comments/<host>` etc.), but be aware
the main agent's fix path may use `record-fragment` or a typed-store
write depending on surface. Cite the SHIPPED file path
(`environments/<tier-folder>/import.yaml`) alongside the fragment ID
so the main agent can navigate either way.

---

## Per-surface caps + floors (line-budget contract)

These are the hard caps from `docs/spec-content-surfaces.md` §"Per-
surface line-budget table". Cross-surface audit cares about FLOORS
(under-population) more than caps; the structural cap validators in
refinement-1 already gate over-cap.

| Surface | Bullet floor | Bullet cap | Defect-class on miss |
|---|---|---|---|
| S1 Root README | n/a | 35 lines | refinement-1 catches over-cap |
| S2 Tier README extract | 1 sentence | 2 sentences ≤ 350 chars | refinement-1 catches |
| S3 Tier yaml | 3 lines/svc | 8 lines/svc | refinement-1 catches |
| **S4 IG items / codebase** | **4** | **5** (incl. engine-emitted IG #1) | `kb-below-floor` if < 4 (use `surface: "S4"`); `kb-over-cap` if > 5 |
| **S5 KB bullets / codebase** | **5** | **8** | `kb-below-floor` if < 5 (use `surface: "S5"`); `kb-over-cap` if > 8 |
| S6 CLAUDE.md | ~30 lines | 50 lines (soft) | refinement-1 catches |

---

## Per-surface tests (one-question editorial test)

- **S1**: *"Can a reader decide in 30 seconds whether this deploys what they need?"*
- **S2**: *"Does this 1–2 sentence card description tell a porter which tier to click?"*
- **S3**: *"Does each service block explain a decision (why this scale / mode / presence), not narrate what the field does?"*
- **S4 (IG)**: *"Would a porter bringing their own code need to copy THIS exact content into their own app?"*
- **S5 (KB)**: *"Would a developer who read the Zerops docs AND the framework docs STILL be surprised by this?"*
- **S6**: *"Is this useful for operating THIS repo — not for deploying or porting?"*
- **S7**: *"Does each comment explain a trade-off the reader couldn't infer from the field name?"*

Items that fail their surface's test go in findings with
`suggestedAction: "drop"` or `"move-to-<surface-id>"`.

---

## Defect class: kb-ig-duplication

**What**: A KB bullet in a codebase teaches the SAME trap + SAME fix
as an IG item in the SAME codebase, with no additional symptom
dimension. Both surfaces become noise.

**Check**: For each codebase {api, app, worker, …}:

1. Read `codebase/<host>/integration-guide/*` fragments.
2. Read `codebase/<host>/knowledge-base` fragment.
3. For each KB bullet ## H3 heading, scan the IG items for one whose
   body addresses the same trap. Indicators of duplication:
   - Same error string quoted (`"Authorization Violation"`, `"Blocked request. This host is not allowed."`).
   - Same code fix shown (`servers + user + pass`, `allowedHosts: true`, `dist/~`).
   - Same env var or yaml field as the central artifact.
4. If both surfaces teach the FIX, flag as duplication.

**Pass condition** for a KB bullet that overlaps with an IG item:
the KB bullet must add the SYMPTOM dimension (porter-observable
failure mode beyond the fix) that the IG didn't cover. KB-first
phrasing (`### <Symptom> — <one-line cause>`) is the signal.

**Action**: `rewrite-as-symptom` (preserve KB, rewrite to lead with
symptom + back-reference IG for the fix) OR `drop` (when IG already
covers the symptom too).

**Severity**: advisory unless > 2 duplications in the same codebase
KB, then blocker (the KB has become an IG echo).

---

## Defect class: kb-below-floor / kb-over-cap (+ S4 IG counts)

**Check S5 KB**: For each codebase, count `### H3` headings inside
the `codebase/<host>/knowledge-base` fragment.

- < 5 bullets → `kb-below-floor` (advisory).
- > 8 bullets → `kb-over-cap` (blocker — refinement-1 should have
  caught this; double-check).

**Check S4 IG**: For each codebase, count IG items (IG #1 is
engine-emitted from the codebase's `zerops-yaml` fragment; items
#2+ are `codebase/<host>/integration-guide/<N>` fragments).

- < 4 items → `kb-below-floor` (advisory) with `surface: "S4"` and
  `fragmentId: "codebase/<host>/integration-guide"`.
- > 5 items → `kb-over-cap` (blocker).

**suggestedAction enum** for both classes: `"drop"` is the only
applicable enum value when no concrete fix exists at this boundary
(below-floor needs new content, not removal; over-cap needs
selection). Emit findings with `suggestedAction: "drop"` and
explain in `rationale` that the main agent must decide whether to
add bullets from the facts log (for below-floor) or rank-and-cut
(for over-cap). The `suggestedAction` field is required by the
JSON schema in `phase_entry.md`; "drop" is the conservative
fallback when no specific action applies.

---

## Defect class: surface-misplacement

**What**: An item on one surface should live on a different surface
per the seven-surface contract. Distinct from `scaffold-code-in-kb`
(below) — that's a specific sub-case where recipe-internal scaffold
prose lands in KB. Surface-misplacement is the broader class:

- A KB bullet that teaches generic framework setup (Vite `mount()`,
  Nest CLI, `php artisan serve`) — framework setup, not platform
  trap. Move to S6 (CLAUDE.md) or drop.
- An IG item that explains a recipe-internal convention the porter
  inherits already-done (e.g., a `## Use this api.ts wrapper`) —
  not Zerops-forced. Move to code comments or drop.
- A CLAUDE.md `## Zerops <topic>` section — Zerops content belongs
  on IG/KB/yaml-comments, not CLAUDE.md. Move to S4/S5/S7.

**Check**: For each item across S4-S7, apply the surface's
one-question editorial test from §"Per-surface tests" above. If the
item fails its surface's test BUT would pass on a different surface,
flag with `suggestedAction: "move-to-<correct-surface-id>"`. If it
fails everywhere, flag with `suggestedAction: "drop"`.

**Severity**: blocker (surface placement defines reader contract).

## Defect class: scaffold-code-in-kb

**What**: A KB bullet cites a recipe-authored source file
(`src/lib/bus.js`, `src/components/X.svelte`,
`src/server/api.ts`) AND describes that file's behavior in present
tense. This is scaffold-code, not a platform trap. Sub-case of
`surface-misplacement` with a concrete signature.

**Check**: For each KB bullet, scan the body for any of these
shapes naming a recipe-internal source file:

- Markdown link form: `[src/<path>]`, `[<filename>](src/<path>)`,
  `[\`src/<path>\`](src/<path>)`.
- Backtick prose form: `` `src/<path>` ``, `` `<recipe-file>.svelte` ``,
  `` `<recipe-file>.ts` ``.
- Bare path mention in prose: `"the recipe wires a small refresh
  bus in src/lib/bus.js"`, `"each card registers a refresh function
  ... see src/components/StatusStrip.svelte"`.

If the body further describes "what this codebase does"
(recipe-internal pattern: poll intervals, refresh-bus shape, event-
bus design) — flag. The KB surface is for platform traps the
porter would hit on ANY codebase, not for documenting the recipe's
own scaffold-time decisions.

**Action**: `move-to-S6` (CLAUDE.md `## Adding a feature panel`-style
section) OR `drop`.

**Severity**: blocker (defines surface placement).

---

## Defect class: aspirational-as-current

**What**: Porter prose asserts the recipe wires up X in present
tense (`"the SPA build receives only `${search_defaultSearchKey}`"`,
`"the api signs JWTs with `${APP_SECRET}`"`), but the actual zerops.yaml
+ source doesn't implement X.

**Check across all prose surfaces (S2 + S3 + S4 + S5 + S6 + S7)**:

KB/IG/CLAUDE.md prose claims (S4 + S5 + S6 + S7) — for each bullet
that names a specific named constant or env var:

1. Identify the named constant (`${search_defaultSearchKey}`,
   `MEILI_SEARCH_KEY`, `APP_SECRET` for JWT signing, etc.).
2. Read the relevant codebase's `zerops.yaml run.envVariables` (and
   `build.envVariables` for SPA codebases).
3. If the constant isn't declared, flag — the prose is aspirational
   but framed as current state.

Tier yaml prose (S3) — the tier `import.yaml`'s `project` block
preamble + per-service comments. Run-40 N-2 worked example: tier-0
import.yaml line 4 says "APP_SECRET is generated once at import and
shared across api + worker so JWT verification holds across
containers." For each tier-yaml prose mention of a framework feature
or named-constant claim:

1. Identify the feature (JWT verification, session sharing, magic-
   link auth, queue-group splitting, etc.) OR the named constant.
2. Cross-check: for feature claims, scan the relevant codebases'
   `package.json` / `composer.json` for the implementing dependency
   (`@nestjs/jwt`, `jsonwebtoken`, `passport-jwt`, etc.). For
   named-constant claims, check the codebases' yaml + source.
3. If absent, flag — tier-yaml prose ships in every deployed
   instance and porters trust it as the canonical "what this tier
   does".

**Framework-feature manifest scan** — read each codebase's manifest:
`<host>dev/package.json` (Node) OR `<host>dev/composer.json` (PHP)
OR `<host>dev/pyproject.toml` / `requirements.txt` (Python). The
brief's stitched-output pointer block enumerates per-codebase
README + zerops.yaml + CLAUDE.md; for `aspirational-as-current` you
also read the same codebase directory's manifest file.

**Action**: `reword-conditional` (rewrite as "if you expose X to the
SPA, here's the trap" rather than "the SPA receives X") OR `drop`.

**Severity**: blocker — recipe lies to porter.

---

## Defect class: yaml-comment-content-drift

**What**: A yaml comment names a Zerops cross-service alias
(`${<host>_<key>}`) that doesn't exist in the same yaml's
envVariables block AND isn't a known cross-service alias.

**Check**: For each tier `import.yaml` AND each codebase
`zerops.yaml`:

1. Scan COMMENTS for `${<host>_<key>}` tokens.
2. For each token, identify the host's **service type** from
   `plan.services[].type` in plan.json (e.g. `db` → `postgresql@18`,
   `cache` → `valkey@7.2`, `broker` → `nats@2.12`, `storage` →
   `object-storage`, `search` → `meilisearch@1.20`). Cross-service
   aliases are SERVICE-TYPE-SPECIFIC — `password` is valid for
   postgres/valkey but NOT for meilisearch (meilisearch publishes
   `masterKey` + `defaultSearchKey`, not `password`).
3. Check whether the SAME yaml file declares an env that resolves
   to the token, OR whether the token is a documented alias VALID
   FOR THE HOST'S SERVICE TYPE per the table below.
4. If the token doesn't appear in envVariables AND isn't a valid
   alias for the host's service type, flag.

**Per-service-type alias allowlist** (only these are documented
Zerops aliases for the named service type):

| Service type | Valid `${<host>_<key>}` suffixes |
|---|---|
| postgresql@* / valkey@* | hostname, port, user, password, dbName (postgres only), connectionString |
| nats@* | hostname, port, portManagement, user, password, connectionString |
| object-storage | apiUrl, apiHost, bucketName, accessKeyId, secretAccessKey |
| meilisearch@* | hostname, port, masterKey, defaultSearchKey, connectionString |
| static (build-from-git frontend slot) | zeropsSubdomain, zeropsSubdomainHost |
| any runtime service | zeropsSubdomain, zeropsSubdomainHost |

Hosts WITHOUT a matching entry in `plan.services[]` are runtime
codebases (api/app/worker); their cross-service tokens reference
peer services and follow the peer-service-type's allowlist.

**Worked example (run-40 N-1)**: tier-0 import.yaml comment says
`"shared with both services via ${search_password}"`. Host `search`
has service type `meilisearch@1.20`. Meilisearch allowlist publishes
`masterKey` + `defaultSearchKey` — NOT `password`. The yaml content's
envVariables uses `${search_masterKey}`. Flag with
`suggestedAction: "fix-named-constant"`, `suggestedReplacement:
"${search_masterKey}"`.

**Action**: `fix-named-constant` with `suggestedReplacement` set to
the correct alias if obvious from sibling yaml content.

**Severity**: blocker.

---

## Defect class: cross-codebase-named-constant-drift

**What**: A named constant (queue group string, env var name, port
number) appears with different values in different surfaces.

**Check**: Read the `## Canonical-latest constants` block in this
brief (engine-rendered from `plan.namedConstants` + canonical-topic
facts). For each constant:

1. Scan every fragment for the constant name.
2. If any surface uses a value different from the canonical, flag.

**Worked example (run-39 closed in run-40)**: `'workers'` in source
vs `'showcase-workers'` in tier yamls. Canonical: `worker-indexer`.
Find any place still using a non-canonical value.

**Action**: `fix-named-constant`.

**Severity**: blocker.

---

## Defect class: ig-cites-recipe-internal-file

**What**: An IG item cites a recipe-authored file path
(`src/lib/api.js`, `src/components/StatusStrip.svelte`) as if the
porter has it. The IG is for porters bringing their own code —
referencing recipe-internal files is dead weight.

**Check**: For each IG item body:

1. Scan for `[src/<path>]` or `` `src/<path>` `` references to files
   that exist only in the recipe's scaffold.
2. Flag any. Exception: when the IG item is teaching a PATTERN and
   uses the recipe's file as a worked example, the body must
   explicitly frame it as "the recipe's example" — without that
   framing, flag.

**Action**: `rewrite-as-symptom` (rephrase to general pattern) or
`drop`.

**Severity**: advisory.

---

## Defect class: missing-citation

**What**: A KB bullet covers a topic that has a dedicated
`zerops_knowledge` guide, but the bullet doesn't cite the guide by
name.

**Check**: For each KB bullet, scan body for topic keywords. The
authoritative list of {topic → required-citation} pairs is the
`## Citation map — topics requiring zerops_knowledge citation`
section rendered into this brief by the engine composer (below the
audit checklist). Walk THAT map, not a hardcoded list — the brief's
citation map is engine-versioned and will evolve.

For each KB bullet:

1. Identify the topic family the bullet covers (rolling-deploys,
   init-commands, object-storage, env-var-model, subdomain-access,
   managed-nats, managed-meilisearch, etc.).
2. Match against the brief's citation map. If the bullet's topic
   has a required citation, scan the bullet body for the cited
   guide name OR the cited docs URL.
3. If neither the guide name nor the URL appears, flag.

**Worked tolerance**: a bullet may legitimately reference multiple
guide topics. The citation is required ONCE per bullet, not once
per topic mention. If bullet body cites the guide for any of its
topics, the bullet passes.

**Action**: `add-citation`.

**Severity**: advisory.

---

## Findings emission

Walk the seven defect classes in order. For each hit, emit ONE
finding. Empty findings list = pass.

After the walk, emit the single fenced JSON block as defined in
`phase_entry.md`. No prose around it.
