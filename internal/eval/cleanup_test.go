package eval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zeropsio/zcp/internal/platform"
)

// withTuneables shrinks cleanup-loop timing knobs for unit tests and
// restores them on cleanup. Production defaults stay untouched.
func withTuneables(t *testing.T, settle, interval time.Duration, maxRetries int) {
	t.Helper()
	prevSettle, prevInterval, prevRetries, prevProcInterval :=
		CleanupSettleTimeout, CleanupVerifyInterval, CleanupVerifyMaxRetries, CleanupProcessPollInterval
	CleanupSettleTimeout = settle
	CleanupVerifyInterval = interval
	CleanupVerifyMaxRetries = maxRetries
	CleanupProcessPollInterval = interval
	t.Cleanup(func() {
		CleanupSettleTimeout = prevSettle
		CleanupVerifyInterval = prevInterval
		CleanupVerifyMaxRetries = prevRetries
		CleanupProcessPollInterval = prevProcInterval
	})
}

func TestDeleteServices_ServiceAlreadyGone_Succeeds(t *testing.T) {
	t.Parallel()

	mock := platform.NewMock().WithError("DeleteService", platform.NewPlatformError(
		platform.ErrServiceNotFound,
		"Service stack not found",
		"",
	))
	services := []platform.ServiceStack{{ID: "svc-app", Name: "app"}}

	if err := deleteServices(context.Background(), mock, services); err != nil {
		t.Fatalf("deleteServices: %v", err)
	}
	if got := mock.CallCounts["DeleteService"]; got != 1 {
		t.Fatalf("DeleteService calls: got %d, want 1", got)
	}
}

func TestDeleteServices_DeleteError_Fails(t *testing.T) {
	t.Parallel()

	mock := platform.NewMock().WithError("DeleteService", errors.New("api unavailable"))
	services := []platform.ServiceStack{{ID: "svc-app", Name: "app"}}

	err := deleteServices(context.Background(), mock, services)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), `delete "app": api unavailable`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mock.CallCounts["DeleteService"]; got != 1 {
		t.Fatalf("DeleteService calls: got %d, want 1", got)
	}
}

// Regression for flow-eval suite 20260503-173119: cleanup aborted on
// "Service stack not found" because the API returned the error via APICode
// rather than HTTP 404 → ErrServiceNotFound. Cleanup must tolerate ALL
// "not found"-class errors so a service that vanished between list and
// delete (concurrent scenarios, retries) doesn't kill the rest of the run.

func TestDeleteServices_NotFoundViaAPICode_Skips(t *testing.T) {
	t.Parallel()
	pe := platform.NewPlatformError(platform.ErrAPIError, "Service stack not found.", "")
	pe.APICode = "serviceStackNotFound"
	mock := platform.NewMock().WithError("DeleteService", pe)
	services := []platform.ServiceStack{{ID: "svc-appdev", Name: "appdev"}}

	if err := deleteServices(context.Background(), mock, services); err != nil {
		t.Fatalf("APICode=serviceStackNotFound must be tolerated: %v", err)
	}
}

func TestDeleteServices_NotFoundViaMessage_Skips(t *testing.T) {
	t.Parallel()
	// Some platform paths leave APICode empty and only put "not found" in the
	// message. The message-substring fallback covers that case.
	pe := platform.NewPlatformError(platform.ErrAPIError, "Service stack not found.", "")
	mock := platform.NewMock().WithError("DeleteService", pe)
	services := []platform.ServiceStack{{ID: "svc-appdev", Name: "appdev"}}

	if err := deleteServices(context.Background(), mock, services); err != nil {
		t.Fatalf("message-only 'not found' must be tolerated: %v", err)
	}
}

// TestVerifyProjectEmpty_NamesResidual pins the residual-error shape: the
// caller (next scenario) needs to read which service is still alive and in
// what status, so it can decide whether to bail or wait.
func TestVerifyProjectEmpty_NamesResidual(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().WithServices([]platform.ServiceStack{
		{ID: "svc-zcp", Name: "zcp", Status: "ACTIVE"},
		{ID: "svc-app", Name: "appdev", Status: "CREATING"},
	})
	err := verifyProjectEmpty(context.Background(), mock, "proj-1")
	if err == nil {
		t.Fatal("expected residual error, got nil")
	}
	if !strings.Contains(err.Error(), "appdev(CREATING)") {
		t.Fatalf("error must name service+status; got: %v", err)
	}
}

// TestVerifyProjectEmpty_OnlyZcp_PassesClean covers the happy path: zcp
// itself is the protected service and must not count as residual.
func TestVerifyProjectEmpty_OnlyZcp_PassesClean(t *testing.T) {
	t.Parallel()
	mock := platform.NewMock().WithServices([]platform.ServiceStack{
		{ID: "svc-zcp", Name: "zcp", Status: "ACTIVE"},
	})
	if err := verifyProjectEmpty(context.Background(), mock, "proj-1"); err != nil {
		t.Fatalf("zcp-only project must verify clean: %v", err)
	}
}

// TestIsAllSettled_TerminalStates pins which statuses count as
// terminal-deletable. CREATING/NEW/STARTING are NOT — they would race
// the platform's create flow and either fail or leave the service stuck.
func TestIsAllSettled_TerminalStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status string
		want   bool
	}{
		{"ACTIVE", true},
		{"READY_TO_DEPLOY", true},
		{"RUNNING", true},
		{"FAILED", true},
		{"STOPPED", true},
		{"CREATING", false},
		{"NEW", false},
		{"STARTING", false},
		{"DELETING", false},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			t.Parallel()
			got := isAllSettled([]platform.ServiceStack{{Name: "x", Status: tt.status}})
			if got != tt.want {
				t.Errorf("isAllSettled(%s) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// TestDeleteAllUserServices_CreatingServiceWaitsThenDeletes simulates the
// observed friction surface: a service is in CREATING when cleanup runs.
// The settle-wait must hold off DeleteService until the service reaches
// ACTIVE, then delete and verify clean.
func TestDeleteAllUserServices_CreatingServiceWaitsThenDeletes(t *testing.T) {
	withTuneables(t, 500*time.Millisecond, 5*time.Millisecond, 3)

	// Mock starts with appdev in CREATING. After ~20ms a goroutine flips
	// it to ACTIVE so settle-wait can advance. WithDeleteRemovesService
	// makes the mock honor the eventual delete, so verify-empty passes
	// after the delete process FINISHEs.
	mock := platform.NewMock().WithServices([]platform.ServiceStack{
		{ID: "svc-app", Name: "appdev", Status: "CREATING"},
	}).WithProcess(&platform.Process{ID: "proc-delete-svc-app", Status: "FINISHED"}).
		WithDeleteRemovesService(true)

	go func() {
		time.Sleep(20 * time.Millisecond)
		mock.WithServices([]platform.ServiceStack{
			{ID: "svc-app", Name: "appdev", Status: "ACTIVE"},
		})
	}()

	if err := deleteAllUserServices(context.Background(), mock, "proj-1"); err != nil {
		t.Fatalf("expected clean cleanup, got: %v", err)
	}
	if got := mock.CallCounts["DeleteService"]; got != 1 {
		t.Fatalf("DeleteService calls: got %d, want 1", got)
	}
}

// TestDeleteAllUserServices_PersistentResidualFailsLoud covers the
// pathological case where a service stays CREATING across all retries.
// Cleanup must not silently proceed — the next scenario needs the
// "residual services" error so the suite stops on a real platform
// problem rather than running a confused agent.
func TestDeleteAllUserServices_PersistentResidualFailsLoud(t *testing.T) {
	withTuneables(t, 30*time.Millisecond, 5*time.Millisecond, 2)

	// Service stays in CREATING forever. Settle-wait times out, delete
	// fires (and "succeeds" per mock), but verify-empty still sees it.
	mock := platform.NewMock().WithServices([]platform.ServiceStack{
		{ID: "svc-stuck", Name: "appdev", Status: "CREATING"},
	}).WithProcess(&platform.Process{ID: "proc-delete-svc-stuck", Status: "FINISHED"})

	err := deleteAllUserServices(context.Background(), mock, "proj-1")
	if err == nil {
		t.Fatal("expected residual error after retries, got nil")
	}
	if !strings.Contains(err.Error(), "residual services") || !strings.Contains(err.Error(), "appdev(CREATING)") {
		t.Fatalf("error must name residual + status; got: %v", err)
	}
	if got := mock.CallCounts["DeleteService"]; got != 2 {
		t.Fatalf("DeleteService should fire once per retry: got %d, want 2", got)
	}
}

func TestIsServiceAlreadyGone_AllChannels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"err_service_not_found_code", platform.NewPlatformError(platform.ErrServiceNotFound, "x", ""), true},
		{"api_code_serviceStackNotFound", func() error {
			pe := platform.NewPlatformError(platform.ErrAPIError, "x", "")
			pe.APICode = "serviceStackNotFound"
			return pe
		}(), true},
		{"api_code_serviceNotFound", func() error {
			pe := platform.NewPlatformError(platform.ErrAPIError, "x", "")
			pe.APICode = "serviceNotFound"
			return pe
		}(), true},
		{"message_only", platform.NewPlatformError(platform.ErrAPIError, "Service stack not found.", ""), true},
		{"unrelated_error", errors.New("api unavailable"), false},
		{"unrelated_platform_error", platform.NewPlatformError(platform.ErrAPIError, "rate limited", ""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isServiceAlreadyGone(tt.err); got != tt.want {
				t.Errorf("isServiceAlreadyGone(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestRemoveClaudeMD_PresentFileRemoved is the happy path: a workdir with a
// CLAUDE.md (with REFLOG content) is wiped clean. Pinned because cleanup's
// step 6 is the explicit guarantee that init.Run on the next scenario sees
// no stale REFLOG; if this regresses, the cross-scenario contamination trap
// (plans/backlog/reflog-vs-live-discover-staleness.md) reopens.
func TestRemoveClaudeMD_PresentFileRemoved(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	contents := "<!-- ZCP:BEGIN -->\nbody\n<!-- ZCP:END -->\n\n<!-- ZEROPS:REFLOG -->\nstale-entry\n<!-- /ZEROPS:REFLOG -->\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("seed CLAUDE.md: %v", err)
	}
	if err := removeClaudeMD(dir); err != nil {
		t.Fatalf("removeClaudeMD on present file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md should be gone; stat err: %v", err)
	}
}

// TestRemoveClaudeMD_AbsentFileNoOp covers the post-cleanWorkDir path:
// cleanWorkDir already removed CLAUDE.md, so step 6 sees nothing to do.
// Must NOT return an error — that would convert a benign double-pass into
// a hard cleanup failure.
func TestRemoveClaudeMD_AbsentFileNoOp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := removeClaudeMD(dir); err != nil {
		t.Fatalf("removeClaudeMD on absent file must be no-op; got: %v", err)
	}
}

// TestCleanupProject_RemovesClaudeMD_EndToEnd pins that the full
// CleanupProject sequence ends with no CLAUDE.md in workDir. Uses a clean
// platform mock so only the workdir-cleanup half is exercised; the
// service-deletion half is covered by sibling tests.
func TestCleanupProject_RemovesClaudeMD_EndToEnd(t *testing.T) {
	t.Parallel()
	withTuneables(t, 200*time.Millisecond, 5*time.Millisecond, 2)

	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("<!-- ZCP:BEGIN -->\n...\n"), 0o644); err != nil {
		t.Fatalf("seed CLAUDE.md: %v", err)
	}

	// Empty project (only zcp), so service-deletion half is a no-op.
	mock := platform.NewMock().WithServices([]platform.ServiceStack{
		{ID: "svc-zcp", Name: "zcp", Status: "ACTIVE"},
	})

	if err := CleanupProject(context.Background(), mock, "proj-1", dir); err != nil {
		t.Fatalf("CleanupProject: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("CLAUDE.md should be gone after CleanupProject; stat err: %v", err)
	}
}
