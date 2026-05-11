---
id: launch-scope-prompt
priority: 2
phases: [launch-production-active]
title: "Launch scope — collect production target details"
references-fields: []
---

### Launch scope — collect production target details

Ask the user for the target shape, then re-call with the inputs.

| Input | Required | Notes |
|---|---|---|
| `productionProjectName` | yes | New Zerops project name (must not collide with existing projects in the org). |
| `region` | yes | Default `eu-central`. Other supported values from Zerops dashboard. |
| `customDomain` | no | If set, ZCP synthesizes DNS records + verification probes; user attaches in Zerops UI. |
| `keepNonHA` | no | Array of managed-service hostnames to keep at `NON_HA` in prod (default: all promoted to `HA`). |
| `envOverrides` | no | Plain-config env value overrides for the prod bundle. **No secret values here** — ZCP never receives them. |

After scope is complete, ZCP advances to `classify-prompt` for the project-env classification pass.
