package tools

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	setupNameDev   = "dev"
	setupNameStage = "stage"
	setupNameProd  = "prod"
)

// deriveProdSetupBlock produces a proposed `setup: prod` yaml fragment
// derived from an existing source setup block. Item #6: replaces the
// generic `<placeholder>` atom template with concrete text the agent
// can apply (and tweak) instead of guessing the runtime / build
// commands / start command from scratch.
//
// Strategy:
//  1. Parse source yaml, find ordered list of setup blocks.
//  2. Prefer `setup: dev` as the source template; if absent, fall back
//     to the first non-prod setup; if only `setup: prod` exists (which
//     shouldn't happen because the caller already checked hasSetupProd),
//     return ErrNoSourceSetup.
//  3. Marshal a new block with `setup: prod` + the source's build/run
//     blocks verbatim.
//  4. Append a guidance comment about adding healthCheck (production
//     readiness rubric requires it).
//
// Returns the yaml fragment (with leading 2-space indent matching the
// list-entry shape) ready to be appended to the source `zerops:` list.
func deriveProdSetupBlock(sourceYAMLBody string) (string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(sourceYAMLBody), &doc); err != nil {
		return "", fmt.Errorf("derive prod setup: parse source yaml: %w", err)
	}
	setupsRaw, ok := doc["zerops"].([]any)
	if !ok || len(setupsRaw) == 0 {
		return "", fmt.Errorf("derive prod setup: source yaml has no top-level `zerops:` list")
	}

	template := pickProdSetupTemplate(setupsRaw)
	if template == nil {
		return "", fmt.Errorf("derive prod setup: no non-prod source setup block found to template from")
	}

	prod := map[string]any{
		"setup": setupNameProd,
	}
	if build, ok := template["build"]; ok {
		prod["build"] = build
	}
	if run, ok := template["run"]; ok {
		prod["run"] = run
	}

	out, err := yaml.Marshal([]any{prod})
	if err != nil {
		return "", fmt.Errorf("derive prod setup: marshal: %w", err)
	}

	rendered := string(out)
	// yaml.Marshal of []any emits "- setup: prod\n  build:..." with
	// list-marker dash. We want the fragment to be appendable under
	// the existing `zerops:` list, so indent the dash by 2 spaces to
	// match the list-entry continuation shape.
	indented := indentYAMLFragment(rendered, "  ")

	// Append healthCheck guidance comment — the production readiness
	// rubric requires run.healthCheck. Comments survive yaml.Marshal
	// only via Node API; simpler to append as a separate guidance
	// string the response surfaces alongside the block.
	return indented, nil
}

// pickProdSetupTemplate selects the source setup block to template
// from. Priority: `setup: dev` (canonical lifecycle baseline) →
// `setup: stage` → first non-prod block. Returns nil when nothing
// suitable found.
func pickProdSetupTemplate(setups []any) map[string]any {
	var dev, stage, firstNonProd map[string]any
	for _, item := range setups {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["setup"].(string)
		if name == setupNameProd {
			continue
		}
		if firstNonProd == nil {
			firstNonProd = m
		}
		if name == setupNameDev {
			dev = m
		}
		if name == setupNameStage {
			stage = m
		}
	}
	switch {
	case dev != nil:
		return dev
	case stage != nil:
		return stage
	default:
		return firstNonProd
	}
}

// indentYAMLFragment prepends prefix to every non-empty line of body.
// Used to align a yaml.Marshal-produced fragment under an existing
// indented context.
func indentYAMLFragment(body, prefix string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, len(lines))
	for i, line := range lines {
		if line == "" {
			out[i] = line
			continue
		}
		out[i] = prefix + line
	}
	return strings.Join(out, "\n")
}

// prodSetupGuidanceWithBlock composes the response guidance when the
// source-control blocker fires for missing setup:prod. Embeds the
// derived block + commit guidance + healthcheck reminder. The agent
// applies + tweaks the block instead of guessing.
func prodSetupGuidanceWithBlock(proposedBlock string) string {
	var b strings.Builder
	b.WriteString("Source zerops.yaml lacks a `setup: prod` block. Append the proposed block below to the top-level `zerops:` list, commit, push to remote, then re-call publish.\n\n")
	b.WriteString("Proposed block (derived from the source's existing dev/stage setup — review + tweak before committing):\n\n")
	b.WriteString("```yaml\n")
	b.WriteString(proposedBlock)
	if !strings.HasSuffix(proposedBlock, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n")
	b.WriteString("Production readiness checklist before commit:\n")
	b.WriteString("- Add `run.healthCheck` (httpGet path on the runtime port) — prod-readiness rubric requires it.\n")
	b.WriteString("- Verify build commands install only prod deps where possible (e.g. `npm ci --omit=dev`).\n")
	b.WriteString("- Verify start command uses production env (`NODE_ENV=production`, `APP_ENV=production`, etc.) — set via project envVariables or the run block.\n")
	return b.String()
}
