package tools

import (
	"testing"
	"time"

	"github.com/zeropsio/zcp/internal/platform"
	"github.com/zeropsio/zcp/internal/topology"
	"github.com/zeropsio/zcp/internal/workflow"
)

func TestContainerCreationAnchor_PriorityOrder(t *testing.T) {
	t.Parallel()

	ccs := time.Date(2026, 4, 22, 6, 5, 0, 0, time.UTC).Format(time.RFC3339Nano)
	pfinish := time.Date(2026, 4, 22, 6, 4, 58, 0, time.UTC).Format(time.RFC3339Nano)
	pfailed := time.Date(2026, 4, 22, 6, 4, 55, 0, time.UTC).Format(time.RFC3339Nano)
	pstart := time.Date(2026, 4, 22, 6, 4, 0, 0, time.UTC).Format(time.RFC3339Nano)

	tests := []struct {
		name  string
		build *platform.BuildInfo
		want  string
	}{
		{
			name:  "nil build returns zero time",
			build: nil,
			want:  "",
		},
		{
			name: "ContainerCreationStart wins over PipelineFinish",
			build: &platform.BuildInfo{
				ContainerCreationStart: &ccs,
				PipelineFinish:         &pfinish,
				PipelineFailed:         &pfailed,
				PipelineStart:          &pstart,
			},
			want: ccs,
		},
		{
			name: "PipelineFinish wins over PipelineFailed",
			build: &platform.BuildInfo{
				PipelineFinish: &pfinish,
				PipelineFailed: &pfailed,
				PipelineStart:  &pstart,
			},
			want: pfinish,
		},
		{
			name: "PipelineFailed wins over PipelineStart",
			build: &platform.BuildInfo{
				PipelineFailed: &pfailed,
				PipelineStart:  &pstart,
			},
			want: pfailed,
		},
		{
			name: "PipelineStart is the last fallback",
			build: &platform.BuildInfo{
				PipelineStart: &pstart,
			},
			want: pstart,
		},
		{
			name:  "nothing set returns zero",
			build: &platform.BuildInfo{},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ev := &platform.AppVersionEvent{Build: tt.build}
			got := containerCreationAnchor(ev)
			if tt.want == "" {
				if !got.IsZero() {
					t.Errorf("expected zero time, got %v", got)
				}
				return
			}
			wantT, _ := time.Parse(time.RFC3339Nano, tt.want)
			if !got.Equal(wantT) {
				t.Errorf("got %v, want %v", got, wantT)
			}
		})
	}
}

// TestResolveDeployTargetTopology pins the integration between
// FindServiceMeta (pair-keyed: stage hostname resolves to dev-keyed
// pair meta) and ServiceMeta.ModeFor (target-relative projection):
// a standard pair stage deploy must NOT match topology.IsDeferredStart
// because the stage runtime runs run.start, not zsc-noop. Reading
// meta.Mode directly here previously read ModeStandard for the stage
// half and matched IsDeferredStart, so the post-deploy next-action
// emitted "Start dev server: zerops_dev_server action=start" — a tool
// the stage runtime doesn't expose. Pre-fix this never surfaced in
// unit tests because TestDeploySuccessNextActions called
// deploySuccessNextActions with explicit (mode, class) inputs and
// never went through FindServiceMeta + ModeFor.
func TestResolveDeployTargetTopology(t *testing.T) {
	t.Parallel()

	standardPair := &workflow.ServiceMeta{
		Hostname:       "appdev",
		StageHostname:  "appstage",
		Mode:           topology.PlanModeStandard,
		BootstrappedAt: time.Now().UTC().Format(time.RFC3339),
	}
	devOnly := &workflow.ServiceMeta{
		Hostname:       "appdev",
		Mode:           topology.PlanModeDev,
		BootstrappedAt: time.Now().UTC().Format(time.RFC3339),
	}
	simple := &workflow.ServiceMeta{
		Hostname:       "worker",
		Mode:           topology.PlanModeSimple,
		BootstrappedAt: time.Now().UTC().Format(time.RFC3339),
	}
	localStage := &workflow.ServiceMeta{
		Hostname:       "myproject",
		StageHostname:  "apistage",
		Mode:           topology.PlanModeLocalStage,
		BootstrappedAt: time.Now().UTC().Format(time.RFC3339),
	}

	tests := []struct {
		name             string
		metas            []*workflow.ServiceMeta
		target           string
		typeVersion      string
		wantMode         topology.Mode
		wantClass        topology.RuntimeClass
		wantDeferredStrt bool // expected IsDeferredStart(mode, class)
	}{
		{
			name:             "standard_pair_stage_half_dynamic_runtime",
			metas:            []*workflow.ServiceMeta{standardPair},
			target:           "appstage",
			typeVersion:      "nodejs@22",
			wantMode:         topology.ModeStage,
			wantClass:        topology.RuntimeDynamic,
			wantDeferredStrt: false, // stage runs run.start, not zsc-noop
		},
		{
			name:             "standard_pair_dev_half_dynamic_runtime",
			metas:            []*workflow.ServiceMeta{standardPair},
			target:           "appdev",
			typeVersion:      "nodejs@22",
			wantMode:         topology.ModeStandard,
			wantClass:        topology.RuntimeDynamic,
			wantDeferredStrt: true, // dev half is zsc-noop until dev_server start
		},
		{
			name:             "dev_only_dynamic_runtime",
			metas:            []*workflow.ServiceMeta{devOnly},
			target:           "appdev",
			typeVersion:      "nodejs@22",
			wantMode:         topology.ModeDev,
			wantClass:        topology.RuntimeDynamic,
			wantDeferredStrt: true,
		},
		{
			name:             "simple_mode_dynamic_runtime",
			metas:            []*workflow.ServiceMeta{simple},
			target:           "worker",
			typeVersion:      "go@1",
			wantMode:         topology.ModeSimple,
			wantClass:        topology.RuntimeDynamic,
			wantDeferredStrt: false,
		},
		{
			name:             "local_stage_pair_stage_half",
			metas:            []*workflow.ServiceMeta{localStage},
			target:           "apistage",
			typeVersion:      "nodejs@22",
			wantMode:         topology.ModeLocalStage,
			wantClass:        topology.RuntimeDynamic,
			wantDeferredStrt: false, // local-stage runtime runs run.start
		},
		{
			name:             "no_meta_returns_empty",
			metas:            nil,
			target:           "unknown",
			typeVersion:      "nodejs@22",
			wantMode:         "",
			wantClass:        "",
			wantDeferredStrt: false,
		},
		{
			name:             "implicit_webserver_runtime",
			metas:            []*workflow.ServiceMeta{standardPair},
			target:           "appdev",
			typeVersion:      "php-nginx@8.4",
			wantMode:         topology.ModeStandard,
			wantClass:        topology.RuntimeImplicitWeb,
			wantDeferredStrt: false, // implicit web auto-starts; not deferred
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stateDir := t.TempDir()
			for _, m := range tt.metas {
				if err := workflow.WriteServiceMeta(stateDir, m); err != nil {
					t.Fatalf("WriteServiceMeta(%q): %v", m.Hostname, err)
				}
			}
			gotMode, gotClass := resolveDeployTargetTopology(stateDir, tt.target, tt.typeVersion)
			if gotMode != tt.wantMode {
				t.Errorf("mode = %q, want %q", gotMode, tt.wantMode)
			}
			if gotClass != tt.wantClass {
				t.Errorf("class = %q, want %q", gotClass, tt.wantClass)
			}
			gotDeferredStrt := topology.IsDeferredStart(gotMode, gotClass)
			if gotDeferredStrt != tt.wantDeferredStrt {
				t.Errorf("IsDeferredStart(%q, %q) = %v, want %v",
					gotMode, gotClass, gotDeferredStrt, tt.wantDeferredStrt)
			}
		})
	}
}

// TestResolveDeployTargetTopology_EmptyArgs covers the early-return guards.
func TestResolveDeployTargetTopology_EmptyArgs(t *testing.T) {
	t.Parallel()
	gotMode, gotClass := resolveDeployTargetTopology("", "appdev", "nodejs@22")
	if gotMode != "" || gotClass != "" {
		t.Errorf("empty stateDir: got (%q, %q), want (\"\", \"\")", gotMode, gotClass)
	}
	gotMode, gotClass = resolveDeployTargetTopology(t.TempDir(), "", "nodejs@22")
	if gotMode != "" || gotClass != "" {
		t.Errorf("empty target: got (%q, %q), want (\"\", \"\")", gotMode, gotClass)
	}
}
