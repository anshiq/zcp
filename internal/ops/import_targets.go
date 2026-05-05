package ops

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/zeropsio/zcp/internal/platform"
)

// IdentifyOverrideTargets returns the subset of import-YAML hostnames that
// already exist in the project — i.e. the services that override=true would
// REPLACE. Used by tools.RegisterImport to feed the diagnose-before-destruct
// gate (plan v4 §3.2): the gate only fires for services with failed
// appVersion history, and the candidate set is exactly the would-replace
// hostnames.
//
// Returns an empty slice (not nil) when the YAML lists hostnames but none
// match existing services. Errors propagate from the YAML parse and the
// ListServices call; YAML structural issues (missing services list,
// non-string hostnames) yield an empty result rather than an error so the
// downstream Import call can produce its own structured error.
func IdentifyOverrideTargets(
	ctx context.Context,
	client platform.Client,
	projectID string,
	content string,
	filePath string,
) ([]string, error) {
	yamlContent, err := resolveInput(content, filePath)
	if err != nil {
		return nil, err
	}

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		return nil, platform.NewPlatformError(
			platform.ErrInvalidImportYml,
			fmt.Sprintf("invalid YAML: %v", err),
			"Check YAML syntax",
		)
	}

	yamlHostnames := extractHostnames(doc)
	if len(yamlHostnames) == 0 {
		return []string{}, nil
	}

	existing, err := listExistingHostnames(ctx, client, projectID)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(yamlHostnames))
	for _, h := range yamlHostnames {
		if existing[h] {
			out = append(out, h)
		}
	}
	return out, nil
}
