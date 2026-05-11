---
id: launch-delete-key
priority: 1
phases: [launch-production-active]
title: "Delete the launch-window API key"
references-fields: []
coverageExempt: "Phase C lands atoms ahead of Phase D handler + Phase E scenarios; pin via launch-production-active fixtures in Phase E per plans/production-lifecycle-2026-05-11.md §11"
---

### Delete the launch-window API key

The production project is live. **Delete the launch-window key now** so ZCP has no further path to mutate prod:

1. Open [Settings → Access Tokens Management](https://app.zerops.io/settings/token-management).
2. Find the token named `zcp-launch-<production-project-name>`.
3. Click **Revoke** (or **Delete**).

ZCP has already discarded the in-memory copy. Revoking the key in Zerops dashboard closes the trust boundary completely.
