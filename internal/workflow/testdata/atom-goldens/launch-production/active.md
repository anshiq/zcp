---
id: launch-production/active
atomIds: [launch-delete-key, launch-intro, launch-post-checklist, launch-scope-prompt, launch-mutation-key-required, launch-write-prod-setup]
description: "Launch-production workflow mid-flow on a source project — bundle composed, awaiting one-shot launch key for the mutation pipeline."
---
### Delete the launch-window API key

The production project is live. **Delete the launch-window key now** so ZCP has no further path to mutate prod:

1. Open [Settings → Access Tokens Management](https://app.zerops.io/settings/token-management).
2. Find the token named `zcp-launch-<production-project-name>`.
3. Click **Revoke** (or **Delete**).

ZCP has already discarded the in-memory copy. Revoking the key in Zerops dashboard closes the trust boundary completely.

---

### Launch production — overview

You are launching the source project to a separate Zerops production project. ZCP prepares the bundle, source-control changes, and verification steps; you (the user) generate a one-shot Zerops API key for the mutation window and **delete that key** after launch completes.

Six top-level statuses gate progress:

| Status | Means |
|---|---|
| `scope-prompt` | ZCP needs: production project name, region, optional custom domain, scaling overrides. |
| `classify-prompt` | Project envs need bucketing (infrastructure / auto-secret / external-secret / plain-config). |
| `ready-to-launch` | Bundle composed, source-control changes pushed, schema clean, blockers cleared. Awaiting one-shot launch key. |
| `launching` | One-shot key in use; ZCP is creating + importing + polling first deploy. |
| `failed` | A mutation step failed; `blockers[]` describes recovery. |
| `launched` | Done. Delete the launch key. Set external secrets in Zerops UI. Attach custom domain in Zerops UI per emitted DNS records. |

ZCP has **zero standing access** to the production project. The one-shot key flows in via the `launchKey` parameter only during `publish` action; it is never written to state, logs, or transcripts.

---

### Launch complete — user-owned steps remaining

ZCP has imported services and validated first deploy. The following steps require the user to act in the Zerops dashboard. ZCP cannot perform them (no standing prod access).

1. **Delete the launch-window key** — open Settings → Access Tokens Management and revoke the token named `zcp-launch-<production-project-name>`.
2. **Set external secrets** — open the production project, navigate to each service that needs Stripe/OpenAI/SMTP/etc. values, and set them under Env Variables → Secret. ZCP listed the keys needed in the prior response.
3. **Attach custom domain** (if requested at scope time) — Project → Public Access → HTTP Routing → Add Domain. Use the DNS records ZCP emitted; add them at the registrar; click Verify in dashboard.
4. **Verify production smoke test** — hit the live URL with a known request shape; check response and logs in dashboard.

After step 4 passes, the launch is complete. For ongoing prod iteration: generate a separate project-scoped `ZCP_API_KEY` (Custom access per project, this one project, Full access) and configure a fresh ZCP MCP session against the production project.

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

---

### One-shot API key required for publish

ZCP cannot create the production project with its standing token (project-scoped). The user generates a temporary **account-wide** Zerops API key for the launch window:

1. Open [Settings → Access Tokens Management](https://app.zerops.io/settings/token-management).
2. Click **Create token**. Name it `zcp-launch-<production-project-name>`.
3. Leave **Custom access per project** UNCHECKED — needs account-wide scope to create projects.
4. Copy the token value (shown once).

Re-call the launch workflow with the publish action and the token value passed as `launchKey`.

The key flows through the workflow handler only — never persisted to state, logs, or transcripts. Once the launch reaches `launched` status, ZCP returns a mandatory checklist that includes **deleting the key** at the same dashboard URL.

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
