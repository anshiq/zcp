package recipe

import (
	"strings"
	"testing"
)

// Run-29 Fix #4 — refinement-suspects extension for IG ↔ yaml-comment
// same-mechanism duplication (Criterion 6 defense-in-depth). The
// authoring-order teaching in synthesis_workflow.md is the primary
// fix; this predicate flags suspects when both surfaces teach the
// same canonical mechanism with overlapping prose context.

// TestCollectRefinementSuspects_IGYAMLCommentDup_Detected — fixture
// with same-mechanism teaching on IG #5 + zerops.yaml comment for the
// same codebase → suspect emitted with code criterion-6-ig-yamlcomment-dup.
func TestCollectRefinementSuspects_IGYAMLCommentDup_Detected(t *testing.T) {
	t.Parallel()
	plan := syntheticShowcasePlan()
	plan.Fragments = map[string]string{
		"codebase/api/integration-guide": "" +
			"### 1. Adding `zerops.yaml`\n\n" +
			"```yaml\nzerops:\n  - setup: api\n```\n\n" +
			"### 5. Same-key shadow trap\n\n" +
			"Pick own-key names that are DIFFERENT from the platform-side keys. " +
			"Declaring `db_hostname: ${db_hostname}` self-shadows: the per-service " +
			"envVariables write runs after the auto-inject so the literal " +
			"${db_hostname} string lands on the OS env.\n",
		"codebase/api/zerops-yaml": "" +
			"zerops:\n" +
			"  - setup: api\n" +
			"    envVariables:\n" +
			"      # Same-key aliasing self-shadows: declaring `db_hostname: ${db_hostname}`\n" +
			"      # writes the literal ${db_hostname} string to the OS env because the\n" +
			"      # per-service envVariables write runs after the auto-inject.\n" +
			"      DB_HOST: ${db_hostname}\n",
	}
	suspects := CollectRefinementSuspects(plan, nil)
	found := false
	for _, s := range suspects {
		if s.Class == "criterion-6-ig-yamlcomment-dup" && s.FragmentID == "codebase/api/integration-guide" {
			found = true
			if !strings.Contains(s.Reason, "${db_hostname}") {
				t.Errorf("expected suspect Reason to name the anchor; got %q", s.Reason)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected criterion-6-ig-yamlcomment-dup suspect on codebase/api; got %+v", suspects)
	}
}

// TestCollectRefinementSuspects_IGYAMLCommentDifferent_NoSuspect — IG
// teaches mechanism A, yaml teaches mechanism B → no suspect.
func TestCollectRefinementSuspects_IGYAMLCommentDifferent_NoSuspect(t *testing.T) {
	t.Parallel()
	plan := syntheticShowcasePlan()
	plan.Fragments = map[string]string{
		"codebase/api/integration-guide": "" +
			"### 1. Adding `zerops.yaml`\n\n" +
			"```yaml\nzerops:\n  - setup: api\n```\n\n" +
			"### 2. Bind to 0.0.0.0\n\nThe porter's app must bind to 0.0.0.0.\n",
		"codebase/api/zerops-yaml": "" +
			"zerops:\n" +
			"  - setup: api\n" +
			"    envVariables:\n" +
			"      # forcePathStyle is required for object-storage compatibility\n" +
			"      # in the s3 client; the platform's S3-compatible API requires it.\n" +
			"      AWS_S3_FORCE_PATH_STYLE: true\n",
	}
	suspects := CollectRefinementSuspects(plan, nil)
	for _, s := range suspects {
		if s.Class == "criterion-6-ig-yamlcomment-dup" {
			t.Errorf("expected no criterion-6-ig-yamlcomment-dup suspect when IG and yaml teach different mechanisms; got %+v", s)
		}
	}
}

// TestCollectRefinementSuspects_IGEngineYamlBlockOnly_NoSuspect — IG #1
// engine-stamped yaml legitimately contains anchor tokens (e.g. yaml
// references the same managed service variables); the predicate must
// strip the first ```yaml block before scanning so engine-emit alone
// never trips the suspect.
func TestCollectRefinementSuspects_IGEngineYamlBlockOnly_NoSuspect(t *testing.T) {
	t.Parallel()
	plan := syntheticShowcasePlan()
	plan.Fragments = map[string]string{
		"codebase/api/integration-guide": "" +
			"### 1. Adding `zerops.yaml`\n\n" +
			"```yaml\n" +
			"zerops:\n" +
			"  - setup: api\n" +
			"    envVariables:\n" +
			"      DB_HOST: ${db_hostname}\n" +
			"```\n\n" +
			"### 2. Bind to 0.0.0.0\n\nNothing here mentions the env tokens.\n",
		"codebase/api/zerops-yaml": "" +
			"zerops:\n" +
			"  - setup: api\n" +
			"    envVariables:\n" +
			"      # `DB_HOST` is the own-key alias.\n" +
			"      DB_HOST: ${db_hostname}\n",
	}
	suspects := CollectRefinementSuspects(plan, nil)
	for _, s := range suspects {
		if s.Class == "criterion-6-ig-yamlcomment-dup" {
			t.Errorf("expected no criterion-6-ig-yamlcomment-dup suspect when IG anchor only appears inside engine-stamped yaml block; got %+v", s)
		}
	}
}
