---
id: brownfield-existing-node-app
description: |
  Scenario C (brownfield local) from the env-handling design pass.
  CWD has a working Node + Postgres app with a populated `.env`,
  user says "deploy this to Zerops". Tests whether the existing
  adopt route handles brownfield projects with localhost-pointing DB
  URLs + secrets that need redistribution across the three channels
  (project envVariables / zerops.yaml run.envVariables / .env.local).

  IMPORTANT: Theme 3 (brownfield-adopt subroute with classify-dotenv
  + adopt-dotenv handlers) is DESIGN-ONLY in the current wave — only
  the architectural skeleton (SourceBrownfieldImport enum +
  brownfieldOverrides parameter) is in place. This eval observes the
  CURRENT brownfield experience using the existing adopt route +
  freshly-introduced env-handling primitives. Findings inform the
  next-wave Theme 3 implementation plan.
seed: empty
fixture: fixtures/brownfield-existing-node-app.yaml
preseed: preseed/brownfield-existing-node-app.sh
tags: [local-mode, adopt-route, brownfield, env-classify, env-channels, node, postgres]
area: local-mode
retrospective:
  promptStyle: briefing-future-agent
notableFriction:
  - id: dot-env-classification-gap
    description: |
      Agent reads existing `.env` (DATABASE_URL pointing to localhost,
      JWT_SECRET, NODE_ENV=development, LOG_LEVEL=debug, PORT, etc.)
      and must distribute these across project envVariables,
      zerops.yaml run.envVariables, and .env.local. ZCP has no
      classify-dotenv tool yet — agent must reason through the
      taxonomy from atom guidance alone. Tests whether the agent
      classifies cleanly or jumbles channels.
    suspectedCauses:
      - Theme 3 classify-dotenv tool does not exist
      - develop-local-env-channels atom decision tree may not generalize
        to "I have an existing .env, where do entries belong"
      - Adopt-route atoms predate the three-channel model
  - id: localhost-db-detection
    description: |
      Existing .env has DATABASE_URL=postgresql://localhost:5432/...
      meaning the user has local Postgres now. Agent should propose
      a managed Postgres on Zerops + rewrite DATABASE_URL to
      ${db_connectionString} ref. Tests whether localhost-URL ->
      managed-service inference fires, and whether the agent
      preserves or rotates JWT_SECRET et al.
    suspectedCauses:
      - No automated heuristic; agent must reason from URL scheme + hostname
      - Risk of agent silently dropping the existing JWT_SECRET (rotation breaks sessions)
  - id: backup-strategy
    description: |
      Theme 3 design calls for backing up the original `.env` to
      .zcp/state/backups/dotenv/<ts>.env before rewriting. ZCP
      doesn't auto-backup yet — agent might overwrite without backup.
      Tests whether the agent surfaces this concern unprompted.
    suspectedCauses:
      - No backup tool (deferred to Theme 3 implementation)
      - Atom guidance for adopt-route doesn't yet mention dotenv backup
  - id: env-mode-flag-split
    description: |
      Existing .env has NODE_ENV=development. The three-channel model
      says: deployed should have NODE_ENV=production, local override
      goes in .env.local. Agent should split the value across two
      channels rather than copying verbatim.
    suspectedCauses:
      - develop-local-env-channels.md decision tree explains the split
      - No active classify-dotenv flow to make this explicit during adoption
---

I have a Node.js + Express app that uses PostgreSQL. It's working locally with a `.env` file pointing at my local Postgres. I want to deploy it to Zerops — please get it running there with a managed Postgres while keeping my local development workflow intact.
