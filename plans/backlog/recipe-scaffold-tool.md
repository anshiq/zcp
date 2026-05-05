**Surfaced**: 2026-05-04 — flow-eval suite `20260504-104436` retrospectives. Three classic-route scenarios (`classic-go-simple`, `classic-bun-simple`, `classic-rust-postgres-standard`) explicitly chose `route="classic"` because their matching recipe over-provisioned a managed dependency the user excluded; agent then scaffolded `zerops.yaml` and app code from scratch despite a curated working version existing in the recipe's repo.

**Why deferred**: the immediate atom-edit plan (`atom-edits-bootstrap-verify-2026-05-04.md`) closes the routing-clarity gaps that surfaced in the same suite; recipe-as-scaffold is a structural addition with its own multi-file design surface, not a content edit. Today the agent has two clean paths (full recipe import; classic route + recipe markdown for inspiration) and the inspiration leak is real but bounded — markdown carries the shape, agent can re-render to a file. The third path (decoupled scaffold) is genuinely better but worth a dedicated plan once we have evidence it shifts agent behaviour.

**Trigger to promote**: any of —
1. A fourth flow-eval scenario surfaces `[recipe-content]`-class friction with retrospective text along the lines of "wrote zerops.yaml from scratch when a recipe was sitting right there" or "had to translate the markdown to a file by hand".
2. User authoring a new recipe finds that the inspiration-via-markdown path produces visibly worse first-deploy success than recipe-import would have, indicating the markdown-as-spec abstraction is failing.
3. We start needing recipes to be parameterizable (e.g. `recipe X without Y dep`) and the import-then-trim variant has to be revisited and re-rejected — at that point a scaffold tool becomes the structurally correct answer to flag publicly.

## Sketch

New action on `zerops_recipe`:

```
zerops_recipe action="scaffold" slug="<recipe-slug>" target="<workdir-path>"
```

Behaviour:

1. Look up recipe metadata; pull `repoUrl` from frontmatter.
2. Shallow-clone the repo (`git clone --depth 1 <repoUrl> <target>`).
3. Remove `.git` so the scaffolded tree is unrelated to the recipe author's history.
4. Parse the cloned `zerops.yaml`; produce a summary block in the response: services declared, env-var references (`${X_*}` patterns), runtime managed-dep dependencies that the YAML hard-codes.
5. Return: list of files copied + the dep summary + a one-line guidance ("strip db references in zerops.yaml lines N-M and remove migrate.py from deployFiles if you don't need a database").

The agent then either:
- Goes `route="classic"` bootstrap with a customised local `zerops.yaml`, deploys via direct push, or
- Goes `route="adopt"` if the project is being grown from an existing infra.

The recipe import path stays unchanged; scaffold is additive.

## Risks

- **Tool description sprawl**: `zerops_recipe` may already have several actions; adding scaffold without a clean home risks bloat. Audit existing recipe tool surface before adding.
- **Empty dir vs in-progress workdir**: scaffold into a workdir that already has files is a destructive operation. Tool must require `target` to be empty or a fresh subdir; refuse otherwise.
- **Repo-side drift**: the canonical recipe app code lives at the upstream repo. If the upstream changes shape (new file, renamed entry point), scaffolded users on old MCP get a surprise. Pin the commit by tag or short SHA in recipe metadata, not just `main`.
- **Branch / mono-repo recipes**: some recipes might live in subdirs of a shared repo. Tool needs to support `repoUrl + path` not just `repoUrl`.
- **License footprint**: the recipe author's repo carries its own LICENSE. Scaffolding deletes `.git` but leaves the LICENSE; user gets a project that legally inherits the recipe author's licence terms by default. Document, don't hide.

## What this is NOT

- A replacement for full recipe import. Both modes coexist.
- A way to "trim" a fully imported recipe (the structurally broken Option B from the originating discussion). Scaffold is content-only; no infra is imported until the agent runs classic-route bootstrap on the scaffolded tree.
- A fork of the recipe repo. No git remote is preserved; the user is on their own from clone-time forward.

## Refs

- Originating discussion in flow-eval suite `20260504-104436` self-reviews:
  `eval/behavioral/runs/20260504-104436/{classic-go-simple,classic-bun-simple,classic-rust-postgres-standard}/self-review.md`.
- Adjacent atoms that would gain a "consider scaffold" cross-reference once the tool ships: `bootstrap-route-options.md`, `bootstrap-recipe-match.md`, `bootstrap-classic-plan-dynamic.md`.
- `internal/tools/recipe.go` and `internal/recipe/` are Aleš's scope — coordinate before touching, per `CLAUDE.local.md` "Recipe generation = Aleš's scope".
