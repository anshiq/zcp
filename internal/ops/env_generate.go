package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zeropsio/zcp/internal/platform"
)

// EnvDotenvResult contains the result of .env file generation.
//
// VPNHint is populated when at least one managed service referenced in
// the resolved env vars looked unreachable — local dev then almost
// certainly needs `zcli vpn up`. The probe is best-effort and non-
// blocking: even if probes fail, the .env file still lands. An empty
// hint means every probed host was reachable (or no probe happened
// because ServiceStack carried no port info).
type EnvDotenvResult struct {
	Path      string `json:"path"`
	Services  int    `json:"services"`
	Variables int    `json:"variables"`
	VPNHint   string `json:"vpnHint,omitempty"`
}

// refPatternInline matches `${hostname_varName}` cross-service references
// anywhere in a string. The platform substitutes inline at deploy time
// (a Postgres URL like `postgresql://${db_user}:${db_password}@db:${db_port}/${db_dbName}`
// gets every embedded ref expanded), so the local .env must do the same to
// stay equivalent to the runtime container. The leading underscore-bearing
// shape is what filters cross-service refs from project-level / runtime
// placeholders (`${zeropsSubdomainHost}`, `${hostname}`, `${port}`) which
// are NOT cross-service references and stay as-is here — project-level
// values are appended later via GetProjectEnv.
var refPatternInline = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9]*_[a-zA-Z_][a-zA-Z0-9_]*)\}`)

// EnvGenerateDotenv reads zerops.yaml `run.envVariables` for a service, resolves ${hostname_varName}
// cross-service references by fetching actual values from managed services, appends project-level
// env vars, and writes a .env file. The LLM never sees secret values — ZCP resolves internally.
// `run.envVariables` is the canonical schema location; the JSON schema rejects envVariables at the
// setup-entry top level (only build.envVariables / run.envVariables are valid).
func EnvGenerateDotenv(
	ctx context.Context,
	client platform.Client,
	projectID string,
	hostname string,
	workingDir string,
) (*EnvDotenvResult, error) {
	if hostname == "" {
		return nil, platform.NewPlatformError(platform.ErrInvalidParameter,
			"serviceHostname is required for generate-dotenv",
			"Specify which service's zerops.yaml envVariables to resolve")
	}

	if workingDir == "" {
		workingDir = "."
	}

	doc, err := ParseZeropsYml(workingDir)
	if err != nil {
		return nil, fmt.Errorf("generate-dotenv: %w", err)
	}

	entry := doc.FindEntry(hostname)
	if entry == nil {
		return nil, platform.NewPlatformError(platform.ErrInvalidParameter,
			fmt.Sprintf("no setup entry for %q in zerops.yaml", hostname),
			"Check that zerops.yaml has a setup: entry matching the service hostname")
	}

	if len(entry.Run.EnvVariables) == 0 {
		return nil, platform.NewPlatformError(platform.ErrInvalidParameter,
			fmt.Sprintf("setup %q has no run.envVariables in zerops.yaml", hostname),
			"Add run.envVariables to zerops.yaml first")
	}

	// Resolve run.envVariables — cross-service refs only.
	serviceEnvCache := make(map[string][]platform.EnvVar)
	resolved := make(map[string]string, len(entry.Run.EnvVariables))
	var unresolvedKeys []string

	for envName, rawValue := range entry.Run.EnvVariables {
		matches := refPatternInline.FindAllStringSubmatchIndex(rawValue, -1)
		if len(matches) == 0 {
			// No cross-service refs — pass value through unchanged.
			resolved[envName] = rawValue
			continue
		}

		// Inline-expand every ${hostname_varName} match. last walks the
		// rawValue, sb accumulates the spliced output. Each match index
		// triple is [fullStart, fullEnd, groupStart, groupEnd]; the group
		// is the host_var body without the surrounding ${}.
		var sb strings.Builder
		hadUnresolved := false
		last := 0
		for _, m := range matches {
			sb.WriteString(rawValue[last:m[0]])
			refBody := rawValue[m[2]:m[3]]
			svcHostname, varName, _ := strings.Cut(refBody, "_")

			if _, cached := serviceEnvCache[svcHostname]; !cached {
				services, listErr := client.ListServices(ctx, projectID)
				if listErr != nil {
					return nil, fmt.Errorf("list services: %w", listErr)
				}
				svc, resolveErr := FindService(services, svcHostname)
				if resolveErr != nil {
					return nil, resolveErr
				}
				envs, getErr := client.GetServiceEnv(ctx, svc.ID)
				if getErr != nil {
					return nil, fmt.Errorf("fetch env vars for %s: %w", svcHostname, getErr)
				}
				serviceEnvCache[svcHostname] = envs
			}

			val := findEnvValue(serviceEnvCache[svcHostname], varName)
			if val == "" {
				hadUnresolved = true
				// Keep the original `${...}` literal in the buffer so a
				// partial-resolution diff is at least debuggable, but the
				// envName goes on the unresolved list and the whole call
				// fails at the outer aggregate check below.
				sb.WriteString(rawValue[m[0]:m[1]])
			} else {
				sb.WriteString(val)
			}
			last = m[1]
		}
		sb.WriteString(rawValue[last:])

		if hadUnresolved {
			unresolvedKeys = append(unresolvedKeys, envName)
		} else {
			resolved[envName] = sb.String()
		}
	}

	if len(unresolvedKeys) > 0 {
		return nil, platform.NewPlatformError(platform.ErrInvalidParameter,
			fmt.Sprintf("could not resolve env vars: %s", strings.Join(unresolvedKeys, ", ")),
			"Check that referenced services are running and have the expected env var keys")
	}

	// Append project-level env vars (auto-injected in containers, needed in .env for local dev).
	projectEnvs, err := client.GetProjectEnv(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("fetch project env vars: %w", err)
	}
	for _, pe := range projectEnvs {
		if _, exists := resolved[pe.Key]; !exists {
			resolved[pe.Key] = pe.Content
		}
	}

	// Write .env file.
	var sb strings.Builder
	sb.WriteString("# Generated by ZCP from zerops.yaml envVariables\n")
	sb.WriteString("# WARNING: Contains secrets. Do not commit.\n\n")
	for envName, val := range resolved {
		sb.WriteString(envName)
		sb.WriteByte('=')
		sb.WriteString(val)
		sb.WriteByte('\n')
	}

	envPath := filepath.Join(workingDir, ".env")
	if err := os.WriteFile(envPath, []byte(sb.String()), 0600); err != nil {
		return nil, fmt.Errorf("write .env: %w", err)
	}

	result := &EnvDotenvResult{
		Path:      envPath,
		Services:  len(serviceEnvCache),
		Variables: len(resolved),
	}

	// Best-effort VPN probe. Only runs when we cross-referenced managed
	// services (serviceEnvCache has entries); a local .env with only
	// project-level vars doesn't need VPN to work on dev. Services are
	// probed via their platform-listed TCP ports. A single unreachable
	// host triggers the hint — users typically run one `zcli vpn up`
	// per project, not per service.
	if len(serviceEnvCache) > 0 {
		if hint := probeManagedServicesForVPN(ctx, client, projectID, serviceEnvCache); hint != "" {
			result.VPNHint = hint
		}
	}
	return result, nil
}

// probeManagedServicesForVPN attempts one TCP dial per referenced
// managed service. Returns a hint string when any probe fails, empty
// when all succeed (or no port info is available to probe against).
func probeManagedServicesForVPN(ctx context.Context, client platform.Client, projectID string, cache map[string][]platform.EnvVar) string {
	services, err := client.ListServices(ctx, projectID)
	if err != nil {
		return ""
	}
	serviceByName := make(map[string]platform.ServiceStack, len(services))
	for _, s := range services {
		serviceByName[s.Name] = s
	}
	for host := range cache {
		svc, ok := serviceByName[host]
		if !ok || len(svc.Ports) == 0 {
			continue
		}
		if !ProbeManagedReachable(ctx, host, svc.Ports[0].Port) {
			return fmt.Sprintf("Managed service %q not reachable on port %d — run `zcli vpn up %s` and retry local dev.", host, svc.Ports[0].Port, projectID)
		}
	}
	return ""
}

func findEnvValue(envs []platform.EnvVar, key string) string {
	for _, e := range envs {
		if e.Key == key {
			return e.Content
		}
	}
	return ""
}
