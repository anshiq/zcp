package recipe

import (
	"errors"
	"fmt"
	"strings"
)

// Run-41 — refinement-2 (cross-surface audit) brief composer.
//
// The first refinement pass walks per-fragment rules (`derived_rules.md`)
// — intra-fragment voice/structure/citation checks. Cross-surface
// defect classes (KB↔IG duplication, KB below-floor, surface-misplaced
// bullets, yaml-comment ↔ yaml-content named-constant drift,
// aspirational-as-current prose, cross-codebase named-constant drift)
// are invisible to that pass by construction: each check compares two
// or more fragments. Run-40 dogfood ([plans/run-40-validation.md])
// surfaced six such defects post-finalize (N-1, N-3, N-4, KB↔IG
// duplication on app + worker, JWT/MEILI_SEARCH_KEY aspirational
// prose) that refinement-1 ran clean over.
//
// The composer assembles only the load-bearing audit substrate from
// `docs/spec-content-surfaces.md` — phase entry (voice + dispatch
// shape) + audit checklist (the defect-class definitions). The spec
// document itself is engine-author reference; the agent never reads
// it (it's under docs/, not in `//go:embed all:content`).
//
// Output contract: diagnosis-only. The audit sub-agent emits a
// fenced JSON findings block; it does NOT call `record-fragment
// mode=replace`. The main agent reads the findings and decides per-
// finding whether to ACT, HOLD, or accept-as-known. This avoids the
// cross-rule conflict pattern that reverted refinement-1's ACT
// attempts in run-40 (slug-stem rule conflict).

// BuildRefinement2Brief composes the brief for the second refinement
// sub-agent dispatched at phase 8 (post-refinement-1, pre-finalize-
// close). Mirrors BuildRefinementBrief signature: plan, parent,
// outputRoot, facts.
//
// Brief content:
//
//  1. phase_entry — voice + output shape (JSON findings block).
//  2. audit_checklist — per-defect-class definitions with checks +
//     suggested actions.
//  3. Stitched output pointer block — paths the sub-agent reads.
//  4. Canonical-latest constants block — engine-rendered from
//     plan.NamedConstants + facts canonical topics so the audit can
//     compare on-surface values against the canonical store.
//  5. Citation map — which topics MUST cite which zerops_knowledge
//     guide; engine-rendered.
//
// No filesystem-local references leak; runDir is the published-
// recipe output root (porter-facing, deliverable-shipping path),
// not the author's working tree. Pinned by
// TestBuildRefinement2Brief_NoFilesystemReferenceLeak.
func BuildRefinement2Brief(plan *Plan, parent *ParentRecipe, runDir string, facts []FactRecord) (Brief, error) {
	if plan == nil {
		return Brief{}, errors.New("nil plan")
	}
	parts := []string{}
	var b strings.Builder

	// Phase entry — voice + dispatch + findings JSON shape.
	if atom, err := readAtom("briefs/refinement2/phase_entry.md"); err == nil {
		b.WriteString(atom)
		b.WriteString("\n\n")
		parts = append(parts, "briefs/refinement2/phase_entry.md")
	} else {
		return Brief{}, fmt.Errorf("read refinement2 phase_entry atom: %w", err)
	}

	// Audit checklist — per-defect-class definitions.
	if atom, err := readAtom("briefs/refinement2/audit_checklist.md"); err == nil {
		b.WriteString(atom)
		b.WriteString("\n\n")
		parts = append(parts, "briefs/refinement2/audit_checklist.md")
	} else {
		return Brief{}, fmt.Errorf("read refinement2 audit_checklist atom: %w", err)
	}

	// Stitched-output pointer block. runDir is the published-recipe
	// output root. Trailing-slash normalization mirrors
	// BuildRefinementBrief (run-40 S-3).
	if runDir != "" {
		runDir = strings.TrimRight(runDir, "/")
		b.WriteString("## Stitched output to audit\n\n")
		b.WriteString("Read each path end-to-end before running cross-surface checks. Don't `Grep`; cross-surface defects need full surfaces held in working memory.\n\n")
		b.WriteString("**Root**\n\n")
		fmt.Fprintf(&b, "- `%s/README.md` — root README (S1)\n", runDir)
		fmt.Fprintf(&b, "- `%s/environments/plan.json` — fragment store (canonical content for every fragmentId)\n", runDir)
		fmt.Fprintf(&b, "- `%s/environments/facts.jsonl` — recorded facts (audit citation source)\n", runDir)
		b.WriteString("\n**Tier environments (S2 + S3)**\n\n")
		for _, t := range Tiers() {
			fmt.Fprintf(&b, "- `%s/environments/%s/README.md` + `import.yaml`\n", runDir, t.Folder)
		}
		b.WriteString("\n**Codebases (S4 + S5 + S6 + S7)**\n\n")
		for _, cb := range plan.Codebases {
			fmt.Fprintf(&b, "- `%s/%sdev/README.md` + `zerops.yaml` + `CLAUDE.md` — IG (S4), KB (S5), CLAUDE.md (S6), yaml comments (S7)\n", runDir, cb.Hostname)
		}
		// Per-codebase dependency manifests for `aspirational-as-current`
		// framework-feature claim cross-check. The audit reads these
		// to confirm a claimed framework feature has its implementing
		// dependency in scope. The brief lists candidate paths for
		// every common manifest shape (Node, PHP, Python); the agent
		// reads whichever exist in the codebase dir.
		b.WriteString("\n**Per-codebase dependency manifests (for aspirational-as-current)**\n\n")
		for _, cb := range plan.Codebases {
			fmt.Fprintf(&b, "- `%s/%sdev/{package.json, composer.json, pyproject.toml, requirements.txt}` — read whichever exists\n", runDir, cb.Hostname)
		}
		b.WriteByte('\n')
		parts = append(parts, "stitched-output-pointer-block")
	}

	// Canonical-latest constants block — same shape the
	// refinement-1 + env-content composers render. The audit reads
	// it for cross-codebase-named-constant-drift + yaml-comment-
	// content-drift checks.
	if appendCanonicalLatestSection(&b, facts) {
		parts = append(parts, "canonical-latest-facts")
	}
	if appendNamedConstantsSection(&b, plan) {
		parts = append(parts, "named-constants")
	}

	// Citation map — extracted from spec-content-surfaces.md §
	// "Citation map". Hardcoded here as a constant block (the spec
	// doc itself isn't embedded; we inline the load-bearing rules).
	b.WriteString(citationMapBlock())
	parts = append(parts, "citation-map")

	return Brief{
		Kind:  BriefRefinement2,
		Body:  b.String(),
		Bytes: b.Len(),
		Parts: parts,
	}, nil
}

// citationMapBlock renders the topic → required-guide-citation map.
// Anchored to `docs/spec-content-surfaces.md §"Citation map"`; kept
// inline because the spec doc isn't embedded in the binary
// (//go:embed all:content covers internal/recipe/content/ only).
//
// When a topic listed below is amended in the spec, update this
// block to match. The test
// TestRefinement2Brief_CitationMapMatchesSpec keeps the two in sync.
func citationMapBlock() string {
	return `## Citation map — topics requiring zerops_knowledge citation

When a KB bullet covers one of these topics, the body MUST cite the
named guide. Missing citations get flagged with
` + "`missing-citation`" + ` (advisory).

- ` + "`rolling-deploys`" + ` / multi-container setups / minContainers≥2 / zero-downtime → cite ` + "`zero-downtime deploys with multi-container setups`" + ` (docs.zerops.io/features/scaling-ha)
- ` + "`init-commands`" + ` / ` + "`zsc execOnce`" + ` / ` + "`${appVersionId}`" + ` / per-deploy lock → cite ` + "`zsc execOnce + per-deploy key model`" + ` (docs.zerops.io/zerops-yaml/specification#initcommands-)
- object-storage / MinIO / S3 / forcePathStyle / presigned URL → cite ` + "`S3-compatible storage on the MinIO backend`" + ` (docs.zerops.io/services/object-storage)
- env-var-model / cross-service alias / same-key shadow / ` + "`${<host>_<key>}`" + ` → cite ` + "`per-key env shape and cross-service aliases`" + ` (docs.zerops.io/zerops-yaml/specification#envvariables-)
- subdomain access / ` + "`httpSupport`" + ` / L7 balancer / VXLAN routing → cite ` + "`Zerops L7 balancer + subdomain access`" + ` (docs.zerops.io/features/access)
- managed NATS / queue groups / pub-sub → cite ` + "`managed NATS broker`" + ` (docs.zerops.io/services/nats)
- managed Meilisearch / search keys / index admin → cite ` + "`managed Meilisearch service`" + ` (docs.zerops.io/services/meilisearch)

A bullet covering a topic NOT in this list has no required citation;
` + "`missing-citation`" + ` does not fire.

`
}
