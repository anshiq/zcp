package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zeropsio/zcp/internal/ops"
	"github.com/zeropsio/zcp/internal/platform"
	"github.com/zeropsio/zcp/internal/runtime"
	"github.com/zeropsio/zcp/internal/topology"
)

// launchSSHStub is a minimal SSH stub returning canned responses for
// the substring patterns the launch source-state reader uses. Mirrors
// integration/export_test.go::exportSSHStub but lives inline here so
// the test stays in the tools package (white-box access to handler
// factory + state file helpers).
type launchSSHStub struct {
	responses map[string]string
}

func (s *launchSSHStub) ExecSSH(_ context.Context, _ string, command string) ([]byte, error) {
	for k, v := range s.responses {
		if strings.Contains(command, k) {
			return []byte(v), nil
		}
	}
	return nil, nil
}

func (s *launchSSHStub) ExecSSHBackground(_ context.Context, _, _ string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

// sequenceLaunchYAML is the canned source zerops.yaml the SSH stub
// returns. Contains dev + prod setup blocks so the source-control
// gate's hasSetupProd check passes.
const sequenceLaunchYAML = `zerops:
  - setup: dev
    build:
      base: nodejs@22
      buildCommands:
        - npm install
    run:
      base: nodejs@22
      start: node dist/server.js
  - setup: prod
    build:
      base: nodejs@22
      buildCommands:
        - npm ci
        - npm run build
    run:
      base: nodejs@22
      start: node dist/server.js
      healthCheck:
        httpGet:
          port: 3000
          path: /health
`

// TestHandleLaunchProduction_FullSequence_HappyPath walks the launch
// workflow through every status transition in one test:
//   - call 1 (empty input)            → scope-prompt
//   - call 2 (productionProjectName)  → classify-prompt
//   - call 3 (+ classifications)      → ready-to-launch (no launchKey)
//   - call 4 (+ launchKey + target)   → launched
//
// Exercises the handler's stateless multi-call narrowing + state file
// persistence + mock admin mutation + post-launch poll aggregation.
//
// Item #7 from the deferred-list — handler-level e2e for the full
// launch flow. White-box test in the tools package so the
// projectAdminClientFactory swap works.
func TestHandleLaunchProduction_FullSequence_HappyPath(t *testing.T) {
	stateDir := withTempState(t)

	// Mock admin returns success on CreateAndImportProject + Process
	// poll returns FINISHED so the launched path runs end-to-end.
	mockAdmin := platform.NewMockProjectAdminClient().
		WithImportResult(&platform.ImportResult{
			ProjectID:   "new-prod-id",
			ProjectName: "myapp-prod",
			ServiceStacks: []platform.ImportedServiceStack{
				{
					ID:   "svc-app",
					Name: "app",
					Processes: []platform.Process{
						{ID: "proc-build-1", Status: "FINISHED"},
					},
				},
			},
		}).
		WithProcess(&platform.Process{ID: "proc-build-1", Status: "FINISHED"}).
		WithClientUserID("client-user-abc")
	defer installMockAdminFactory(t, mockAdmin)()

	// Mock platform.Client for source-project Discover + envs.
	mockClient := platform.NewMock().
		WithProject(&platform.Project{ID: "source-id", Name: "myapp-dev", Status: "ACTIVE"}).
		WithServices([]platform.ServiceStack{
			{
				ID:   "svc-app-src",
				Name: "app",
				ServiceStackTypeInfo: platform.ServiceTypeInfo{
					ServiceStackTypeVersionName:  "nodejs@22",
					ServiceStackTypeCategoryName: "USER",
				},
				Status: "ACTIVE",
				Mode:   "NON_HA",
			},
		}).
		WithProjectEnv([]platform.EnvVar{
			{Key: "LOG_LEVEL", Content: "info"},
		})

	// SSH stub returns the source zerops.yaml (with setup: prod block)
	// + git remote + git SHA when the source-state reader queries.
	ssh := &launchSSHStub{responses: map[string]string{
		"cat /var/www/zerops.yaml": sequenceLaunchYAML,
		"git remote get-url":       "https://github.com/example/myapp.git",
		"git rev-parse HEAD":       "abc123def456",
	}}

	rt := runtime.Info{InContainer: true, ServiceName: "zcp"}
	ctx := context.Background()

	// Call 1 — scope-prompt: empty input.
	call1, _, err := handleLaunchProduction(ctx, "source-id", mockClient, WorkflowInput{
		Workflow: workflowLaunchProduction,
	}, stateDir, rt, ssh)
	if err != nil {
		t.Fatalf("call 1: %v", err)
	}
	resp1 := decodeLaunchResp(t, []byte(extractText(call1)))
	if resp1.Status != topology.LaunchStatusScopePrompt {
		t.Errorf("call 1 status: got %q want scope-prompt", resp1.Status)
	}

	// Call 2 — classify-prompt: project name set, classifications empty.
	call2, _, err := handleLaunchProduction(ctx, "source-id", mockClient, WorkflowInput{
		Workflow:              workflowLaunchProduction,
		ProductionProjectName: "myapp-prod",
		Region:                "eu-central",
	}, stateDir, rt, ssh)
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	resp2 := decodeLaunchResp(t, []byte(extractText(call2)))
	if resp2.Status != topology.LaunchStatusClassifyPrompt {
		t.Errorf("call 2 status: got %q want classify-prompt", resp2.Status)
	}
	if len(resp2.Classifications) != 1 || resp2.Classifications[0].Key != "LOG_LEVEL" {
		t.Errorf("call 2 classifications: got %+v", resp2.Classifications)
	}

	// Call 3 — ready-to-launch: classifications complete, no launchKey.
	call3, _, err := handleLaunchProduction(ctx, "source-id", mockClient, WorkflowInput{
		Workflow:              workflowLaunchProduction,
		ProductionProjectName: "myapp-prod",
		Region:                "eu-central",
		TargetService:         "app",
		EnvClassifications:    map[string]string{"LOG_LEVEL": "plain-config"},
	}, stateDir, rt, ssh)
	if err != nil {
		t.Fatalf("call 3: %v", err)
	}
	resp3 := decodeLaunchResp(t, []byte(extractText(call3)))
	if resp3.Status != topology.LaunchStatusReadyToLaunch {
		t.Errorf("call 3 status: got %q want ready-to-launch", resp3.Status)
	}

	// Call 4 — publish: launchKey supplied + full inputs → launched.
	call4, _, err := handleLaunchProduction(ctx, "source-id", mockClient, WorkflowInput{
		Workflow:              workflowLaunchProduction,
		ProductionProjectName: "myapp-prod",
		Region:                "eu-central",
		TargetService:         "app",
		EnvClassifications:    map[string]string{"LOG_LEVEL": "plain-config"},
		LaunchKey:             sentinelLaunchKey,
	}, stateDir, rt, ssh)
	if err != nil {
		t.Fatalf("call 4: %v", err)
	}
	resp4 := decodeLaunchResp(t, []byte(extractText(call4)))
	if resp4.Status != topology.LaunchStatusLaunched {
		t.Fatalf("call 4 status: got %q want launched\nresponse: %s", resp4.Status, extractText(call4))
	}

	// P-LP-1: launchKey must not appear in any response.
	for i, body := range []string{extractText(call1), extractText(call2), extractText(call3), extractText(call4)} {
		if strings.Contains(body, sentinelLaunchKey) {
			t.Errorf("call %d response leaks launchKey: %s", i+1, body)
		}
	}

	// Admin mock captured the import.
	if mockAdmin.CapturedImportYAML == "" {
		t.Error("admin.CreateAndImportProject not invoked")
	}
	if !strings.Contains(mockAdmin.CapturedImportYAML, "myapp-prod") {
		t.Errorf("import yaml missing target name: %s", mockAdmin.CapturedImportYAML)
	}

	// Admin mock granted self ADMIN role (A.10).
	if mockAdmin.CapturedGrantSelfRoleProject != "new-prod-id" {
		t.Errorf("GrantSelfRole not called with target project; got %q", mockAdmin.CapturedGrantSelfRoleProject)
	}
	if mockAdmin.CapturedGrantSelfRoleCode != "ADMIN" {
		t.Errorf("GrantSelfRole roleCode: got %q want ADMIN", mockAdmin.CapturedGrantSelfRoleCode)
	}

	// State file written with launched status.
	launchID := generateLaunchID("source-id", "myapp-prod")
	state, err := readLaunchState(stateDir, launchID)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if state == nil {
		t.Fatal("state file missing post-launch")
	}
	if state.Status != topology.LaunchStatusLaunched {
		t.Errorf("state.Status: got %q want launched", state.Status)
	}
	if state.TargetProjectID != "new-prod-id" {
		t.Errorf("state.TargetProjectID: got %q", state.TargetProjectID)
	}

	// Compile-time pin: launchSSHStub satisfies ops.SSHDeployer.
	var _ ops.SSHDeployer = (*launchSSHStub)(nil)
}

// TestHandleLaunchProduction_FullSequence_ProcessFailedTransitionsToFailed
// pins that when admin import succeeds but a process poll surfaces
// FAILED, the handler transitions state.Status → failed (not launched).
func TestHandleLaunchProduction_FullSequence_ProcessFailedTransitionsToFailed(t *testing.T) {
	stateDir := withTempState(t)
	failReason := "build failed: tsc compile error"

	mockAdmin := platform.NewMockProjectAdminClient().
		WithImportResult(&platform.ImportResult{
			ProjectID:   "new-prod-id",
			ProjectName: "myapp-prod",
			ServiceStacks: []platform.ImportedServiceStack{
				{
					ID:   "svc-app",
					Name: "app",
					Processes: []platform.Process{
						{ID: "proc-build-1", Status: "FINISHED"},
					},
				},
			},
		}).
		WithProcess(&platform.Process{ID: "proc-build-1", Status: "FAILED", FailReason: &failReason}).
		WithClientUserID("client-user-abc")
	defer installMockAdminFactory(t, mockAdmin)()

	mockClient := platform.NewMock().
		WithProject(&platform.Project{ID: "source-id", Name: "myapp-dev", Status: "ACTIVE"}).
		WithServices([]platform.ServiceStack{
			{
				ID:   "svc-app-src",
				Name: "app",
				ServiceStackTypeInfo: platform.ServiceTypeInfo{
					ServiceStackTypeVersionName:  "nodejs@22",
					ServiceStackTypeCategoryName: "USER",
				},
				Status: "ACTIVE",
			},
		})

	ssh := &launchSSHStub{responses: map[string]string{
		"cat /var/www/zerops.yaml": sequenceLaunchYAML,
		"git remote get-url":       "https://github.com/example/myapp.git",
		"git rev-parse HEAD":       "abc123def456",
	}}

	rt := runtime.Info{InContainer: true, ServiceName: "zcp"}
	call, _, err := handleLaunchProduction(context.Background(), "source-id", mockClient, WorkflowInput{
		Workflow:              workflowLaunchProduction,
		ProductionProjectName: "myapp-prod",
		Region:                "eu-central",
		TargetService:         "app",
		LaunchKey:             sentinelLaunchKey,
	}, stateDir, rt, ssh)
	if err != nil {
		t.Fatalf("handleLaunchProduction: %v", err)
	}
	resp := decodeLaunchResp(t, []byte(extractText(call)))
	if resp.Status != topology.LaunchStatusFailed {
		t.Fatalf("status: got %q want failed", resp.Status)
	}
	if len(resp.Blockers) == 0 {
		t.Fatal("expected failure blocker")
	}
}
