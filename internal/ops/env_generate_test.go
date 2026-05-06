package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeropsio/zcp/internal/platform"
)

//nolint:maintidx // single table-driven test, intentional broad coverage of the resolver paths
func TestEnvGenerateDotenv_ResolvesRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		zeropsYml    string
		hostname     string
		serviceEnvs  map[string][]platform.EnvVar
		projectEnvs  []platform.EnvVar
		wantVars     int
		wantServices int
		wantContains []string
		wantErr      string
	}{
		{
			name: "cross-service references resolved",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
        DB_PORT: ${db_port}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "hostname", Content: "db"},
					{ID: "e2", Key: "port", Content: "5432"},
				},
			},
			wantVars:     2,
			wantServices: 1,
			wantContains: []string{"DB_HOST=db", "DB_PORT=5432"},
		},
		{
			name: "project-level env vars appended",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "hostname", Content: "db"},
				},
			},
			projectEnvs: []platform.EnvVar{
				{ID: "pe1", Key: "APP_KEY", Content: "base64:secretkey"},
			},
			wantVars:     2, // 1 from zerops.yaml + 1 project
			wantServices: 1,
			wantContains: []string{"DB_HOST=db", "APP_KEY=base64:secretkey"},
		},
		{
			name: "static value passthrough",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        NODE_ENV: production
        DB_HOST: ${db_hostname}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "hostname", Content: "db"},
				},
			},
			wantVars:     2,
			wantServices: 1,
			wantContains: []string{"NODE_ENV=production", "DB_HOST=db"},
		},
		{
			// Compound expression: ${...} refs embedded inside a larger
			// string. The platform substitutes inline at deploy time; the
			// local .env must do the same so DATABASE_URL works against the
			// VPN'd managed service. Reproducer: behavioral eval suite
			// 20260506-145922 — agent wrote a Postgres URL with embedded
			// refs and got literal `${db_user}` in the .env, breaking
			// `npm start` against the VPN.
			name: "compound URL with multiple cross-service refs",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DATABASE_URL: postgresql://${db_user}:${db_password}@db:${db_port}/${db_dbName}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "user", Content: "appuser"},
					{ID: "e2", Key: "password", Content: "s3cret"},
					{ID: "e3", Key: "port", Content: "5432"},
					{ID: "e4", Key: "dbName", Content: "main"},
				},
			},
			wantVars:     1,
			wantServices: 1,
			wantContains: []string{"DATABASE_URL=postgresql://appuser:s3cret@db:5432/main"},
		},
		{
			// Mix: lone ref + compound ref + static value, all in one yaml.
			// Each variable must resolve independently.
			name: "mixed lone + compound + static",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
        DATABASE_URL: postgresql://${db_user}@${db_hostname}:${db_port}/main
        NODE_ENV: production
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "hostname", Content: "db"},
					{ID: "e2", Key: "user", Content: "u"},
					{ID: "e3", Key: "port", Content: "5432"},
				},
			},
			wantVars:     3,
			wantServices: 1,
			wantContains: []string{
				"DB_HOST=db",
				"DATABASE_URL=postgresql://u@db:5432/main",
				"NODE_ENV=production",
			},
		},
		{
			// Compound with one unresolved ref must error — partial
			// resolution would silently leave a literal `${...}` in the
			// .env, which is exactly the failure mode this whole fix
			// avoids. The error names the unresolved var so the agent
			// can fix the yaml.
			name: "compound with unresolved ref errors",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DATABASE_URL: postgresql://${db_user}:${db_typoed}@db/main
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "user", Content: "u"},
				},
			},
			wantErr: "could not resolve",
		},
		{
			// Recursive expansion: cross-service ref to a value that is
			// itself a sibling-template. Zerops's managed-service
			// `connectionString` follows this shape — `${db_connectionString}`
			// resolves to db.connectionString's value, which is the
			// template `postgresql://${user}:${password}@${hostname}:${port}`
			// where the lone refs are sibling lookups within db's own
			// env. To match deploy-time semantics in the consumer
			// container, the local .env must recurse: expand the cross-
			// service ref, then expand the resulting template against
			// the source service (db) for lone refs.
			name: "recursive: connectionString template fully expands",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DATABASE_URL: ${db_connectionString}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "user", Content: "myuser"},
					{ID: "e2", Key: "password", Content: "s3cret"},
					{ID: "e3", Key: "hostname", Content: "db"},
					{ID: "e4", Key: "port", Content: "5432"},
					{ID: "e5", Key: "connectionString", Content: "postgresql://${user}:${password}@${hostname}:${port}/main"},
				},
			},
			wantVars:     1,
			wantServices: 1,
			wantContains: []string{"DATABASE_URL=postgresql://myuser:s3cret@db:5432/main"},
		},
		{
			// Recursive expansion across a cross-service hop in a fetched
			// template. db's connectionString template embeds a
			// ${cache_url} cross-service ref (rare but a valid Zerops
			// pattern when one managed service composes another's URL).
			// The recursive expander must follow the chain.
			name: "recursive: nested cross-service ref inside fetched value",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_URL: ${db_composed}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "d1", Key: "composed", Content: "db@${cache_url}"},
				},
				"cache": {
					{ID: "c1", Key: "url", Content: "redis://cache:6379"},
				},
			},
			wantVars:     1,
			wantServices: 2,
			wantContains: []string{"DB_URL=db@redis://cache:6379"},
		},
		{
			// Cycle detection: db.x references db.y references db.x.
			// Without cycle detection, the recursive expander would
			// loop forever (or hit the depth limit and produce a
			// nonsense partial). Surface a specific cycle error so the
			// agent can fix the offending env var on the source side.
			name: "recursive: cycle detection errors",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        FOO: ${db_x}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "d1", Key: "x", Content: "${y}"},
					{ID: "d2", Key: "y", Content: "${x}"},
				},
			},
			wantErr: "circular",
		},
		{
			// Top-level lone refs (no underscore, no source-service context)
			// stay literal — they're either project-level vars (handled by
			// the GetProjectEnv pass) or runtime placeholders that the
			// deploy-time container resolves. The recursive expander must
			// NOT try to resolve them as cross-service refs.
			name: "recursive: top-level lone ref stays literal",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        URL: https://${hostname}:${port}/api
`,
			hostname:     "app",
			serviceEnvs:  map[string][]platform.EnvVar{},
			wantVars:     1,
			wantServices: 0,
			wantContains: []string{"URL=https://${hostname}:${port}/api"},
		},
		{
			name: "zerops.yaml envVariable takes precedence over project env",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        SHARED_KEY: custom_value
`,
			hostname: "app",
			projectEnvs: []platform.EnvVar{
				{ID: "pe1", Key: "SHARED_KEY", Content: "project_value"},
			},
			wantVars:     1,
			wantServices: 0,
			wantContains: []string{"SHARED_KEY=custom_value"},
		},
		{
			name: "missing service hostname",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
`,
			hostname: "",
			wantErr:  "serviceHostname is required",
		},
		{
			name: "no setup entry for hostname",
			zeropsYml: `zerops:
  - setup: other
    run:
      envVariables:
        FOO: bar
`,
			hostname: "app",
			wantErr:  "no setup entry",
		},
		{
			name: "no envVariables in entry",
			zeropsYml: `zerops:
  - setup: app
    build:
      base: nodejs@22
`,
			hostname: "app",
			wantErr:  "no run.envVariables",
		},
		{
			name: "top-level envVariables ignored (schema requires run.envVariables)",
			zeropsYml: `zerops:
  - setup: app
    envVariables:
      DB_HOST: ${db_hostname}
`,
			hostname: "app",
			wantErr:  "no run.envVariables",
		},
		{
			name: "unresolved reference",
			zeropsYml: `zerops:
  - setup: app
    run:
      envVariables:
        DB_HOST: ${db_hostname}
`,
			hostname: "app",
			serviceEnvs: map[string][]platform.EnvVar{
				"db": {
					{ID: "e1", Key: "port", Content: "5432"},
				},
			},
			wantErr: "could not resolve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(tmpDir, "zerops.yaml"), []byte(tt.zeropsYml), 0644); err != nil {
				t.Fatalf("write zerops.yaml: %v", err)
			}

			services := make([]platform.ServiceStack, 0, 1+len(tt.serviceEnvs))
			services = append(services, platform.ServiceStack{
				ID: "svc-app", Name: "app", ProjectID: "proj-1", Status: "RUNNING",
				ServiceStackTypeInfo: platform.ServiceTypeInfo{ServiceStackTypeVersionName: "nodejs@22"},
			})
			for svcName := range tt.serviceEnvs {
				services = append(services, platform.ServiceStack{
					ID: "svc-" + svcName, Name: svcName, ProjectID: "proj-1", Status: "RUNNING",
					ServiceStackTypeInfo: platform.ServiceTypeInfo{ServiceStackTypeVersionName: "postgresql@16"},
				})
			}

			mock := platform.NewMock().
				WithProject(&platform.Project{ID: "proj-1", Name: "test", Status: statusActive}).
				WithServices(services)
			for svcName, envs := range tt.serviceEnvs {
				mock = mock.WithServiceEnv("svc-"+svcName, envs)
			}
			if tt.projectEnvs != nil {
				mock = mock.WithProjectEnv(tt.projectEnvs)
			}

			result, err := EnvGenerateDotenv(context.Background(), mock, "proj-1", tt.hostname, tmpDir)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Variables != tt.wantVars {
				t.Errorf("variables = %d, want %d", result.Variables, tt.wantVars)
			}
			if result.Services != tt.wantServices {
				t.Errorf("services = %d, want %d", result.Services, tt.wantServices)
			}

			envContent, err := os.ReadFile(filepath.Join(tmpDir, ".env"))
			if err != nil {
				t.Fatalf("read .env: %v", err)
			}
			content := string(envContent)

			for _, want := range tt.wantContains {
				if !strings.Contains(content, want) {
					t.Errorf(".env should contain %q, got:\n%s", want, content)
				}
			}

			if !strings.Contains(content, "Generated by ZCP") {
				t.Error(".env should contain header comment")
			}
		})
	}
}
