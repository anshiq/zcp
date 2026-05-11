---
id: launch-write-prod-setup
priority: 5
phases: [launch-production-active]
title: "Write prod setup block to source zerops.yaml"
references-fields: []
coverageExempt: "Phase C lands atoms ahead of Phase D handler + Phase E scenarios; pin via launch-production-active fixtures in Phase E per plans/production-lifecycle-2026-05-11.md §11"
---

### Write prod setup block to source zerops.yaml

Launch needs `setup: prod` in the source repo's `zerops.yaml` **before** publishing. Production builds from the same git URL as dev/stage; the prod-specific build/run commands live under a separate `setup:` entry that the launch bundle references.

Append the block to `zerops.yaml` at repo root, commit, and push to the configured remote. The launch workflow verifies the block exists before mutating the destination project.

```yaml
zerops:
  - setup: prod
    build:
      base: <runtime>
      buildCommands:
        - <production build commands — typically same as stage with NODE_ENV=production or APP_ENV=production semantics>
      deployFiles: <production deploy artifact paths>
    run:
      base: <runtime>
      start: <production start command>
      healthCheck:
        httpGet:
          port: <port>
          path: /health
```

`healthCheck` is required — production deploys gate readiness via the `prod-healthcheck-required` blocker if missing.

After commit + push, re-call `zerops_workflow workflow="launch-production" action="status"` to re-probe; the workflow advances to `ready-to-launch` once the block resolves.
