package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zeropsio/zerops-go/dto/input/body"
	"github.com/zeropsio/zerops-go/dto/input/path"
	zgotypes "github.com/zeropsio/zerops-go/types"
	"github.com/zeropsio/zerops-go/types/uuid"
)

// ProjectAdminClient is the cross-project mutation surface used exclusively
// by the launch-production workflow handler. Constructed per workflow
// invocation from a user-supplied launch-window key, used during a single
// workflow run, and discarded.
//
// Discipline (P-LP-2): NewProjectAdminClient is callable only from
// internal/tools/workflow_launch_production.go. Pinned by
// internal/topology/architecture_test.go::TestProjectAdminClientRestrictedImport.
//
// Key handling (P-LP-1):
//   - The launch-window key flows in via the constructor only.
//   - The struct holding the key uses a private field with no getter/Stringer.
//   - Close() zeros the underlying SDK handler reference for GC reachability.
//   - The key is never serialized, logged, or written to state.
//   - GetServiceEnvKeys / GetProjectEnvKeys return EnvKey (no Value field) —
//     P-LP-5 invariant: ZCP never reads external secret values.
type ProjectAdminClient interface {
	// CreateAndImportProject wraps PostClientProjectImport — single API call
	// that creates the project and imports services from import YAML in one
	// shot. Returns synchronously with the new project ID + per-service
	// stack IDs + per-service async processes to poll. Per-service `Error`
	// surfaces import-time validation issues without aborting the whole
	// import.
	CreateAndImportProject(ctx context.Context, yaml string, opts CreateOpts) (*ImportResult, error)

	// ListServices for the target project (read-only). Used to verify
	// external-secret presence post-import + to discover service IDs for
	// further calls.
	ListServices(ctx context.Context, projectID string) ([]ServiceStack, error)

	// GetServiceEnvKeys returns env entry keys + sensitive flag for a
	// service. Intentionally omits Value field per P-LP-5. Used to verify
	// that the user has set external secrets in Zerops UI without ZCP
	// reading those values.
	GetServiceEnvKeys(ctx context.Context, serviceID string) ([]EnvKey, error)

	// GetProjectEnvKeys returns project-level env entry keys + sensitive
	// flag. Same omit-Value semantics.
	GetProjectEnvKeys(ctx context.Context, projectID string) ([]EnvKey, error)

	// GetProcess fetches an async process state. Wraps existing infra.
	GetProcess(ctx context.Context, processID string) (*Process, error)

	// DeleteProject initiates an async project delete. Returns the
	// delete-process; caller polls via GetProcess.
	DeleteProject(ctx context.Context, projectID string) (*Process, error)

	// Close zeros internal references so the GC reclaims the SDK handler
	// (which holds the launch-window key inside its authenticated transport).
	// Caller MUST `defer admin.Close()`.
	Close()
}

// CreateOpts holds project-creation options NOT derived from the import yaml.
// Project name + envVariables + service stack list all come from the import
// yaml body. CreateOpts carries the dimensions the yaml doesn't naturally
// express (region, tags appended at create-time for organization).
type CreateOpts struct {
	// Location is the region code. Default if empty: derived from yaml or
	// platform default. Verified values include "eu-central". See spec
	// docs/spec-launch-production-platform-spike.md §A.4.
	Location string
	// Tags applied to the project at creation; appended to whatever the
	// import yaml declares (under project.tags).
	Tags []string
}

// EnvKey is an environment variable entry surfaced WITHOUT its value.
//
// Distinct from EnvVar (which carries Content) by design — used when ZCP
// verifies the presence of user-set external secrets in a target prod
// project without ever reading those secrets through MCP. Pinned by
// P-LP-5 invariant.
type EnvKey struct {
	ID        string
	Key       string
	Sensitive bool
}

// ErrEmptyLaunchKey is returned by NewProjectAdminClient when launchKey is "".
var ErrEmptyLaunchKey = errors.New("project admin: launch-window API key required")

// ErrNoClientResolved is returned when the supplied key authenticates but
// resolves to no client / org. Account-wide scope is required for
// CreateAndImportProject.
var ErrNoClientResolved = errors.New("project admin: launch-window key resolves to no client (org-wide access required)")

// ErrClientClosed is returned by ProjectAdminClient methods after Close().
var ErrClientClosed = errors.New("project admin: client closed")

// defaultAPIHostForAdmin is used when no apiHost is supplied. Mirrors the
// existing defaultAPIHost in the apitest harness; kept local so platform
// stays self-contained.
const defaultAPIHostForAdmin = "api.app-prg1.zerops.io"

// NewProjectAdminClient constructs a ProjectAdminClient from a launch-window
// key. The key is held internally by the wrapped ZeropsClient (inside its
// SDK handler's authenticated transport); this struct never copies it into
// a separately addressable field.
//
// Behavior:
//   - Empty launchKey → ErrEmptyLaunchKey.
//   - Validates the key by calling GetUserInfo (one cheap GET). Invalid or
//     expired keys surface the SDK's mapped error.
//   - Discovers clientID from the response — needed for CreateAndImportProject.
//   - Returns ErrNoClientResolved if the key authenticates but lacks org access.
//
// Caller MUST defer Close() on the returned client.
func NewProjectAdminClient(launchKey, apiHost string) (ProjectAdminClient, error) {
	if launchKey == "" {
		return nil, ErrEmptyLaunchKey
	}
	if apiHost == "" {
		apiHost = defaultAPIHostForAdmin
	}
	z, err := NewZeropsClient(launchKey, apiHost)
	if err != nil {
		return nil, fmt.Errorf("project admin: construct client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := z.GetUserInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("project admin: validate key: %w", err)
	}
	if info.ID == "" {
		return nil, ErrNoClientResolved
	}
	return &projectAdminClient{
		zerops:   z,
		clientID: info.ID,
	}, nil
}

// projectAdminClient implements ProjectAdminClient against the live SDK.
//
// The wrapped *ZeropsClient holds the launch-window key inside its
// authenticated SDK handler — there is no separate token field on this
// struct, so the key is unreachable from any reflection / String() /
// JSON path on projectAdminClient.
type projectAdminClient struct {
	zerops   *ZeropsClient
	clientID string
}

// CreateAndImportProject implements ProjectAdminClient.
func (p *projectAdminClient) CreateAndImportProject(ctx context.Context, yaml string, opts CreateOpts) (*ImportResult, error) {
	if p.zerops == nil {
		return nil, ErrClientClosed
	}
	_ = opts // CreateOpts.Location / Tags are encoded into the yaml body by the caller (LaunchBundleBuilder); kept on the signature for future expansion if the API gains separate fields.

	pathParam := path.ClientId{Id: uuid.ClientId(p.clientID)}
	bodyParam := body.ProjectImport{
		Yaml: zgotypes.Text(yaml),
	}
	resp, err := p.zerops.handler.PostClientProjectImport(ctx, pathParam, bodyParam)
	if err != nil {
		return nil, mapSDKError(err, "project")
	}
	out, err := resp.Output()
	if err != nil {
		return nil, mapSDKError(err, "project")
	}

	result := &ImportResult{
		ProjectID:   out.ProjectId.TypedString().String(),
		ProjectName: out.ProjectName.String(),
	}
	for _, stack := range out.ServiceStacks {
		imported := ImportedServiceStack{
			ID:   stack.Id.TypedString().String(),
			Name: stack.Name.String(),
		}
		if stack.Error != nil {
			imported.Error = &APIError{
				Code:    stack.Error.Code.String(),
				Message: stack.Error.Message.String(),
				Meta:    decodeAPIMetaJSON(stack.Error.Meta.Native()),
			}
		}
		for _, proc := range stack.Processes {
			imported.Processes = append(imported.Processes, mapProcess(proc))
		}
		result.ServiceStacks = append(result.ServiceStacks, imported)
	}
	return result, nil
}

// ListServices implements ProjectAdminClient — same semantics as Client.ListServices
// but uses the admin transport.
func (p *projectAdminClient) ListServices(ctx context.Context, projectID string) ([]ServiceStack, error) {
	if p.zerops == nil {
		return nil, ErrClientClosed
	}
	return p.zerops.ListServices(ctx, projectID)
}

// GetServiceEnvKeys implements ProjectAdminClient — returns EnvKey entries
// stripped of values. Wraps the existing GetServiceEnv but discards Content.
func (p *projectAdminClient) GetServiceEnvKeys(ctx context.Context, serviceID string) ([]EnvKey, error) {
	if p.zerops == nil {
		return nil, ErrClientClosed
	}
	vars, err := p.zerops.GetServiceEnv(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	return stripEnvValues(vars), nil
}

// GetProjectEnvKeys implements ProjectAdminClient — returns project-level
// EnvKey entries stripped of values.
func (p *projectAdminClient) GetProjectEnvKeys(ctx context.Context, projectID string) ([]EnvKey, error) {
	if p.zerops == nil {
		return nil, ErrClientClosed
	}
	vars, err := p.zerops.GetProjectEnv(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return stripEnvValues(vars), nil
}

// GetProcess implements ProjectAdminClient — same semantics as Client.GetProcess.
func (p *projectAdminClient) GetProcess(ctx context.Context, processID string) (*Process, error) {
	if p.zerops == nil {
		return nil, ErrClientClosed
	}
	return p.zerops.GetProcess(ctx, processID)
}

// DeleteProject implements ProjectAdminClient. Wraps SDK DeleteProject.
func (p *projectAdminClient) DeleteProject(ctx context.Context, projectID string) (*Process, error) {
	if p.zerops == nil {
		return nil, ErrClientClosed
	}
	pathParam := path.ProjectId{Id: uuid.ProjectId(projectID)}
	resp, err := p.zerops.handler.DeleteProject(ctx, pathParam)
	if err != nil {
		return nil, mapSDKError(err, "project")
	}
	out, err := resp.Output()
	if err != nil {
		return nil, mapSDKError(err, "project")
	}
	proc := mapProcess(out)
	return &proc, nil
}

// Close zeros the wrapped client reference so the SDK handler (which holds
// the launch-window key) becomes unreachable and eligible for GC. Subsequent
// method calls return ErrClientClosed.
func (p *projectAdminClient) Close() {
	p.zerops = nil
	p.clientID = ""
}

// stripEnvValues maps []EnvVar (with Content) to []EnvKey (without Content).
// Centralizing the strip ensures GetServiceEnvKeys / GetProjectEnvKeys can
// never accidentally surface values — P-LP-5 invariant.
func stripEnvValues(vars []EnvVar) []EnvKey {
	out := make([]EnvKey, 0, len(vars))
	for _, v := range vars {
		out = append(out, EnvKey{
			ID:        v.ID,
			Key:       v.Key,
			Sensitive: envEntrySensitive(v),
		})
	}
	return out
}

// envEntrySensitive returns true if EnvVar.Content indicates a sensitive
// entry per platform convention. The Zerops API marks sensitive entries
// with a server-side flag we don't yet model on EnvVar; until we do, this
// is a no-op returning false. Phase B e2e (TestProjectAdminClient_GetServiceEnv_OmitsValues)
// verifies real sensitive-flag handling end-to-end.
//
// Note: even when Sensitive=false here, the Value field is STILL omitted
// from EnvKey — the omit is unconditional. Sensitive is a separate signal
// the caller can use to decide whether to surface the key to the user.
func envEntrySensitive(_ EnvVar) bool {
	// Phase B e2e fills this in once we observe the API's sensitive-flag
	// field on EnvVar. Current EnvVar struct lacks a Sensitive flag; we
	// can either extend it OR query a separate endpoint. Decision deferred
	// to e2e observation.
	return false
}
