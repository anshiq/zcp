---
id: greenfield-node-postgres
description: |
  Scenario B (greenfield local) from the env-handling design pass.
  Empty Zerops project + empty CWD; user says "I want to start a
  Node + Postgres project locally". Tests whether bootstrap classic
  route synthesizes both the project (import.yml with managed db)
  AND the local artefacts (zerops.yaml + minimal app skeleton +
  initial .env.local seed) and ends in a runnable local app deploying
  to a stage runtime.

  This is the existing classic-local path under the new env-handling
  contract — verifies that the three-channel model (project envVars
  + zerops.yaml run.envVars + .env.local overlay) integrates cleanly
  with the greenfield greenfield path. No new ZCP code under test;
  the eval observes whether atom guidance steers correctly.
seed: empty
fixture: fixtures/greenfield-node-postgres.yaml
preseedScript: preseed/greenfield-node-postgres.sh
tags: [local-mode, classic-route, greenfield, env-channels, first-deploy, node, postgres]
area: local-mode
retrospective:
  promptStyle: briefing-future-agent
notableFriction:
  - id: env-channel-confusion
    description: |
      Agent might put shared secrets in zerops.yaml run.envVariables
      (deployed-only channel) instead of project.envVariables (shared
      channel) — the resulting deployed runtime keeps secrets but the
      local .env doesn't get them. Tests whether the develop-local-
      env-channels atom decision tree fires and is followed.
    suspectedCauses:
      - develop-local-env-channels atom may not surface during bootstrap
      - bootstrap-active phase atoms may steer differently from develop-active
  - id: zerops-yaml-shape
    description: |
      Bootstrap classic-local writes a zerops.yaml that the agent
      authors itself. Schema has a single setup block named after the
      runtime (commonly "app"). Tests whether scaffold-zerops-yaml
      atom generates a working setup with sensible run.envVariables
      that survive generate-dotenv.
    suspectedCauses:
      - scaffold-zerops-yaml atom guidance for run.envVariables shape
      - zerops.yaml schema validation surfacing
  - id: missed-env-local-seed
    description: |
      The new EnsureEnvLocal helper isn't yet wired into bootstrap
      classic-local — agent must author .env.local manually if they
      want APP_ENV=local-style overrides. Tests whether the lack of
      wiring causes friction or whether agents naturally edit
      .env.local after generate-dotenv.
    suspectedCauses:
      - bootstrap-classic-* atoms don't reference .env.local
      - EnsureEnvLocal is library-only, not invoked by any handler yet
---

I want to start a new Node.js project that uses PostgreSQL for storage. Empty folder. Please set it up on Zerops so I can develop locally and deploy when ready. Use the bootstrap workflow (`zerops_workflow workflow="bootstrap"`) for the initial setup.
