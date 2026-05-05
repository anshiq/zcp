// Tests for: import.go diagnose-before-destruct gate (plan v4 §3.2).
// Pinned by these tests:
//   - override=true on a service with failed appVersion history → first call
//     refused with ErrDiagnosisRequired + structured WouldDestroy
//   - override=true on a service with NO failed history → passes (gate
//     bypassed, agent's standard override warning applies)
//   - matching confirmDestructive on second call → proceeds to ImportServices
//   - partial / mismatched confirmDestructive → still refused
//   - non-override import never gates regardless of failed history
package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zeropsio/zcp/internal/platform"
)

func TestImport_OverrideOnFailedRequiresAck(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().
		WithServices([]platform.ServiceStack{
			{ID: "s1", Name: "api", Status: platform.ServiceStatusReadyToDeploy},
		}).
		WithAppVersionEvents([]platform.AppVersionEvent{
			{ID: "av-1", ServiceStackID: "s1", Status: platform.BuildStatusBuildFailed, Created: "2026-05-05T10:00:00Z"},
		})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	RegisterImport(srv, mock, "proj-1", testEngine(t), "", nil)

	yaml := "services:\n  - hostname: api\n    type: nodejs@22\n"
	result := callTool(t, srv, "zerops_import", map[string]any{
		"content":  yaml,
		"override": true,
	})
	if !result.IsError {
		t.Fatalf("expected IsError on override of failed service without ack")
	}

	var wire ErrorWire
	if err := json.Unmarshal([]byte(getTextContent(t, result)), &wire); err != nil {
		t.Fatalf("parse error wire: %v", err)
	}
	if wire.Code != platform.ErrDiagnosisRequired {
		t.Errorf("Code = %q, want %q", wire.Code, platform.ErrDiagnosisRequired)
	}
	if wire.WouldDestroy == nil {
		t.Fatalf("WouldDestroy missing on first-call refusal")
	}
	if wire.WouldDestroy.Operation != "import-override" {
		t.Errorf("Operation = %q", wire.WouldDestroy.Operation)
	}
	if len(wire.WouldDestroy.Targets) != 1 || wire.WouldDestroy.Targets[0] != "api" {
		t.Errorf("Targets = %v", wire.WouldDestroy.Targets)
	}
}

func TestImport_OverrideOnHealthyPasses(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().
		WithServices([]platform.ServiceStack{
			{ID: "s1", Name: "api", Status: platform.ServiceStatusActive},
		}).
		WithAppVersionEvents([]platform.AppVersionEvent{
			{ID: "av-1", ServiceStackID: "s1", Status: "ACTIVE", Created: "2026-05-05T10:00:00Z"},
		}).
		WithImportResult(&platform.ImportResult{
			ProjectID:   "proj-1",
			ProjectName: "myproject",
			ServiceStacks: []platform.ImportedServiceStack{
				{ID: "s1", Name: "api", Processes: []platform.Process{
					{ID: "p-1", ActionName: "serviceStackImport", Status: serviceStatusRunning},
				}},
			},
		}).
		WithProcess(&platform.Process{ID: "p-1", Status: statusFinished})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	RegisterImport(srv, mock, "proj-1", testEngine(t), "", nil)

	yaml := "services:\n  - hostname: api\n    type: nodejs@22\n"
	result := callTool(t, srv, "zerops_import", map[string]any{
		"content":  yaml,
		"override": true,
	})
	if result.IsError {
		t.Fatalf("override of healthy service should pass; got error: %s", getTextContent(t, result))
	}
}

func TestImport_AcknowledgedOverrideProceeds(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().
		WithServices([]platform.ServiceStack{
			{ID: "s1", Name: "api", Status: platform.ServiceStatusReadyToDeploy},
		}).
		WithAppVersionEvents([]platform.AppVersionEvent{
			{ID: "av-1", ServiceStackID: "s1", Status: platform.BuildStatusBuildFailed, Created: "2026-05-05T10:00:00Z"},
		}).
		WithImportResult(&platform.ImportResult{
			ProjectID:   "proj-1",
			ProjectName: "myproject",
			ServiceStacks: []platform.ImportedServiceStack{
				{ID: "s1", Name: "api", Processes: []platform.Process{
					{ID: "p-1", ActionName: "serviceStackImport", Status: serviceStatusRunning},
				}},
			},
		}).
		WithProcess(&platform.Process{ID: "p-1", Status: statusFinished})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	RegisterImport(srv, mock, "proj-1", testEngine(t), "", nil)

	yaml := "services:\n  - hostname: api\n    type: nodejs@22\n"
	result := callTool(t, srv, "zerops_import", map[string]any{
		"content":  yaml,
		"override": true,
		"confirmDestructive": map[string]any{
			"operation":           "import-override",
			"acknowledgedTargets": []string{"api"},
		},
	})
	if result.IsError {
		t.Fatalf("matching ack should proceed; got error: %s", getTextContent(t, result))
	}
}

func TestImport_PartialAckRejected(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().
		WithServices([]platform.ServiceStack{
			{ID: "s1", Name: "api", Status: platform.ServiceStatusReadyToDeploy},
			{ID: "s2", Name: "worker", Status: platform.ServiceStatusReadyToDeploy},
		}).
		WithAppVersionEvents([]platform.AppVersionEvent{
			{ID: "av-1", ServiceStackID: "s1", Status: platform.BuildStatusBuildFailed, Created: "2026-05-05T10:00:00Z"},
			{ID: "av-2", ServiceStackID: "s2", Status: platform.BuildStatusDeployFailed, Created: "2026-05-05T11:00:00Z"},
		})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	RegisterImport(srv, mock, "proj-1", testEngine(t), "", nil)

	yaml := "services:\n  - hostname: api\n    type: nodejs@22\n  - hostname: worker\n    type: nodejs@22\n"
	result := callTool(t, srv, "zerops_import", map[string]any{
		"content":  yaml,
		"override": true,
		"confirmDestructive": map[string]any{
			"operation":           "import-override",
			"acknowledgedTargets": []string{"api"}, // missing "worker"
		},
	})
	if !result.IsError {
		t.Fatalf("partial ack must be rejected; got success")
	}
	var wire ErrorWire
	if err := json.Unmarshal([]byte(getTextContent(t, result)), &wire); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if wire.Code != platform.ErrDiagnosisRequired {
		t.Errorf("Code = %q, want %q", wire.Code, platform.ErrDiagnosisRequired)
	}
}

func TestImport_AckOperationMismatchRejected(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().
		WithServices([]platform.ServiceStack{
			{ID: "s1", Name: "api", Status: platform.ServiceStatusReadyToDeploy},
		}).
		WithAppVersionEvents([]platform.AppVersionEvent{
			{ID: "av-1", ServiceStackID: "s1", Status: platform.BuildStatusBuildFailed, Created: "2026-05-05T10:00:00Z"},
		})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	RegisterImport(srv, mock, "proj-1", testEngine(t), "", nil)

	yaml := "services:\n  - hostname: api\n    type: nodejs@22\n"
	result := callTool(t, srv, "zerops_import", map[string]any{
		"content":  yaml,
		"override": true,
		"confirmDestructive": map[string]any{
			"operation":           "env-set", // wrong operation
			"acknowledgedTargets": []string{"api"},
		},
	})
	if !result.IsError {
		t.Fatalf("operation mismatch must be rejected")
	}
	if !strings.Contains(getTextContent(t, result), "operation") {
		t.Errorf("error text should mention operation mismatch: %s", getTextContent(t, result))
	}
}

func TestImport_NonOverrideImportNotGated(t *testing.T) {
	t.Parallel()
	// New service, no override — failed history on a same-named service in
	// the project should NOT block a new import; failed history only matters
	// when override=true would replace the failed service.
	mock := platform.NewMock().
		WithImportResult(&platform.ImportResult{
			ProjectID:   "proj-1",
			ProjectName: "myproject",
			ServiceStacks: []platform.ImportedServiceStack{
				{ID: "s2", Name: "newsvc", Processes: []platform.Process{
					{ID: "p-1", ActionName: "serviceStackImport", Status: serviceStatusRunning},
				}},
			},
		}).
		WithProcess(&platform.Process{ID: "p-1", Status: statusFinished})

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1"}, nil)
	RegisterImport(srv, mock, "proj-1", testEngine(t), "", nil)

	yaml := "services:\n  - hostname: newsvc\n    type: nodejs@22\n"
	result := callTool(t, srv, "zerops_import", map[string]any{
		"content": yaml,
		// no override
	})
	if result.IsError {
		t.Errorf("non-override import should pass: %s", getTextContent(t, result))
	}
}
