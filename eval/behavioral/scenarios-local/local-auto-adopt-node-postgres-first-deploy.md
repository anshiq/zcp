---
id: local-auto-adopt-node-postgres-first-deploy
description: |
  First behavioral scenario for LOCAL mode (agent runs on a developer
  Mac, not a Zerops container). Project pre-seeded with one
  undeployed nodejs runtime + one postgres; preseed writes a minimal
  Node notes API into the workdir. Tests auto-adopt classification,
  deploy_local schema (no sourceService), .env bridge, and VPN
  guidance — the local-mode-only navigation surface.
seed: settled
fixture: fixtures/local-auto-adopt-node-postgres.yaml
preseedScript: preseed/local-auto-adopt-node-postgres.sh
tags: [local-mode, auto-adopt, deploy-local, env-bridge, first-deploy, node, postgres]
area: local-mode
retrospective:
  promptStyle: briefing-future-agent
notableFriction:
  # Informational only — does NOT gate the run. Helps the local Claude
  # session spot what to look for in the retrospective.
  - id: schema-confusion
    description: |
      Local mode's zerops_deploy schema has no sourceService. Agent may
      hallucinate the field by analogy with container mode (where SSH
      deploy uses it for cross-deploy). The deploy should call only
      targetService=app + workingDir=<cwd>.
    suspectedCauses:
      - internal/tools/deploy_local.go schema lacks sourceService → instrumentation differs from internal/tools/deploy_ssh.go
      - atom corpus may not surface the local-vs-container schema split
  - id: redundant-bootstrap-attempt
    description: |
      Auto-adopt has already linked `app` as local-stage at server
      start. Agent may still try zerops_workflow workflow="bootstrap"
      because the adoption note may not be salient enough in the
      instructions, or the agent may default to bootstrap-first
      reflexes from container-mode evals.
    suspectedCauses:
      - server.go::runLocalAutoAdopt note formatting + adoption-note salience
      - workflow.FormatAdoptionNote shape vs first-call routing decision
  - id: env-bridge-missed
    description: |
      Agent must generate .env from zerops_env action="generate-dotenv"
      so server.js can read ${db_*} when run on the operator's Mac.
      May skip this step entirely (just deploy + done) or build .env
      by hand from zerops_discover output instead of using the dedicated
      tool path.
    suspectedCauses:
      - tool description for zerops_env action="generate-dotenv" may not surface from the agent's mental model
      - .env-bridge atom guidance may be conditioned on managed-service detection that's not firing
  - id: vpn-command-shape
    description: |
      For local development against managed services the agent should
      emit `zcli vpn up <projectId>` with the actual project ID, not a
      placeholder. Surfaces whether VPN guidance interpolates project
      context correctly.
---

This folder has a small Node notes API. Please deploy it to my Zerops project and set up local database env vars so I can run it on my machine.
