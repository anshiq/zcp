---
Surfaced: 2026-05-04 — flow-eval suite `20260504-104436` scenario `existing-simple-mode-add-endpoint` failed at seed (`process … failed: unknown`) on a buildFromGit fixture without `startWithoutCode: true`. The case is intentionally interesting: ZCP should be able to recover such a service into a workable dev/simple state via a "flip startWithoutCode true" replace pattern (or equivalent), and the recovery procedure isn't covered anywhere today. Marked deferred so the in-suite scenario doesn't keep failing while we figure out the dedicated test.
Why deferred: needs an isolated, fully-controlled fixture + a clear contract for what ZCP is supposed to do when this combination is presented. Not the kind of test that should ride a behavioral retrospective without its own seed-success story first.
Trigger to promote: when we have (a) the recovery procedure agreed (flip startWithoutCode true vs alternative), and (b) a fixture that reliably reproduces the trapped-import state without flaking at seed.
---

# Dedicated test: import-without-`startWithoutCode` recovery pattern

## Problem

A `services[]` entry in import.yaml that sets `buildFromGit:` but does **not** set `startWithoutCode: true` reaches the platform but the deploy process can hang or fail (`process … failed: unknown`) when the build artefact does not match what the runtime expects to start. The behavioral suite seed step hits this with the python-hello-world-app repo — `SeedDeployed` waits 15 minutes for ACTIVE and times out.

The actual case worth testing is **after** import succeeds: a runtime exists in some non-deployed-but-imported state, and the user wants to work on it as a dev/simple service. The historical pattern for that recovery is to flip `startWithoutCode: true` (via a replace API path or zcli command) so the runtime idles, then the agent can adopt + deploy fresh code.

## Why this isn't `existing-simple-mode-add-endpoint`

That scenario was authored as an adopt-into-simple-mode coverage slot. It accidentally reproduces the import-without-startWithoutCode trap because the fixture didn't set `startWithoutCode: true`. That makes it a poor adopt test (never gets past seed) and a confusing edge-case test (no fixture-level control over the failure mode). The replacement `existing-simple-mode-node-add-endpoint` uses a fixture with `zeropsSetup: prod` against the nodejs-hello-world-app repo (a known-working combination) and covers the original adopt-into-simple semantic cleanly.

## What the dedicated scenario should do

1. **Reliably reproduce the trapped-import state** at seed without timing out. Either:
   - Use a fixture variant that imports the service in a known-trapped state (e.g. type=runtime, buildFromGit pointing at a deliberately-incompatible repo with a build that succeeds but produces no usable artefact).
   - Or import with `startWithoutCode: true`, deploy bad code via the test setup, then strip the `startWithoutCode` flag via a replace call, capturing the trapped post-import state explicitly.
2. **Hand the agent a clear user prompt**: "I've got a runtime that's stuck — it's there in the project but the build never produced a working app. Can you fix it so I can work on it as a dev service?"
3. **Test the recovery procedure**: agent should reach the replace-startWithoutCode path (or whichever procedure ZCP commits to) and recover the runtime to an idling state, then deploy fresh code.

## Constraints

- Must not flake at seed. If the fixture takes >2 minutes to reach a stable trapped state, the test design is wrong; pick a simpler reproduction.
- Must not depend on the live state of any external GitHub repo. Either use a vendored fixture or a Zerops-controlled mailpit-style utility repo.
- Should NOT be combined with the working simple-mode-adopt slot. They test different things — keep them separate scenarios so retrospectives are crisp.

## Open design question

Whether ZCP should expose the recovery as:

- A workflow step (`zerops_workflow workflow=develop scope=[trapped-host] action=…` somehow surfacing the flip path), or
- A direct ops primitive (`zerops_manage action="reset-to-startWithoutCode" hostname=…` or similar), or
- Documentation-only (atom guides agent to do it via existing `zerops_import` replace semantics).

Pick this before writing the scenario — the scenario's expected agent path depends on the decision.

## Related

- `plans/archive/atom-edits-bootstrap-verify-2026-05-04.md` — the suite where this trap surfaced.
- `eval/behavioral/scenarios/existing-simple-mode-add-endpoint.md` — currently disabled (no `retrospective:` block); kept in tree as a marker pointing at this plan.
- `eval/behavioral/scenarios/existing-simple-mode-node-add-endpoint.md` — replacement coverage of the original adopt-into-simple semantic.
