package tools

import (
	"strings"
	"testing"
)

const sourceWithDevOnly = `zerops:
  - setup: dev
    build:
      base: nodejs@22
      buildCommands:
        - npm install
    run:
      base: nodejs@22
      start: node dist/server.js
`

const sourceWithDevAndStage = `zerops:
  - setup: dev
    build:
      base: nodejs@22
      buildCommands:
        - npm install
    run:
      base: nodejs@22
      start: node dist/server.js
  - setup: stage
    build:
      base: nodejs@22
      buildCommands:
        - npm ci
    run:
      base: nodejs@22
      start: node dist/server.js
`

const sourceWithSimpleOnly = `zerops:
  - setup: simple
    build:
      base: bun@1
      buildCommands:
        - bun install
    run:
      base: bun@1
      start: bun run index.ts
`

const sourceWithOnlyProd = `zerops:
  - setup: prod
    build:
      base: nodejs@22
`

// TestDeriveProdSetupBlock_PreferDev verifies dev wins over other names
// as the template source.
func TestDeriveProdSetupBlock_PreferDev(t *testing.T) {
	t.Parallel()
	got, err := deriveProdSetupBlock(sourceWithDevAndStage)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if !strings.Contains(got, "setup: prod") {
		t.Errorf("output missing setup: prod\n%s", got)
	}
	// dev's buildCommand was `npm install`; stage was `npm ci`.
	// Preferring dev means the proposed block carries `npm install`.
	if !strings.Contains(got, "npm install") {
		t.Errorf("expected dev's npm install (template prefer dev), got:\n%s", got)
	}
	if strings.Contains(got, "npm ci") {
		t.Errorf("expected dev template, not stage:\n%s", got)
	}
}

// TestDeriveProdSetupBlock_FallbackToFirstNonProd uses simple-mode
// source — dev/stage absent — falls back to first non-prod block.
func TestDeriveProdSetupBlock_FallbackToFirstNonProd(t *testing.T) {
	t.Parallel()
	got, err := deriveProdSetupBlock(sourceWithSimpleOnly)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if !strings.Contains(got, "setup: prod") {
		t.Errorf("output missing setup: prod\n%s", got)
	}
	if !strings.Contains(got, "bun@1") {
		t.Errorf("expected bun@1 base copied verbatim:\n%s", got)
	}
}

// TestDeriveProdSetupBlock_DevOnlyCopiesVerbatim verifies the
// derivation copies build + run blocks unchanged.
func TestDeriveProdSetupBlock_DevOnlyCopiesVerbatim(t *testing.T) {
	t.Parallel()
	got, err := deriveProdSetupBlock(sourceWithDevOnly)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	for _, want := range []string{
		"setup: prod",
		"base: nodejs@22",
		"npm install",
		"node dist/server.js",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output:\n%s", want, got)
		}
	}
}

// TestDeriveProdSetupBlock_OnlyProdReturnsError pins the edge case
// where source has only setup:prod (defensive — caller checks
// hasSetupProd first so this shouldn't fire in practice).
func TestDeriveProdSetupBlock_OnlyProdReturnsError(t *testing.T) {
	t.Parallel()
	_, err := deriveProdSetupBlock(sourceWithOnlyProd)
	if err == nil {
		t.Fatal("expected error when only setup:prod is available as template")
	}
}

// TestDeriveProdSetupBlock_MalformedYAMLReturnsError pins yaml parse
// failure handling.
func TestDeriveProdSetupBlock_MalformedYAMLReturnsError(t *testing.T) {
	t.Parallel()
	_, err := deriveProdSetupBlock("zerops:\n  - setup: [unclosed")
	if err == nil {
		t.Fatal("expected error for malformed yaml")
	}
}

// TestProdSetupGuidanceWithBlock_EmbedsBlockAndChecklist verifies the
// response guidance carries the derived block + readiness checklist.
func TestProdSetupGuidanceWithBlock_EmbedsBlockAndChecklist(t *testing.T) {
	t.Parallel()
	proposed, err := deriveProdSetupBlock(sourceWithDevOnly)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	guidance := prodSetupGuidanceWithBlock(proposed)
	for _, want := range []string{
		"setup: prod",
		"npm install",
		"healthCheck",
		"NODE_ENV=production",
		"```yaml",
	} {
		if !strings.Contains(guidance, want) {
			t.Errorf("guidance missing %q:\n%s", want, guidance)
		}
	}
}
