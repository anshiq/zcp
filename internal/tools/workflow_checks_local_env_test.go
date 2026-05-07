package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeropsio/zcp/internal/platform"
)

// TestCheckLocalDotenvFresh_Fresh pins that a .env in sync with sources
// reports LocalDotenvFresh with no recovery hint.
func TestCheckLocalDotenvFresh_Fresh(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yaml := `zerops:
  - setup: app
    run:
      envVariables:
        APP_NAME: hello
`
	if err := os.WriteFile(filepath.Join(tmpDir, "zerops.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write zerops.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("APP_NAME=hello\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	mock := platform.NewMock().
		WithProject(&platform.Project{ID: "p1", Name: "test", Status: statusActive}).
		WithServices([]platform.ServiceStack{
			{ID: "svc-app", Name: "app", ProjectID: "p1", Status: statusActive},
		})

	got := checkLocalDotenvFresh(context.Background(), mock, "p1", "app", tmpDir)
	if got.Status != LocalDotenvFresh {
		t.Errorf("Status = %s, want %s (detail: %s)", got.Status, LocalDotenvFresh, got.Detail)
	}
	if got.RecoveryHint != nil {
		t.Errorf("Fresh status should have no recovery hint; got %+v", got.RecoveryHint)
	}
}

// TestCheckLocalDotenvFresh_Stale pins that yaml-changed-vs-env
// surfaces LocalDotenvStale with a generate-dotenv recovery hint.
func TestCheckLocalDotenvFresh_Stale(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yaml := `zerops:
  - setup: app
    run:
      envVariables:
        APP_NAME: new-value
        DEBUG: true
`
	if err := os.WriteFile(filepath.Join(tmpDir, "zerops.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write zerops.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("APP_NAME=old-value\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	mock := platform.NewMock().
		WithProject(&platform.Project{ID: "p1", Name: "test", Status: statusActive}).
		WithServices([]platform.ServiceStack{
			{ID: "svc-app", Name: "app", ProjectID: "p1", Status: statusActive},
		})

	got := checkLocalDotenvFresh(context.Background(), mock, "p1", "app", tmpDir)
	if got.Status != LocalDotenvStale {
		t.Errorf("Status = %s, want %s", got.Status, LocalDotenvStale)
	}
	if got.RecoveryHint == nil || got.RecoveryHint.Action != "generate-dotenv" {
		t.Errorf("recovery hint should target generate-dotenv; got %+v", got.RecoveryHint)
	}
}

// TestCheckLocalDotenvFresh_UnownedEdits pins that user-direct edits
// to .env (keys not in any source) surface LocalDotenvUnownedEdits
// with a preview-recovery hint.
func TestCheckLocalDotenvFresh_UnownedEdits(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yaml := `zerops:
  - setup: app
    run:
      envVariables:
        APP_NAME: hello
`
	if err := os.WriteFile(filepath.Join(tmpDir, "zerops.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write zerops.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("APP_NAME=hello\nMANUAL_EDIT=foo\n"), 0600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	mock := platform.NewMock().
		WithProject(&platform.Project{ID: "p1", Name: "test", Status: statusActive}).
		WithServices([]platform.ServiceStack{
			{ID: "svc-app", Name: "app", ProjectID: "p1", Status: statusActive},
		})

	got := checkLocalDotenvFresh(context.Background(), mock, "p1", "app", tmpDir)
	if got.Status != LocalDotenvUnownedEdits {
		t.Errorf("Status = %s, want %s", got.Status, LocalDotenvUnownedEdits)
	}
	if got.Diff == nil || len(got.Diff.Unowned) == 0 {
		t.Errorf("Diff.Unowned should list MANUAL_EDIT; got %+v", got.Diff)
	}
	if got.RecoveryHint == nil || got.RecoveryHint.Args["preview"] != "true" {
		t.Errorf("recovery hint should suggest preview=true; got %+v", got.RecoveryHint)
	}
}

// TestCheckLocalDotenvFresh_Missing pins that absent .env surfaces
// LocalDotenvMissing with a regen recovery hint.
func TestCheckLocalDotenvFresh_Missing(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yaml := `zerops:
  - setup: app
    run:
      envVariables:
        APP_NAME: hello
`
	if err := os.WriteFile(filepath.Join(tmpDir, "zerops.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write zerops.yaml: %v", err)
	}

	mock := platform.NewMock().
		WithProject(&platform.Project{ID: "p1", Name: "test", Status: statusActive}).
		WithServices([]platform.ServiceStack{
			{ID: "svc-app", Name: "app", ProjectID: "p1", Status: statusActive},
		})

	got := checkLocalDotenvFresh(context.Background(), mock, "p1", "app", tmpDir)
	if got.Status != LocalDotenvMissing {
		t.Errorf("Status = %s, want %s", got.Status, LocalDotenvMissing)
	}
	if got.RecoveryHint == nil {
		t.Error("missing .env should have recovery hint pointing at generate-dotenv")
	}
}

// TestCheckLocalDotenvFresh_VPNDown pins that transient resolve
// failures surface LocalDotenvVPNDown with the preview hint (so
// agent can re-check after vpn up without writing).
func TestCheckLocalDotenvFresh_VPNDown(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	yaml := `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "zerops.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatalf("write zerops.yaml: %v", err)
	}

	mock := platform.NewMock().
		WithProject(&platform.Project{ID: "p1", Name: "test", Status: statusActive}).
		WithServices([]platform.ServiceStack{
			{ID: "svc-app", Name: "app", ProjectID: "p1", Status: statusActive},
			{ID: "svc-db", Name: "db", ProjectID: "p1", Status: statusActive},
		}).
		WithError("GetServiceEnv", errors.New("connection refused"))

	got := checkLocalDotenvFresh(context.Background(), mock, "p1", "app", tmpDir)
	if got.Status != LocalDotenvVPNDown {
		t.Errorf("Status = %s, want %s (detail: %s)", got.Status, LocalDotenvVPNDown, got.Detail)
	}
}

// TestCheckLocalDotenvFresh_Skipped_NoZeropsYaml pins that absence
// of zerops.yaml in CWD skips the check (not an error — this is the
// not-applicable signal).
func TestCheckLocalDotenvFresh_Skipped_NoZeropsYaml(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	mock := platform.NewMock().
		WithProject(&platform.Project{ID: "p1", Name: "test", Status: statusActive})
	got := checkLocalDotenvFresh(context.Background(), mock, "p1", "app", tmpDir)
	if got.Status != LocalDotenvSkipped {
		t.Errorf("Status = %s, want %s", got.Status, LocalDotenvSkipped)
	}
}
