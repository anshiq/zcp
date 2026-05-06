**Surfaced**: 2026-05-06, behavioral eval `local-auto-adopt-node-postgres-first-deploy` retrospective + follow-up alignment investigation. Agent triangulated correctly but flagged friction; second eval run with the same scenario shape did not surface the friction (agent went straight to `setup: <hostname>`). Symptom is real but mild.

**Why deferred**: The immediate symptom (agent uncertain whether to use `setup: prod`/`setup: dev` or `setup: <hostname>` in adopt-simple-mode) was patched in the same fix sweep by tightening the `zerops_deploy` `setup` parameter doc on `internal/tools/deploy_local.go::deployLocalInputSchema` and `internal/tools/deploy_ssh.go::deploySSHInputSchema` (commit at the head of this work). That removes the misleading "(bootstrap workflows only)" parenthetical and explicitly flags the dev/prod naming as recipe-flow convention. Agent already gets it right — the friction is "had to triangulate from two sources," not "got it wrong." Bigger architectural fix is not justified by current evidence.

**Sketch — the deeper issue**: When an agent in an adopt scenario calls `zerops_knowledge` for runtime patterns (e.g. "nodejs hello world"), the returned recipe-knowledge file (`internal/knowledge/recipes/<slug>.md`) is shaped for the FULL recipe-flow (dev + stage with cross-deploy). It contains `setup: prod` / `setup: dev` blocks without flagging that this naming is recipe-specific. An agent in an adopt-simple or local-stage context reading that file may copy the convention even though the convention there is `setup: <hostname>` (single setup, name-matches-host so the deploy `setup` param can be omitted).

The recipe-knowledge files are gitignored (synced via Strapi), so editing them locally requires `zcp sync push`. Per CLAUDE.local.md "sync push amplification" warning, that's outside the scope of a one-shot local fix.

**Trigger to promote**:
- Two more behavioral evals (any scenario) where the agent independently surfaces "I confused recipe-flow naming with adopt-simple naming" or makes the actual mistake (writes `setup: prod` for an adopt scenario, hits a deploy mismatch).
- Or a separate scope decision to invest in recipe-knowledge context-conditioning (e.g. `zerops_knowledge` returning different excerpts based on the live work-session phase / adopt vs greenfield).

**Possible directions** (when triggered):
1. **Per-recipe knowledge prelude** — add a one-paragraph "Naming convention" section to each recipe knowledge file that distinguishes recipe-flow (`dev`/`prod`/`worker`) from adopt-flow (`<hostname>`). Cross-cuts every recipe; high amplification cost, simple guidance.
2. **Adopt-flow atom for setup naming** — currently `develop-first-deploy-scaffold-yaml.md` says "setup matches the runtime hostname" once. Add an explicit anti-pattern atom: "Don't copy recipe-flow `setup: prod` / `setup: dev` into adopt-simple yamls — see scaffold-yaml atom for the right shape." Concrete cost; small footprint.
3. **`zerops_knowledge` context-aware filtering** — return excerpts conditioned on the active work-session mode. Larger arch change; would land alongside other context-aware knowledge work.

**Refs**:
- Self-review: `eval/behavioral/runs-local/20260506-133416/local-auto-adopt-node-postgres-first-deploy/self-review.md` (paragraph 5)
- Atom that already says it: `internal/content/atoms/develop-first-deploy-scaffold-yaml.md`
- Recipe knowledge that doesn't pre-empt: `internal/knowledge/recipes/nodejs-hello-world.md`
- Tool param doc tightened in commit: `internal/tools/deploy_local.go::deployLocalInputSchema`, `internal/tools/deploy_ssh.go::deploySSHInputSchema`
