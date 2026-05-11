package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run-40 ENG-5 — gateConsumesServicesDerivable refuses feature
// complete-phase when a codebase declares a managed-service
// dependency the on-disk yaml doesn't reference. Pinned against
// run-39's S1-4 failure mode: `worker.ConsumesServices=["broker","db"]`
// with worker yaml lacking `${db_*}` references after feature rewrote
// the consumer code.

// TestGateConsumesServicesDerivable_DeclaredButNotReferenced fires a
// blocking violation when a declared host is absent from the on-disk
// yaml.
func TestGateConsumesServicesDerivable_DeclaredButNotReferenced(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	yaml := `zerops:
  - setup: worker
    run:
      base: nodejs@22
      envVariables:
        NATS_HOST: ${broker_hostname}
        NATS_PORT: ${broker_port}
`
	workerDir := filepath.Join(dir, "workerdev")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "zerops.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	plan := &Plan{
		Slug: "synth-showcase",
		Codebases: []Codebase{
			{
				Hostname:         "worker",
				SourceRoot:       workerDir,
				ConsumesServices: []string{"broker", "db"}, // db is the ghost
			},
		},
		Services: []Service{
			{Hostname: "broker", Kind: ServiceKindManaged, Type: "nats@2.10"},
			{Hostname: "db", Kind: ServiceKindManaged, Type: "postgresql@18"},
		},
	}

	violations := gateConsumesServicesDerivable(GateContext{Plan: plan})
	if len(violations) != 1 {
		t.Fatalf("expected one ghost-dependency violation; got %d (%+v)", len(violations), violations)
	}
	v := violations[0]
	if v.Code != "consumes-services-ghost-dependency" {
		t.Errorf("violation code = %q, want %q", v.Code, "consumes-services-ghost-dependency")
	}
	if v.Severity != SeverityBlocking {
		t.Errorf("violation severity = %v, want Blocking", v.Severity)
	}
	if !strings.Contains(v.Path, "codebase/worker/consumesServices") {
		t.Errorf("violation path should name the codebase consumer slot; got %q", v.Path)
	}
	if !strings.Contains(v.Message, `"db"`) {
		t.Errorf("violation message should name the ghost host `db`; got %q", v.Message)
	}
}

// TestGateConsumesServicesDerivable_AllDeclaredHostsPresent — happy
// path: declared = derived, no violations.
func TestGateConsumesServicesDerivable_AllDeclaredHostsPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerDir := filepath.Join(dir, "workerdev")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := `zerops:
  - setup: worker
    run:
      envVariables:
        NATS_HOST: ${broker_hostname}
        DB_HOST: ${db_hostname}
`
	if err := os.WriteFile(filepath.Join(workerDir, "zerops.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	plan := &Plan{
		Slug: "synth-showcase",
		Codebases: []Codebase{
			{
				Hostname:         "worker",
				SourceRoot:       workerDir,
				ConsumesServices: []string{"broker", "db"},
			},
		},
		Services: []Service{
			{Hostname: "broker", Kind: ServiceKindManaged, Type: "nats@2.10"},
			{Hostname: "db", Kind: ServiceKindManaged, Type: "postgresql@18"},
		},
	}

	violations := gateConsumesServicesDerivable(GateContext{Plan: plan})
	if len(violations) != 0 {
		t.Errorf("expected no violations when declared == derived; got %d: %+v", len(violations), violations)
	}
}

// TestGateConsumesServicesDerivable_MultipleGhosts_AllSurface — when
// a codebase has more than one ghost, all surface in one round-trip
// (one violation per ghost). Catches the agent's tendency to fix one
// thing per cycle when the engine only surfaced one at a time.
func TestGateConsumesServicesDerivable_MultipleGhosts_AllSurface(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	workerDir := filepath.Join(dir, "workerdev")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := `zerops:
  - setup: worker
    run:
      envVariables:
        NATS_HOST: ${broker_hostname}
`
	if err := os.WriteFile(filepath.Join(workerDir, "zerops.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	plan := &Plan{
		Slug: "synth-showcase",
		Codebases: []Codebase{
			{
				Hostname:         "worker",
				SourceRoot:       workerDir,
				ConsumesServices: []string{"broker", "cache", "db", "search"}, // 3 ghosts
			},
		},
		Services: []Service{
			{Hostname: "broker", Kind: ServiceKindManaged, Type: "nats@2.10"},
			{Hostname: "cache", Kind: ServiceKindManaged, Type: "valkey@8"},
			{Hostname: "db", Kind: ServiceKindManaged, Type: "postgresql@18"},
			{Hostname: "search", Kind: ServiceKindManaged, Type: "meilisearch@1"},
		},
	}

	violations := gateConsumesServicesDerivable(GateContext{Plan: plan})
	if len(violations) != 3 {
		t.Fatalf("expected 3 ghost violations (cache, db, search); got %d: %+v", len(violations), violations)
	}
	seen := map[string]bool{}
	for _, v := range violations {
		for _, host := range []string{"cache", "db", "search"} {
			if strings.Contains(v.Message, `"`+host+`"`) {
				seen[host] = true
			}
		}
	}
	for _, host := range []string{"cache", "db", "search"} {
		if !seen[host] {
			t.Errorf("missing ghost violation for host %q", host)
		}
	}
}

// TestGateConsumesServicesDerivable_NilConsumesServicesSkipped pins
// the back-compat skip: codebases with nil ConsumesServices haven't
// been analyzed yet (sim path, pre-scaffold-close); the gate skips
// them so it doesn't ship a violation before the populate path has
// had a chance to run.
func TestGateConsumesServicesDerivable_NilConsumesServicesSkipped(t *testing.T) {
	t.Parallel()
	plan := &Plan{
		Slug: "synth-showcase",
		Codebases: []Codebase{
			{Hostname: "worker", SourceRoot: "/var/www/workerdev", ConsumesServices: nil},
		},
		Services: []Service{
			{Hostname: "db", Kind: ServiceKindManaged, Type: "postgresql@18"},
		},
	}
	violations := gateConsumesServicesDerivable(GateContext{Plan: plan})
	if len(violations) != 0 {
		t.Errorf("nil ConsumesServices should skip the gate; got %+v", violations)
	}
}

// TestGateConsumesServicesDerivable_AbsentYamlSkipped pins the
// conservative-skip behavior: when the codebase yaml isn't on disk
// (sim/pre-stitch shape) the gate skips rather than blocking on a
// missing read. populateConsumesServicesFromYaml inherits the same
// conservative contract; the gate matches.
func TestGateConsumesServicesDerivable_AbsentYamlSkipped(t *testing.T) {
	t.Parallel()
	plan := &Plan{
		Slug: "synth-showcase",
		Codebases: []Codebase{
			{Hostname: "worker", SourceRoot: "/var/www/definitely-not-here", ConsumesServices: []string{"db"}},
		},
		Services: []Service{
			{Hostname: "db", Kind: ServiceKindManaged, Type: "postgresql@18"},
		},
	}
	violations := gateConsumesServicesDerivable(GateContext{Plan: plan})
	if len(violations) != 0 {
		t.Errorf("absent yaml should skip the gate (conservative); got %+v", violations)
	}
}

// TestFeatureGates_RegistersConsumesServicesDerivable pins the gate
// wiring: feature complete-phase MUST include the ghost-dependency
// check.
func TestFeatureGates_RegistersConsumesServicesDerivable(t *testing.T) {
	t.Parallel()
	gates := FeatureGates()
	mustHaveGate(t, gates, "consumes-services-derivable")
}
