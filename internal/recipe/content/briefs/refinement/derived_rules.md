# Derived rules — golden-grounded scoring substrate

Every rule cites observable evidence in `laravel-jetstream` + `laravel-showcase`. Walk every stitched fragment; for each, consult the rules under that fragment's surface and score as ACT or HOLD per `phase_entry/refinement.md`.

Principle-shaped, with observable anchors. Score rule-by-rule, not by pattern matching. Complements `embedded_rubric.md`; when they disagree, cited golden evidence wins.

---

## Universal voice rules (all surfaces)

- **V1 — porter-clones-and-runs framing.** Voice addresses someone who clones the apps repo and deploys, not someone learning the framework or the platform. "This app uses Y for Z" / "this showcases how to integrate X with Zerops". Never "we chose", "during scaffold", "the agent owns", "the recipe author".
- **V2 — names what the recipe IS, not what it could be.** Names the specific framework + feature set. Never "any Node HTTP framework", "if you use Symfony instead", "alternative starter kits". Jetstream README has zero alternatives across 291 lines.
- **V3 — link text references real concepts the porter recognizes**, not internal corpus slugs verbalized. Use `[Laravel documentation]`, `[zsc health-check]`, `[multi-container setups]`. Forbidden: `[managed NATS service]`, `[Zerops env-var model]`, `[Zerops object-storage service]`, `[Zerops rolling-deploys guide]` — these are corpus slug names rendered as English. Test: the link text must name the SUBJECT, not the lookup-key.
- **V4 — porter-actionable phrasings tied to platform signal.** "Feel free to change this", "Configure this to use real SMTP sinks", "Bump … once …". Imperatives + customization invitations.
- **V5 — defer broad platform concepts to docs when inline explanation would sprawl.** Conditional, not mandatory. Use `https://docs.zerops.io/...` references when sprawl is likely; inline when 1-2 sentences cover it.
- **V6 — no authoring vocabulary in any porter-facing surface.** Forbidden tokens: `zerops_dev_server`, `zsc noop`, "the agent", "scaffold", "feature phase", "recipe author", "record-fact". (The yaml `start: zsc noop --silent` directive is legitimate yaml content; surrounding comments must not adopt authoring voice.)

## Recipe-repo root README

The root README is a **6-tier entry point**, not a documentation surface.

- **R1 — short** (~28 lines). Single intro paragraph + deploy button + cover image + 6-tier link list + catalog punt + Discord. Drop exact line count; keep "short entry-point" principle.
- **R2 — intro paragraph names framework + what it demonstrates.** One sentence; optionally a "with all the essentials …" follow-on naming features.
- **R3 — 6 tier links uniform shape.** `**TIER NAME** [[info]](path) — [[deploy with one click]](url)`. Both `[info]` and `[deploy]` for every tier.
- **R4 — trailing punt to recipe catalog**, not in-line teaching: "For more advanced examples see all PHP recipes on Zerops."
- **R5 — trailing Discord invite.**
- **R6 — no IG, no KB, no architecture description, no managed-services list.** Root README has zero `## H2` content sections. Zero exceptions.

## Tier README

Tier READMEs are short delta descriptions.

- **T1 — short** (Stage README is 7 lines; AI Agent is 8). Title + one-sentence framing + 2-3 line intro extract. Drop exact line count; keep "short delta" principle.
- **T2 — intro extract names this tier's DELTA**, not its absolute spec. "Stage uses the same configuration as production, but runs on the lowest scaling settings."
- **T3 — title links back to recipe deploy page**: `[recipe-name (info + deploy)](recipe-url)`.
- **T4 — no tier-promotion narrative.** Tier README does NOT say "promote to tier 5 when X" or frame the current tier as a stepping-stone.

## Tier import.yaml

- **TY1 — top-of-file comment mirrors tier README intro extract.** Length varies (2-4 lines). Drop exact line count; keep "mirror the intro" principle.
- **TY2 — per-service comment is 1-2 sentences naming SERVICE ROLE in this recipe.**
- **TY3 — optional services explicitly marked optional.** "Feel free to remove this service, if you wish to stage-test your app with as-close-as-possible production setup."
- **TY4 — comments name framework-canonical effects.** "Used by the Laravel app to store data" — not "consumed by the api codebase".
- **TY5 — service ordering with `priority` justified in human terms when non-default.**

## Apps-repo Integration Guide (per codebase)

- **IG1 — IG #1 is engine-emitted "Add `zerops.yaml`" with verbatim yaml.** Agent does not author this slot.
- **IG2 — IG #2-#N are 1-4 line steps.** Jetstream's IG #2 is 4 lines (composer require + edit one file). #3 is 2 lines. #4 is one paragraph. **Each step does ONE thing.** Showcase items are 1-3 paragraphs. Candidate items running 19-70 lines violate this hard.
- **IG3 — IG body links to specific apps-repo GitHub lines** when relevant. Concrete, ungeneralized: `[league/flysystem-aws-s3-v3](https://github.com/.../composer.json#L14)`.
- **IG4 — no cross-framework adapt-paths.** Stays recipe-framework-specific. Zero "if you use Symfony" / "any PHP framework" / "Express's `app.set`" / "Fastify uses `trustProxy: true`" / "Webpack/Astro/SvelteKit/Next/Nuxt". Jetstream has zero across 291 lines.
- **IG5 — frame each IG step around a concrete change.** Concept explanation must serve that change. Step #2 = "Add Support For Object Storage" → `composer require league/flysystem-aws-s3-v3` + edit `config/jetstream.php#L79`. Concept explanation lives in the step's body insofar as it justifies the change.

## Apps-repo "Recipe features" + "Production vs Development"

- **RF1 — "Recipe features" is a bulleted list of ~8 bullets** framed by what the recipe SETS UP, not what porters can do.
- **PD1 — "Production vs. Development" is 3 bullets max.** Names HA migration paths the porter takes when scaling. Examples: "Use highly available version of the PostgreSQL database", "Use at least two containers", "Use production-ready third-party SMTP server instead of Mailpit". Not a tier-promotion narrative.

## Apps-repo Knowledge Base

- **KB1 — KB items use H3 heading + paragraph(s) + optional callout/fenced shell.** Jetstream `### Maintenance Mode` + paragraph + `> [!CAUTION]` + ```shell. Showcase mostly uses flat bullets with bolded stems. Both shapes are observed; flat-bullets-only with no H3 structure is below the bar (apidev). Sibling-divergence (one codebase uses H2, another uses `### Gotchas`, a third has no header) is below the bar.
- **KB3 — KB stem is symptom-first OR directive-tightly-mapped.** Showcase: "**No `.env` file**" / "**Cache commands in `initCommands`, not `buildCommands`**" / "**`APP_KEY` is project-level**" — directive-tightly-mapped where the body opens with the observable failure.
- **KB4 — `> [!CAUTION]` callouts are reserved for porter-destructive operations.** Jetstream uses CAUTION around `php artisan down` + health-check disable sequence.
- **KB5 — fenced shell blocks show the porter's command sequence**, not platform-internal commands.
- **KB6 — KB primarily addresses framework × platform intersection traps**; ad-hoc operational items are a secondary class. Showcase has 7 KB items, 6 of them framework-platform intersection traps (env shadowing, build-vs-runtime path mismatch, project-level APP_KEY, base-image extension absences, MinIO path-style, Vite manifest HMR-vs-build interaction). 1 ops item. That's the bar.

## Apps-repo zerops.yaml

- **Y1 — top-of-file block comment names the setup pattern.** ~5 lines. Names what each setup IS and what each does.
- **Y2 — comments are causal and porter-relevant.** Mechanism + effect + (often) so-what. Examples: "Stderr logging sends output to Zerops runtime log viewer — no log files to manage or rotate." / "Readiness check gates the traffic switch — new containers must answer HTTP 200 before the L7 balancer routes to them. This enables zero-downtime deploys." Field-name documentation alone is insufficient.
- **Y3 — comments explain framework requirements that motivate Zerops choices.** "PHP applications generally require a web server such as Nginx or Apache HTTP server."
- **Y4 — URL deferrals are conditional**, not mandatory. Use when inline explanation would sprawl; inline when 1-2 sentences suffice. Showcase has zero URL deferrals; jetstream has 4. Both are acceptable.
- **Y5 — comments name PORTER-customization opportunities.** "Feel free to change this value to your own custom domain, after setting up the domain access."
- **Y6 — init-command comments explain WHEN they run + WHY**, not what they are.
- **Y7 — comments use framework-canonical terms only.** Forbidden: authoring tokens (V6 list).
- **Y8 — no tier-promotion narrative inside yaml comments.**
- **Y9 — no decision-tree-of-alternatives in comments.** Comments name THIS choice, not "alternatives are X, Y, Z and we picked W because…".
- **Y10 — each setup section opens with a 1-2 line preamble** naming the setup's PURPOSE + what it optimizes for + (when applicable) what it deliberately omits. Showcase: `# Worker — background job processor consuming from Redis queue. Same codebase as the app, different entry point. No HTTP traffic — no healthCheck, readinessCheck, or documentRoot.`
- **Y11 — when a setup has fewer fields than its sibling, comments name the deliberate absences.** "No HTTP traffic — no healthCheck, readinessCheck, or documentRoot."
- **Y12 — skip `extends:`; repeat envVariables inline per setup.** When inline repetition spans many lines, lead with a one-line note: showcase shape `# Same service wiring as prod — only mode flags differ.` Recipe yaml is read top-to-bottom by porter; flat yaml beats DRY-with-indirection.

## Gap-fillers — yaml fields neither golden comments well

- **TIER-1 — per-tier `mode: HA` / `mode: NON_HA`** comment names what HA buys vs what NON_HA misses (cf. showcase tier `5 — Highly-available Production/import.yaml:47-53,:59-65,:79-84`).
- **PREPROC-1 — `<@generateRandomString(<32>)>` preprocessor** comment names per-end-user generation timing (cf. jetstream tier-0 import.yaml:24-28).

## Scoring loop

For each stitched fragment:

1. **Hard-rule pass FIRST.** Walk every rule prefixed V (universal voice), every rule prefixed Y7 (no authoring vocabulary), and IG4 (no cross-framework adapt-paths). These are HARD RULES — a single hit ACTs regardless of how the fragment scores on Voice/Trade-off/etc. criteria. Run-32 phase 2 lesson: a fragment scoring 8.5+ on the Voice criterion (rich friendly-authority phrasings) can STILL contain `zerops_dev_server` and require an ACT — the criterion score is independent of hard-rule presence.
2. **Criterion pass.** Then walk rules under the fragment's surface section.
3. For each candidate-violation, check threshold: cite the violated rule + the exact fragment text + the preserving edit. All three present: ACT. Any fuzzy: HOLD with a note.

Bias toward ACT — snapshot/restore reverts a wrong ACT automatically.

## Out of scope here

Cross-codebase coherence (env-var naming across siblings), factuality (URL correctness), stitching artifacts — refinement should HOLD with a class-named notice. Not refinable from this rule list.
