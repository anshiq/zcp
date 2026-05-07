---
id: bootstrap-recipe-import-local
priority: 2
phases: [bootstrap-active]
routes: [recipe]
environments: [local]
steps: [provision]
title: "Recipe local — provision + first-deploy is the bootstrap completion"
coverageExempt: "recipe+local+provision — covered by recipe-local-flow design (Theme 1) + flow-eval-local scenarios"
---

### Provision shape

ZCP transformed the recipe import.yml for local mode:

- Services with `zeropsSetup: dev` were dropped (your CWD replaces them).
- `buildFromGit:` was stripped from the remaining runtime — the stage starts in `READY_TO_DEPLOY`, NOT auto-seeded from upstream.

Submit the transformed YAML via `zerops_import` as usual. Stage runtime + managed deps come up; stage subdomain returns 502/503 until your first local deploy lands code on it.

### After services are RUNNING / READY_TO_DEPLOY

```
1. zerops_env action="get" project=true       # surface project envVariables
2. zerops_env action="generate-dotenv" setup="<setup-name>"
                                                # renders .env from project + zerops.yaml + .env.local
3. Add ".env" + ".env.local" to .gitignore if not already there
4. Guide user: zcli vpn up <projectId>          # access managed services from your machine
5. composer install / npm install / framework equivalent
6. First local run: php artisan serve / npm run dev / etc.
```

### First deploy is the mandatory bootstrap completion

Stage was created without `buildFromGit`, so its subdomain is dead until you push real code.

```
zerops_deploy targetService="<stage-hostname>" workingDir="<cwd>"
```

This single deploy verifies the build pipeline, env wiring, and runtime startup end-to-end. After it lands, stage subdomain becomes live. Until you run it, the bootstrap is not complete.

Subsequent iterations: edit locally → re-run `npm run dev` (etc.) for fast feedback → `zerops_deploy` to update stage when validating production-shape builds.
