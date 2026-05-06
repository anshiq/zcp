# test-pinning-elevation — 2026-05-06 (rewritten after dual review)

Goal: prevent the same defect class behind H1+H2+H3a from recurring. **First draft proposed AST scanner + eval-corpus table; both reviewers (codex resume + codex fresh) found that draft fundamentally wrong.** This document reflects the post-review state.

**Parent context**: `plans/archive/eval-findings-fix-plan-2026-05-05.md` shipped 4 fixes (commits `410c419f`, `312d7391`, `703274a0`, `c488754e`).

---

## Why the first draft was wrong

The original plan proposed:
- **T1**: AST scanner over `internal/tools/` flagging `convertError(...ErrPrerequisiteMissing..., WithRecoveryStatus())`
- **T2**: separate eval-corpus regression table for classifier signals

Both reviewers independently surfaced fatal flaws:

### T1 (AST) — wrong shape
1. **Wouldn't be GREEN today.** Production code at `workflow_adopt_local.go:51`, `workflow_record_deploy.go:203/218/228/234/240`, `workflow_develop.go:195`, and `deploy_local_git.go:104-166` all use `ErrPrerequisiteMissing + WithRecoveryStatus()`. T1's invariant "PREREQUISITE_MISSING = actionable = specific Recovery" is **false** for at least 5 of 18 sites — they're transient (events fetch failed), manual ("restart ZCP"), or filesystem-state ("workingDir not git"). Walker would produce real false positives needing immediate allow-list comments.
2. **Misses one of two H2 fix sites.** `workflow_close_mode.go:90` uses `ErrServiceNotFound`, not `ErrPrerequisiteMissing`. T1 narrowed to PREREQUISITE_MISSING wouldn't catch a regression there.
3. **Cross-function flow blind spot is concrete, not theoretical.** `deploy_strategy_gate.go:69` builds the error inside `checkLocalOnlyGate`; `deploy_local.go:98` calls `convertError(err, WithRecoveryStatus())` with opaque `err`. Walker can't trace cross-file helper construction. Already present in current tree.
4. **Plan claimed "H2 already swept" — falsified.** I missed `workflow_develop.go:195-198` (errStandardPairStageMissing path) in my own H2 sweep. It still has generic Recovery despite being agent-actionable via bootstrap+adopt.

### T2 (eval corpus) — wrong shape
1. **`eval/behavioral/README.md:33`**: explicit "not a CI gate"; `:187`: "no automated grader". Manually-maintained table anchored only to README prose has zero enforcement — author forgets to append row, T2 doesn't fail because there's no missing-row detector.
2. **Splitting from existing `TestClassifyDeployFailure_*` is unjustified.** Existing tables already accept eval-derived rows (e.g. `deploy_failure_test.go:100-114` is the H3a empty-BuildLogs row I just added). Splitting adds maintenance friction without reproducibility gain.
3. **Auto-derivation ruled out.** `eval/behavioral/.gitignore:1` excludes `runs/` from git; transcript files aren't tracked, so deriving corpus at test time isn't possible.

### Meta-pattern was overclaimed
First draft framed T1+T2 as "preventing the H1+H3a/H2-symptom defect class". Codex fresh: "T1 only fires on one symptom shape; doesn't touch `build_plan.go:382-395` (H2's router root), misses close-mode site (ErrServiceNotFound). The plan claims broader coverage than T1 actually provides." Drop the unified framing.

---

## What goes in instead

The substance under both reviews points the same direction: **the right fix is structural, not test-pinning**.

The error code `ErrPrerequisiteMissing` is too coarse — it conflates 5+ semantic categories under one wire string:
- "service not bootstrapped, agent must adopt" (H2 territory — agent-actionable via bootstrap+adopt)
- "transient API failure" (retry via status)
- "manual operator action required" (restart ZCP, edit config)
- "filesystem state needs setup" (git init, mount path)
- "deploy strategy gated" (configure GitPushState first)

Per CLAUDE.local.md: *"When choosing between minimal patch and structural correction, pick structural unless the patch IS the correct design."* AST walker + allow-list comments is patch-on-coarse-taxonomy. Splitting the error code at the source is the structural correction.

---

## Revised plan: 3 changes

### S1 — introduce `ErrAdoptRequired` and migrate H2's three actual sites

**Goal**: encode the "agent must adopt before this operation" semantic in the error code itself. Recovery shape becomes self-evident from the code name.

**Migration**:
- `internal/platform/errors.go` — new constant `ErrAdoptRequired = "ADOPT_REQUIRED"`
- `internal/tools/workflow_develop.go:113` — change `ErrPrerequisiteMissing` → `ErrAdoptRequired` (already has specific Recovery from H2 fix)
- `internal/tools/workflow_develop.go:195` — same migration. **This site I missed in original H2 sweep — fresh codex caught it.** errStandardPairStageMissing is agent-actionable (re-bootstrap with route=adopt); currently has prose hint but `WithRecoveryStatus()` Recovery. Migrate code AND replace `WithRecoveryStatus()` with specific Recovery.
- `internal/tools/workflow_close_mode.go:90` — change `ErrServiceNotFound` → `ErrAdoptRequired` (semantically more accurate; service IS found, just unmanaged). Recovery already specific from H2 fix.

After migration, `ErrAdoptRequired` appears at exactly 3 sites, each with specific Recovery `{tool: zerops_workflow, action: start, args: {workflow: bootstrap, route: adopt}}`. The contract is self-enforcing through the error code name.

**Why also workflow_close_mode.go:90 (ErrServiceNotFound → ErrAdoptRequired)**: codex fresh's grep showed close_mode emits `ErrServiceNotFound` for "Service is not bootstrapped". That's miscategorized — the service IS found, it's just unmanaged. ErrAdoptRequired is correct semantically. Migrating to it also makes the per-error-code Recovery contract uniform.

### S2 — narrow test for ErrAdoptRequired contract (replaces T1)

**Single enumerated test** in `internal/tools/recovery_contract_test.go` (new file):

```go
// TestErrAdoptRequiredCarriesAdoptRecovery pins the contract that every
// ErrAdoptRequired emission carries Recovery pointing at bootstrap+adopt.
// The error code's name encodes the next-call; the test verifies the name
// is honored. New ErrAdoptRequired sites added without specific Recovery
// fail this test.
//
// Why narrow to this code: ErrAdoptRequired is semantically specific
// (service exists, agent must run bootstrap+adopt before working with it).
// Other PREREQUISITE_MISSING-class codes legitimately use status Recovery
// for transient/manual/filesystem cases — those are not in scope.
func TestErrAdoptRequiredCarriesAdoptRecovery(t *testing.T) { ... }
```

Drives the three handlers with inputs that trigger each ErrAdoptRequired path; asserts Recovery shape.

**Why enumerated, not AST**: 3 known sites; ErrAdoptRequired's narrow semantic guarantees the table stays small. AST scanner's value (catch new sites without enumeration drift) is moot when the error code itself is so specific that any new site is intentional and visible. Enumerated table forces the author to think about both code emit AND test row at the same time.

### S3 — fix missed test gap in TestClassifyDeployFailure_Prepare

Fresh codex's other catch: H3a fix made `baselineForPhase(PhasePrepare)` hasLogs-aware, but `TestClassifyDeployFailure_Prepare` has no empty-BuildLogs regression row. Add one mirroring the build-baseline-empty-logs case.

```go
{
    name: "prepare-baseline-empty-logs",
    input: FailureInput{
        Phase:     PhasePrepare,
        BuildLogs: nil,
    },
    wantCategory:             topology.FailureClassStart,
    wantSignal:               "phase:prepare",
    wantNotInSuggestedAction: "Read buildLogs",
},
```

(Requires extending the Prepare test struct with `wantNotInSuggestedAction` field — same shape as Build struct).

---

## Explicitly NOT done (each rejected with reason)

- **AST walker for ErrPrerequisiteMissing class** — false positives on legitimate transient/manual/filesystem cases; cross-function flow blind spot already exists in tree; produces "fix everything to silence me" pressure.
- **Eval-corpus separate table** — fresh codex's grep at `deploy_failure_test.go:404-425` confirms existing tables already host eval-derived cases; splitting adds friction without gain.
- **Type-system Recovery requirement** (Recovery as PlatformError struct field) — fresh codex was right that this would handle helper-built error flow that AST can't. But the scope is sweeping (every PlatformError carries Recovery), and ErrAdoptRequired solves the immediate H2 contract concern. Reconsider if more error-code-vs-Recovery contracts emerge.
- **Router-handler agreement contract test** — fresh codex flagged this as right call for catching H2's actual root (routing layer disagreement at `build_plan.go:382-395`). I'm leaving this OUT of priority-1 elevation for scope reasons; it's a different test idiom and warrants its own plan. Logging as next-after backlog candidate.
- **Parameter-threading symmetry AST** — codex resume's note: behavioral test catches present case but not future field additions. Fair, but cheap mitigation is a `// invariant: status-dispatch functions in this file take (status string, hasLogs bool)` comment + a `grep` in the test, not a walker. Out of scope; revisit if H3a-class regresses.
- **Fix `setup="prod"` hardcoding at `build_plan.go:265`** — fresh codex caught that `build_plan_test.go:172-174` pins the known-bad assumption with a test, locking in symptom-fix. Real concern, but tracked under `audit-prerelease-internal-testing-2026-04-29.md` H1 §3 (per code comment at lines 252-256). Don't duplicate.

---

## RED-GREEN cadence

**S1** (introduce ErrAdoptRequired + migrate):
- Pure refactor for `workflow_develop.go:113` (already has specific Recovery; just rename code) — no RED needed, but change-impact verification across affected layers.
- For `workflow_develop.go:195` and (potentially) `workflow_close_mode.go:90`: behavior change (Recovery shape change). RED test FIRST per TDD invariant.
- Atom corpus check: confirm no atom prose references "PREREQUISITE_MISSING" in a way that breaks when develop:113 changes wire string. Already verified: only `setup-git-push-*.md` atoms reference PREREQUISITE_MISSING and they're for git-push setup (different sites, unaffected by migration).

**S2** (recovery_contract_test.go):
- After S1 migration, write enumerated test asserting all 3 ErrAdoptRequired sites emit specific Recovery. GREEN immediately on post-S1 code.
- Manual RED demo (post-hoc): revert one site's Recovery to status, confirm test fails, restore.

**S3** (Prepare empty-logs):
- RED first: add table row, run, confirm fail (current PhasePrepare baseline has unconditional `SuggestedAction: "Read buildLogs..."` — wait, no, my H3a fix already conditioned it). Verify what current state is before claiming RED.

Actually: after H3a fix (commit `410c419f`), `baselineForPhase(PhasePrepare)` IS hasLogs-aware. So S3 row is GREEN immediately, just regression-locks existing behavior. Add the row, no RED phase.

---

## Cross-validation log

**Codex resume**:
- Confirmed AST narrow scope claims false positives at 3+ sites
- Confirmed cross-function flow blind spot (deploy_strategy_gate → deploy_local)
- Flagged eval-corpus protocol as bit-rot risk (no missing-row detector)
- Pushed back on parameter-threading-symmetry quick-rejection
- Flagged T1's "meta-pattern" framing as overclaim

**Codex fresh** (cold-eyes):
- Independent confirmation of all of resume's concerns
- Plus: `workflow_close_mode.go:90` uses ErrServiceNotFound, T1 misses it
- Plus: I missed `workflow_develop.go:195` in original H2 sweep
- Plus: TestClassifyDeployFailure_Prepare missing empty-logs row (H3a coverage gap)
- Plus: `build_plan.go:265` setup="prod" hardcoding pinned by `build_plan_test.go:172-174` (test locks known-bad)
- Pushed back on type-system rejection (says it WOULD handle cross-function flow correctly)

**Convergence**: both reviewers agree T1 (AST) and T2 (eval corpus) as proposed are wrong. Both implicitly point at structural alternatives. Fresh codex more explicit about scope critique.

**Divergences**: codex resume proposed narrowing T1 to workflow_*.go only; codex fresh proposed type-system fix instead. I picked a third path (split error code) that's smaller scope than type-system but cleaner than narrowed AST.

---

## Risks of revised plan

- **ErrAdoptRequired wire-string change is observable to agents.** Agents reading a stable failure-code list may have learned `PREREQUISITE_MISSING` as the name. Atoms checked — only setup-git-push-*.md atoms cite the string and those reference different sites (unaffected). Eval scenarios may expect specific code; verify via `grep PREREQUISITE_MISSING eval/`.
- **One-site sweep is small enough to forget**: 3 sites today. Future site author may emit `ErrPrerequisiteMissing` when they should emit `ErrAdoptRequired`. The narrow test S2 catches the symptom but only when the wrong-coded site emits; doesn't prevent a wrong-naming choice. Mitigation: docstring on `ErrAdoptRequired` constant explains semantics + link to test.
- **Recovery contract becomes per-code rather than per-site invariant** — over time more error codes might benefit from analogous contracts. If yes, the right next step is the type-system refactor (Recovery as PlatformError field) rather than ad-hoc per-code tests.

---

## Order of operations

1. Verify no eval scenarios or atoms break on `PREREQUISITE_MISSING` → `ADOPT_REQUIRED` wire-string change. (`grep -rn PREREQUISITE_MISSING eval/ docs/ ../zerops-docs/`)
2. S1: introduce `ErrAdoptRequired` + migrate 3 sites + update existing H2 test (`TestHandleDevelopBriefing_NoBootstrappedServices_RecoveryPointsAtBootstrapAdopt`) which currently asserts `PREREQUISITE_MISSING` substring → must update to `ADOPT_REQUIRED`.
3. S2: write `recovery_contract_test.go` with enumerated table.
4. S3: add empty-logs row to `TestClassifyDeployFailure_Prepare`.
5. Run all layers (`go test -race ./... -count=1` + `make lint-local`) to confirm clean.

Single commit OR three separate (S1 + S2 + S3) — leaning toward two: S1+S2 together (they're a cohesive contract intro) and S3 separate (different concern).
