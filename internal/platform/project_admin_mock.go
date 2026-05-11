package platform

import (
	"context"
	"sync"
)

// MockProjectAdminClient is the mock implementation of ProjectAdminClient
// used by unit + integration tests. Mirrors the configuration shape of
// platform.Mock but kept separate because the surfaces are disjoint.
//
// Configuration is via With* setters (option-pattern); state is captured
// for assertions via Captured* fields. Concurrent-safe.
type MockProjectAdminClient struct {
	mu sync.Mutex

	// configured returns
	importResult       *ImportResult
	importErr          error
	listServicesResult []ServiceStack
	listServicesErr    error
	serviceEnvKeys     map[string][]EnvKey // keyed by serviceID
	serviceEnvErr      error
	projectEnvKeys     map[string][]EnvKey // keyed by projectID
	projectEnvErr      error
	processResult      *Process
	processErr         error
	deleteResult       *Process
	deleteErr          error

	// state capture for assertions
	CapturedImportYAML    string
	CapturedImportOpts    CreateOpts
	CapturedDeleteProject string
	CapturedGetProcessID  string
	Closed                bool
}

// NewMockProjectAdminClient creates a fresh mock.
func NewMockProjectAdminClient() *MockProjectAdminClient {
	return &MockProjectAdminClient{
		serviceEnvKeys: make(map[string][]EnvKey),
		projectEnvKeys: make(map[string][]EnvKey),
	}
}

// WithImportResult configures the result returned by CreateAndImportProject.
func (m *MockProjectAdminClient) WithImportResult(r *ImportResult) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.importResult = r
	return m
}

// WithImportError configures the error returned by CreateAndImportProject.
func (m *MockProjectAdminClient) WithImportError(err error) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.importErr = err
	return m
}

// WithServices configures ListServices result.
func (m *MockProjectAdminClient) WithServices(services []ServiceStack) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listServicesResult = services
	return m
}

// WithServiceEnvKeys configures GetServiceEnvKeys result for a serviceID.
func (m *MockProjectAdminClient) WithServiceEnvKeys(serviceID string, keys []EnvKey) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceEnvKeys[serviceID] = keys
	return m
}

// WithProjectEnvKeys configures GetProjectEnvKeys result for a projectID.
func (m *MockProjectAdminClient) WithProjectEnvKeys(projectID string, keys []EnvKey) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectEnvKeys[projectID] = keys
	return m
}

// WithProcess configures GetProcess result.
func (m *MockProjectAdminClient) WithProcess(p *Process) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processResult = p
	return m
}

// WithDeleteResult configures DeleteProject result.
func (m *MockProjectAdminClient) WithDeleteResult(p *Process) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteResult = p
	return m
}

// WithDeleteError configures DeleteProject error.
func (m *MockProjectAdminClient) WithDeleteError(err error) *MockProjectAdminClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteErr = err
	return m
}

// CreateAndImportProject implements ProjectAdminClient.
func (m *MockProjectAdminClient) CreateAndImportProject(_ context.Context, yaml string, opts CreateOpts) (*ImportResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, ErrClientClosed
	}
	m.CapturedImportYAML = yaml
	m.CapturedImportOpts = opts
	if m.importErr != nil {
		return nil, m.importErr
	}
	return m.importResult, nil
}

// ListServices implements ProjectAdminClient.
func (m *MockProjectAdminClient) ListServices(_ context.Context, _ string) ([]ServiceStack, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, ErrClientClosed
	}
	if m.listServicesErr != nil {
		return nil, m.listServicesErr
	}
	return m.listServicesResult, nil
}

// GetServiceEnvKeys implements ProjectAdminClient.
func (m *MockProjectAdminClient) GetServiceEnvKeys(_ context.Context, serviceID string) ([]EnvKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, ErrClientClosed
	}
	if m.serviceEnvErr != nil {
		return nil, m.serviceEnvErr
	}
	return m.serviceEnvKeys[serviceID], nil
}

// GetProjectEnvKeys implements ProjectAdminClient.
func (m *MockProjectAdminClient) GetProjectEnvKeys(_ context.Context, projectID string) ([]EnvKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, ErrClientClosed
	}
	if m.projectEnvErr != nil {
		return nil, m.projectEnvErr
	}
	return m.projectEnvKeys[projectID], nil
}

// GetProcess implements ProjectAdminClient.
func (m *MockProjectAdminClient) GetProcess(_ context.Context, processID string) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, ErrClientClosed
	}
	m.CapturedGetProcessID = processID
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResult, nil
}

// DeleteProject implements ProjectAdminClient.
func (m *MockProjectAdminClient) DeleteProject(_ context.Context, projectID string) (*Process, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, ErrClientClosed
	}
	m.CapturedDeleteProject = projectID
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return m.deleteResult, nil
}

// Close implements ProjectAdminClient. After Close, all method calls return
// ErrClientClosed — matches the real client's contract.
func (m *MockProjectAdminClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Closed = true
}
