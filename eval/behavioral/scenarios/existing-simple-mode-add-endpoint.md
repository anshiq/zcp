---
id: existing-simple-mode-add-endpoint
description: |
  DEFERRED — DO NOT AUTO-LOAD.

  Intentionally captures an unresolved edge case: importing a runtime
  service via `buildFromGit` WITHOUT `startWithoutCode: true` and then
  expecting ZCP to act on it as a dev-style adoptable service. The
  fixture used here (`fixtures/python-simple-deployed.yaml`, buildFromGit
  to `zerops-recipe-apps/python-hello-world-app`) reliably fails at the
  `SeedDeployed` step with a `process … failed: unknown` because the
  imported service never reaches ACTIVE within the 15-minute wait
  budget under that combination.

  Historically the resolution pattern for this case is "import then
  flip `startWithoutCode: true`" or a similar replace-procedure so a
  freshly-imported runtime can be worked on as a dev service before it
  has any deploy. This scenario is left in the corpus as a marker but
  has its `retrospective:` block removed so `loadBehavioralScenarios`
  skips it (no auto-load via `flow-eval all`); a dedicated, fully-
  controlled scenario for the import-without-startWithoutCode behavior
  will be authored under `plans/backlog/test-import-without-startwithoutcode-pattern.md`
  and replace this one when ready.

  The simple-mode adopt coverage slot is currently held by
  `existing-simple-mode-node-add-endpoint` (working fixture using the
  prod setup of `nodejs-hello-world-app`).
seed: deployed
fixture: fixtures/python-simple-deployed.yaml
tags: [deferred, adopt, simple-mode, self-deploy, develop, python, no-stage, import-without-startWithoutCode]
area: adopt-and-develop
userPersona: |
  Your single Python service `api` is running on Zerops in simple
  mode (one container, no staging). You want to add `GET /version`
  returning a JSON object with the current build SHA, and have it
  deployed and verified. You don't want a staging slot — keep it as
  one immutable runtime. Push back if the agent proposes promoting
  to a dev/stage pair or treats this as a fresh bootstrap.
notableFriction:
  - id: adopt-simple-no-stage
    description: |
      Adopting a simple-mode service should not surface a
      stage-promote question. Surfaces whether the adopt atom
      branches on mode rather than always asking the standard-pair
      stage question.
  - id: simple-mode-self-deploy
    description: |
      Re-deploy of a simple-mode runtime is self-deploy (DM-2). Agent
      must NOT narrow deployFiles below `[.]` for a self-deploy.
      Surfaces whether the deploy atom flags self-deploy on a single
      runtime.
  - id: import-without-startWithoutCode-edge
    description: |
      The fixture imports buildFromGit without startWithoutCode:true.
      Today this combination strands the seed at "process failed:
      unknown" before the agent even sees a project. The dedicated
      scenario in the backlog will cover the recovery pattern (flip
      startWithoutCode after import or equivalent procedure) under a
      controlled fixture.
---

The `api` Python service on Zerops is up and running. Add a `GET /version` endpoint that returns the current build SHA as JSON, then deploy and verify it. Keep it as one container — no staging slot.
