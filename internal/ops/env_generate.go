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

// refPatternAny matches any `${name}` placeholder. The interpreter
// classifies based on the body's shape AND the resolution context:
//
//   - Body contains `_` → cross-service `${host_var}`. Resolved by
//     fetching `host`'s env vars and looking up `var`. Always tried,
//     regardless of whether we're at top level or inside a recursive
//     expansion.
//   - Body has no `_` AND we're inside a source-service context (recursing
//     through a fetched value) → sibling lookup against the source
//     service's own env vars. This matches Zerops's deploy-time
//     semantics for templates like `connectionString =
//     postgresql://${user}:${password}@${hostname}:${port}` where the
//     lone refs resolve against the source service's siblings.
//   - Body has no `_` AND we're at top level (yaml run.envVariables) →
//     left literal. Project-level vars get appended later via
//     GetProjectEnv; runtime-only placeholders (`${zeropsSubdomainHost}`)
//     resolve at deploy time inside the container.
var refPatternAny = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)

// maxRefExpansionDepth caps recursive expansion regardless of cycle
// detection — a defensive bound for pathological chains of length N
// where each ref is unique (cycle detection wouldn't fire). 16 levels
// is far past any realistic Zerops env-var template chain.
const maxRefExpansionDepth = 16

// refExpander resolves `${...}` placeholders against the live API.
// cache is shared across all expandRefs calls within one
// EnvGenerateDotenv invocation: a single ListServices and one
// GetServiceEnv per touched service, regardless of how many refs
// reference the service or how deeply they recurse.
type refExpander struct {
	client    platform.Client
	projectID string
	cache     map[string][]platform.EnvVar
}

// expandRefs walks `value` and substitutes resolvable `${...}` refs.
// sourceService is "" at top level (yaml `run.envVariables`); inside a
// recursive call it names the service whose value we're currently
// expanding (lone refs there resolve against that service's siblings).
//
// visited carries the chain of `host.var` keys already resolved on this
// recursion path; re-encountering one is a cycle. Each recursive call
// gets its own copy so siblings at the same level don't false-positive.
//
// Returns:
//   - expanded string (with resolvable refs substituted, unresolved refs
//     left as their original `${...}` literal so partial-resolution is
//     debuggable),
//   - count of unresolved refs the caller can aggregate for error
//     messaging (0 means full success),
//   - infrastructure / cycle errors that abort the whole operation.
func (r *refExpander) expandRefs(ctx context.Context, value, sourceService string, visited map[string]bool, depth int) (string, int, error) {
	if depth > maxRefExpansionDepth {
		return "", 0, fmt.Errorf("ref expansion depth exceeded (>%d) at %q", maxRefExpansionDepth, value)
	}
	matches := refPatternAny.FindAllStringSubmatchIndex(value, -1)
	if len(matches) == 0 {
		return value, 0, nil
	}
	var sb strings.Builder
	unresolved := 0
	last := 0
	for _, m := range matches {
		sb.WriteString(value[last:m[0]])
		refBody := value[m[2]:m[3]]
		original := value[m[0]:m[1]]

		var svcHost, varName string
		switch {
		case strings.Contains(refBody, "_"):
			svcHost, varName, _ = strings.Cut(refBody, "_")
		case sourceService != "":
			svcHost, varName = sourceService, refBody
		default:
			// Top-level lone ref — leave literal.
			sb.WriteString(original)
			last = m[1]
			continue
		}

		key := svcHost + "." + varName
		if visited[key] {
			return "", 0, fmt.Errorf("circular reference at ${%s}: chain re-enters %s", refBody, key)
		}

		if _, cached := r.cache[svcHost]; !cached {
			services, err := r.client.ListServices(ctx, r.projectID)
			if err != nil {
				return "", 0, fmt.Errorf("list services: %w", err)
			}
			svc, err := FindService(services, svcHost)
			if err != nil {
				if depth == 0 {
					// Top-level typo / missing service — surface the
					// specific error the user can act on.
					return "", 0, err
				}
				// Inside a recursive expansion: maybe the fetched
				// template references a host that's part of platform
				// vocabulary ZCP doesn't model. Leave literal so .env
				// keeps the original ref visible.
				sb.WriteString(original)
				last = m[1]
				unresolved++
				continue
			}
			envs, err := r.client.GetServiceEnv(ctx, svc.ID)
			if err != nil {
				return "", 0, fmt.Errorf("fetch env vars for %s: %w", svcHost, err)
			}
			r.cache[svcHost] = envs
		}

		rawVal := findEnvValue(r.cache[svcHost], varName)
		if rawVal == "" {
			sb.WriteString(original)
			last = m[1]
			unresolved++
			continue
		}

		nextVisited := make(map[string]bool, len(visited)+1)
		for k := range visited {
			nextVisited[k] = true
		}
		nextVisited[key] = true
		expanded, sub, err := r.expandRefs(ctx, rawVal, svcHost, nextVisited, depth+1)
		if err != nil {
			return "", 0, err
		}
		unresolved += sub
		sb.WriteString(expanded)
		last = m[1]
	}
	sb.WriteString(value[last:])
	return sb.String(), unresolved, nil
}

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

	// Resolve run.envVariables. The expander handles cross-service refs
	// (${host_var}) AND recursively resolves through fetched values —
	// e.g. `${db_connectionString}` resolves to db's connectionString
	// value, which is itself the template
	// `postgresql://${user}:${password}@${hostname}:${port}` where the
	// lone refs are siblings within db's own env. Recursion matches
	// Zerops's deploy-time semantics so the local .env is equivalent to
	// what lands inside the runtime container.
	serviceEnvCache := make(map[string][]platform.EnvVar)
	expander := &refExpander{client: client, projectID: projectID, cache: serviceEnvCache}
	resolved := make(map[string]string, len(entry.Run.EnvVariables))
	var unresolvedKeys []string

	for envName, rawValue := range entry.Run.EnvVariables {
		expanded, unresolvedCount, expErr := expander.expandRefs(ctx, rawValue, "", map[string]bool{}, 0)
		if expErr != nil {
			return nil, expErr
		}
		if unresolvedCount > 0 {
			unresolvedKeys = append(unresolvedKeys, envName)
		} else {
			resolved[envName] = expanded
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
