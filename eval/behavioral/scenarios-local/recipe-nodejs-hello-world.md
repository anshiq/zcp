---
id: recipe-nodejs-hello-world
description: |
  Recipe-route in local mode (Theme 1): empty Zerops project + empty
  CWD. Agent must (a) match the nodejs-hello-world recipe, (b) submit
  the localized import.yml (ZCP drops appdev + strips buildFromGit
  during the transform), (c) clone the recipe app repo into the CWD,
  (d) generate-dotenv from the local zerops.yaml, (e) run the app
  locally, (f) deploy to stage as the mandatory bootstrap-completion
  step. Stage subdomain is dead until step (f) lands code.
seed: empty
fixture: fixtures/recipe-nodejs-hello-world.yaml
preseedScript: preseed/recipe-nodejs-hello-world.sh
tags: [local-mode, recipe-route, first-deploy, node, postgres, env-bridge]
area: local-mode
retrospective:
  promptStyle: briefing-future-agent
notableFriction:
  - id: missed-recipe-route
    description: |
      Agent might pick the classic route and write zerops.yaml
      manually instead of matching the nodejs-hello-world recipe.
      Tests whether recipe matching surfaces strongly enough at
      bootstrap discovery.
    suspectedCauses:
      - bootstrap-route-options atom may not rank recipe match high enough
      - recipe corpus matching against the user's intent string
  - id: skip-clone-step
    description: |
      Agent might submit the import without cloning the recipe app
      repo into CWD. Local mode has no SSH-in dev workspace — without
      the clone the local app cannot run. Tests whether
      bootstrap-recipe-local-clone atom fires and is followed.
    suspectedCauses:
      - atom filter may not match this scenario's bootstrap state
      - RecipeMatch.Repo plumbing may not surface in atom template vars
  - id: forgot-first-deploy
    description: |
      Stage runtime starts in READY_TO_DEPLOY (buildFromGit stripped).
      Agent might mark bootstrap complete without deploying — stage
      subdomain stays 502/503. Bootstrap-recipe-import-local atom
      explicitly mandates the first deploy as the completion step.
    suspectedCauses:
      - close step may not check for first-deploy in local-recipe context
      - atom guidance about "mandatory" may not be salient enough
  - id: env-bridge-shape
    description: |
      Generate-dotenv must use setup parameter (not legacy
      serviceHostname) for clarity in recipe context where setup
      names like 'prod' aren't always service hostnames.
    suspectedCauses:
      - tool description may still steer toward serviceHostname
      - atom may not emphasize the new setup parameter
---

I want to start a simple Node.js + Postgres project locally on Zerops. Empty folder, get me a working dev setup. Use the bootstrap workflow (`zerops_workflow workflow="bootstrap"`) — that's the entry point for any new project, including ones that match a Zerops recipe.
