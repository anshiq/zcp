// Tests for: ops/non_running_recovery.go — NonRunningRecovery helper.
// Discriminates between READY_TO_DEPLOY-with-failed-history (override),
// READY_TO_DEPLOY-clean (logs), FAILED (events), and intentional states
// (STOPPED/NEW → nil).
package ops

import (
	"context"
	"testing"

	"github.com/zeropsio/zcp/internal/platform"
)

func TestNonRunningRecovery_ReadyToDeployWithFailed_PointsAtImport(t *testing.T) {
	t.Parallel()
	client := platform.NewMock().
		WithServices([]platform.ServiceStack{{ID: "s1", Name: "api"}}).
		WithAppVersionEvents([]platform.AppVersionEvent{
			{ID: "av-1", ServiceStackID: "s1", Status: platform.BuildStatusBuildFailed, Created: "2026-05-05T10:00:00Z"},
		})

	rec := NonRunningRecovery(context.Background(), client, nil, "p-1", "api", platform.ServiceStatusReadyToDeploy)
	if rec == nil {
		t.Fatalf("expected Recovery for READY_TO_DEPLOY+failed history, got nil")
	}
	if rec.Tool != "zerops_import" {
		t.Errorf("Tool = %q, want %q", rec.Tool, "zerops_import")
	}
	if rec.Args["override"] != "true" {
		t.Errorf("Args[override] = %q, want %q", rec.Args["override"], "true")
	}
	if rec.Args["startWithoutCode"] != "true" {
		t.Errorf("Args[startWithoutCode] = %q, want %q", rec.Args["startWithoutCode"], "true")
	}
}

func TestNonRunningRecovery_ReadyToDeployNoFailedHistory_PointsAtLogs(t *testing.T) {
	t.Parallel()
	client := platform.NewMock().
		WithServices([]platform.ServiceStack{{ID: "s1", Name: "api"}}).
		WithAppVersionEvents(nil)

	rec := NonRunningRecovery(context.Background(), client, nil, "p-1", "api", platform.ServiceStatusReadyToDeploy)
	if rec == nil {
		t.Fatalf("expected Recovery for never-deployed READY_TO_DEPLOY, got nil")
	}
	if rec.Tool != "zerops_logs" {
		t.Errorf("Tool = %q, want %q", rec.Tool, "zerops_logs")
	}
	if rec.Args["serviceHostname"] != "api" {
		t.Errorf("Args[serviceHostname] = %q", rec.Args["serviceHostname"])
	}
	if rec.Args["facility"] != "application" {
		t.Errorf("Args[facility] = %q, want %q", rec.Args["facility"], "application")
	}
}

func TestNonRunningRecovery_FailedStatus_PointsAtEvents(t *testing.T) {
	t.Parallel()
	client := platform.NewMock().
		WithServices([]platform.ServiceStack{{ID: "s1", Name: "api"}})

	rec := NonRunningRecovery(context.Background(), client, nil, "p-1", "api", platform.ServiceStatusFailed)
	if rec == nil {
		t.Fatalf("expected Recovery for FAILED status, got nil")
	}
	if rec.Tool != "zerops_events" {
		t.Errorf("Tool = %q, want %q", rec.Tool, "zerops_events")
	}
	if rec.Args["serviceHostname"] != "api" {
		t.Errorf("Args[serviceHostname] = %q", rec.Args["serviceHostname"])
	}
}

func TestNonRunningRecovery_StoppedReturnsNil(t *testing.T) {
	t.Parallel()
	client := platform.NewMock().
		WithServices([]platform.ServiceStack{{ID: "s1", Name: "api"}})

	rec := NonRunningRecovery(context.Background(), client, nil, "p-1", "api", platform.ServiceStatusStopped)
	if rec != nil {
		t.Fatalf("STOPPED is intentional state — expected nil Recovery, got %+v", rec)
	}
}

func TestNonRunningRecovery_NewReturnsNil(t *testing.T) {
	t.Parallel()
	client := platform.NewMock().
		WithServices([]platform.ServiceStack{{ID: "s1", Name: "api"}})

	rec := NonRunningRecovery(context.Background(), client, nil, "p-1", "api", platform.ServiceStatusNew)
	if rec != nil {
		t.Fatalf("NEW is pre-deploy state — expected nil Recovery, got %+v", rec)
	}
}

func TestNonRunningRecovery_RunningReturnsNil(t *testing.T) {
	t.Parallel()
	client := platform.NewMock().
		WithServices([]platform.ServiceStack{{ID: "s1", Name: "api"}})

	rec := NonRunningRecovery(context.Background(), client, nil, "p-1", "api", platform.ServiceStatusRunning)
	if rec != nil {
		t.Fatalf("RUNNING is healthy — expected nil Recovery, got %+v", rec)
	}
}
