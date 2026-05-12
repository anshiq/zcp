---
title: Inject project.userRoles into launch-production import yaml
status: rejected
rejected: 2026-05-12
---

## Why rejected

Empirically verified 2026-05-11 against real Zerops platform with admin
token (`canCreateProjects: true`): `PostClientProjectImport` **silently
drops** the `project.userRoles[]` field in the import yaml. Project gets
created without any explicit per-project role for the launching token,
regardless of yaml content. Inspected post-create via
`PostProjectSearch` — `userRoles=[]` whether yaml carries the field or
not.

## Working fix shipped instead (v9.84.1)

`ProjectAdminClient.GrantSelfRole(ctx, projectID, roleCode)` makes a
separate `PutClientUserRoles` API call after import succeeds. Reads
existing role list, merges the new entry, writes back. Documented in
`docs/spec-launch-production-platform-spike.md §A.10`.

## Why the original idea looked attractive

- **Atomicity**: one API call instead of two (create + grant). If grant
  fails after create succeeds, we have a project without ZCP-recorded
  per-project role. Mitigation today: bundle warning, user reads via UI.
  Functional but not pristine.
- **Multi-user support**: import yaml could grant ADMIN to multiple team
  members at create time. Today's GrantSelfRole only handles the
  launching user; team additions need separate `PutClientUserRoles`
  calls.

Both upsides are theoretical. The atomicity concern is mitigated by the
platform's auto-OWNER-assignment to the creating clientUser (verified
in the same e2e run — clientUser has OWNER role even without an
explicit grant call). Multi-user grants are not a v1 use case ZCP needs.

## What would change the calculus

- Platform team confirms a supported field shape for import-yaml role
  assignment (different field name? different endpoint? schema
  version?).
- Live JSON schema (`import-project-yml-json-schema.json`) explicitly
  documents the field.
- Multi-user grant becomes a v1 launch-production requirement (user
  feedback after dogfooding).

Until any of those land, the post-create `GrantSelfRole` path is the
canonical solution. Re-opening would just rediscover the silent-drop
behavior.

## References

- Spike doc: `docs/spec-launch-production-platform-spike.md §A.10`
- Working fix commit: `acef05c2` (v9.84.1)
- e2e verification: `internal/platform/project_admin_api_test.go::TestProjectAdminClient_GetServiceEnvKeys_OmitsValues`
