// Tests for: tools/destructive_ack.go — shared diagnose-before-destruct
// shape (plan v4 §3.1). Pinned by these tests:
//   - DiagnosedDestruction round-trips through ErrorWire JSON
//   - ErrDiagnosisRequired is the canonical error code on first-call refusal
//   - ValidateDestructiveAck accepts matching ops/targets/failure-class
//   - ValidateDestructiveAck rejects on operation, target-set, or class drift
package tools

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/zeropsio/zcp/internal/platform"
)

func TestErrDiagnosisRequired_WireForm(t *testing.T) {
	t.Parallel()
	pe := platform.NewPlatformError(
		platform.ErrDiagnosisRequired,
		"override would destroy 1 service with failed history",
		"Read zerops_logs and re-call with confirmDestructive matching wouldDestroy.",
	)
	wire := platformErrorToWire(pe)
	if wire.Code != "DIAGNOSIS_REQUIRED" {
		t.Errorf("Code = %q, want %q", wire.Code, "DIAGNOSIS_REQUIRED")
	}

	out, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ErrorWire
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if back.Code != "DIAGNOSIS_REQUIRED" {
		t.Errorf("round-trip Code = %q", back.Code)
	}
}

func TestDestructiveAck_ShapeRoundTrip(t *testing.T) {
	t.Parallel()
	wire := ErrorWire{
		Code:  platform.ErrDiagnosisRequired,
		Error: "override would destroy 1 service with failed history",
		WouldDestroy: &DiagnosedDestruction{
			Operation: "import-override",
			Targets:   []string{"appdev"},
			Loss: DestructionLoss{
				ServiceStacks: []string{"appdev"},
				EnvVars:       []string{"DATABASE_URL", "CACHE_URL"},
			},
		},
	}

	out, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ErrorWire
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.WouldDestroy == nil {
		t.Fatalf("WouldDestroy dropped")
	}
	if back.WouldDestroy.Operation != "import-override" {
		t.Errorf("Operation = %q", back.WouldDestroy.Operation)
	}
	if len(back.WouldDestroy.Targets) != 1 || back.WouldDestroy.Targets[0] != "appdev" {
		t.Errorf("Targets = %v", back.WouldDestroy.Targets)
	}
	if len(back.WouldDestroy.Loss.EnvVars) != 2 {
		t.Errorf("Loss.EnvVars = %v", back.WouldDestroy.Loss.EnvVars)
	}
}

func TestValidateDestructiveAck_OperationMismatch(t *testing.T) {
	t.Parallel()
	expected := DiagnosedDestruction{
		Operation: "import-override",
		Targets:   []string{"appdev"},
	}
	ack := &DestructiveAck{
		Operation:           "env-set", // wrong operation
		AcknowledgedTargets: []string{"appdev"},
	}
	err := ValidateDestructiveAck(ack, expected)
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	var pe *platform.PlatformError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PlatformError, got %T: %v", err, err)
	}
	if pe.Code != platform.ErrDiagnosisRequired {
		t.Errorf("Code = %q, want %q", pe.Code, platform.ErrDiagnosisRequired)
	}
}

func TestValidateDestructiveAck_TargetSetMismatch(t *testing.T) {
	t.Parallel()
	expected := DiagnosedDestruction{
		Operation: "import-override",
		Targets:   []string{"appdev", "appstage"},
	}

	cases := []struct {
		name string
		ack  *DestructiveAck
	}{
		{name: "subset", ack: &DestructiveAck{Operation: "import-override", AcknowledgedTargets: []string{"appdev"}}},
		{name: "superset", ack: &DestructiveAck{Operation: "import-override", AcknowledgedTargets: []string{"appdev", "appstage", "appqa"}}},
		{name: "different-name", ack: &DestructiveAck{Operation: "import-override", AcknowledgedTargets: []string{"appdev", "wrongname"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateDestructiveAck(tc.ack, expected); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestValidateDestructiveAck_TargetSetMatchOrderInsensitive(t *testing.T) {
	t.Parallel()
	expected := DiagnosedDestruction{
		Operation: "import-override",
		Targets:   []string{"appdev", "appstage"},
	}
	ack := &DestructiveAck{
		Operation:           "import-override",
		AcknowledgedTargets: []string{"appstage", "appdev"}, // reversed order
	}
	if err := ValidateDestructiveAck(ack, expected); err != nil {
		t.Errorf("expected match (order-insensitive), got error: %v", err)
	}
}

func TestValidateDestructiveAck_NilRejected(t *testing.T) {
	t.Parallel()
	expected := DiagnosedDestruction{
		Operation: "import-override",
		Targets:   []string{"appdev"},
	}
	if err := ValidateDestructiveAck(nil, expected); err == nil {
		t.Fatal("nil ack must be rejected")
	}
}
