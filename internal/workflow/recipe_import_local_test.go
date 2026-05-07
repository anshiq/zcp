package workflow

import (
	"strings"
	"testing"
)

// TestLocalizeRecipeImportYAML_DropsZeropsSetupDev pins that services
// declaring `zeropsSetup: dev` are removed entirely. Local mode
// replaces the SSH-in dev runtime with the user's CWD.
func TestLocalizeRecipeImportYAML_DropsZeropsSetupDev(t *testing.T) {
	t.Parallel()
	const input = `project:
  name: nodejs-hello-world-agent
services:
  - hostname: appdev
    type: nodejs@22
    zeropsSetup: dev
    buildFromGit: https://github.com/zerops-recipe-apps/nodejs-hello-world-app
  - hostname: appstage
    type: nodejs@22
    zeropsSetup: prod
    buildFromGit: https://github.com/zerops-recipe-apps/nodejs-hello-world-app
  - hostname: db
    type: postgresql@18
    mode: NON_HA
`
	got, err := LocalizeRecipeImportYAML(input)
	if err != nil {
		t.Fatalf("LocalizeRecipeImportYAML: %v", err)
	}
	if strings.Contains(got, "appdev") {
		t.Errorf("appdev should be dropped; got:\n%s", got)
	}
	if !strings.Contains(got, "appstage") {
		t.Errorf("appstage should remain; got:\n%s", got)
	}
	if !strings.Contains(got, "hostname: db") {
		t.Errorf("managed db should remain; got:\n%s", got)
	}
}

// TestLocalizeRecipeImportYAML_PreservesBuildFromGit pins that
// remaining runtime services KEEP buildFromGit. Zerops API rejects
// a runtime declaring zeropsSetup without buildFromGit
// ("pipelineConfig requires source URL"). Stage auto-seeds from
// upstream on import; first local deploy is a normal iteration,
// not part of bootstrap. Surfaced by flow-eval-local
// recipe-nodejs-hello-world suite 20260507-125401.
func TestLocalizeRecipeImportYAML_PreservesBuildFromGit(t *testing.T) {
	t.Parallel()
	const input = `services:
  - hostname: appstage
    type: nodejs@22
    zeropsSetup: prod
    buildFromGit: https://github.com/zerops-recipe-apps/nodejs-hello-world-app
    enableSubdomainAccess: true
  - hostname: db
    type: postgresql@18
`
	got, err := LocalizeRecipeImportYAML(input)
	if err != nil {
		t.Fatalf("LocalizeRecipeImportYAML: %v", err)
	}
	if !strings.Contains(got, "buildFromGit:") {
		t.Errorf("buildFromGit MUST remain (Zerops API requires it with zeropsSetup); got:\n%s", got)
	}
	if !strings.Contains(got, "zeropsSetup: prod") {
		t.Errorf("zeropsSetup: prod should remain; got:\n%s", got)
	}
	if !strings.Contains(got, "enableSubdomainAccess: true") {
		t.Errorf("enableSubdomainAccess should remain; got:\n%s", got)
	}
}

// TestLocalizeRecipeImportYAML_PreservesManagedServices pins that
// managed services (no zeropsSetup) are untouched.
func TestLocalizeRecipeImportYAML_PreservesManagedServices(t *testing.T) {
	t.Parallel()
	const input = `services:
  - hostname: appstage
    type: nodejs@22
    zeropsSetup: prod
    buildFromGit: https://example.com/repo
  - hostname: db
    type: postgresql@18
    mode: NON_HA
    priority: 10
`
	got, err := LocalizeRecipeImportYAML(input)
	if err != nil {
		t.Fatalf("LocalizeRecipeImportYAML: %v", err)
	}
	if !strings.Contains(got, "type: postgresql@18") {
		t.Errorf("managed db type should remain; got:\n%s", got)
	}
	if !strings.Contains(got, "mode: NON_HA") {
		t.Errorf("managed db mode should remain; got:\n%s", got)
	}
	if !strings.Contains(got, "priority: 10") {
		t.Errorf("managed db priority should remain; got:\n%s", got)
	}
}

// TestLocalizeRecipeImportYAML_NoOpForRecipesAlreadyLocalShape pins
// that recipes shaped for local mode (single runtime, no dev block —
// e.g. nextjs-ssr-hello-world) pass through with buildFromGit
// stripped from the single runtime. Idempotency: running twice
// yields the same result as running once.
func TestLocalizeRecipeImportYAML_NoOpForRecipesAlreadyLocalShape(t *testing.T) {
	t.Parallel()
	const input = `services:
  - hostname: app
    type: nodejs@22
    zeropsSetup: prod
    buildFromGit: https://example.com/repo
  - hostname: db
    type: postgresql@18
`
	first, err := LocalizeRecipeImportYAML(input)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := LocalizeRecipeImportYAML(first)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Errorf("transform not idempotent:\nfirst:\n%s\n\nsecond:\n%s", first, second)
	}
}

// TestLocalizeRecipeImportYAML_PreservesYAMLComments pins that
// comments in the recipe YAML survive the transform. Recipe authors
// embed comments documenting service roles; agents read them.
func TestLocalizeRecipeImportYAML_PreservesYAMLComments(t *testing.T) {
	t.Parallel()
	const input = `# Top-level comment about the recipe
services:
  # comment for app service
  - hostname: app
    type: nodejs@22
    zeropsSetup: prod
    buildFromGit: https://example.com/repo
  # comment for db
  - hostname: db
    type: postgresql@18
`
	got, err := LocalizeRecipeImportYAML(input)
	if err != nil {
		t.Fatalf("LocalizeRecipeImportYAML: %v", err)
	}
	for _, want := range []string{
		"# Top-level comment",
		"# comment for app",
		"# comment for db",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("comment %q dropped; got:\n%s", want, got)
		}
	}
}

// TestLocalizeRecipeImportYAML_EmptyInput pins that empty input
// returns empty output without error (defensive — callers may pass
// empty strings during early bootstrap).
func TestLocalizeRecipeImportYAML_EmptyInput(t *testing.T) {
	t.Parallel()
	got, err := LocalizeRecipeImportYAML("")
	if err != nil {
		t.Fatalf("empty input should not error: %v", err)
	}
	if got != "" {
		t.Errorf("empty input → empty output; got %q", got)
	}
}

// TestLocalizeRecipeImportYAML_DropAllRuntimes pins the unusual case
// of every runtime being zeropsSetup: dev — output keeps managed
// services only. Agent must error out somewhere upstream because the
// resulting topology has no runtime to deploy to, but this transform
// is shape-faithful.
func TestLocalizeRecipeImportYAML_DropAllRuntimes(t *testing.T) {
	t.Parallel()
	const input = `services:
  - hostname: appdev
    type: nodejs@22
    zeropsSetup: dev
    buildFromGit: https://example.com/repo
  - hostname: db
    type: postgresql@18
`
	got, err := LocalizeRecipeImportYAML(input)
	if err != nil {
		t.Fatalf("LocalizeRecipeImportYAML: %v", err)
	}
	if strings.Contains(got, "appdev") {
		t.Errorf("appdev should be dropped; got:\n%s", got)
	}
	if !strings.Contains(got, "hostname: db") {
		t.Errorf("managed db should remain; got:\n%s", got)
	}
}
