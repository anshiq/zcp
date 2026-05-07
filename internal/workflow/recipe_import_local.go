package workflow

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// LocalizeRecipeImportYAML transforms a container-shape recipe import
// YAML into local-mode shape. Two operations:
//
//  1. Drop services with `zeropsSetup: dev` — local mode replaces the
//     SSH-in dev runtime with the user's CWD; provisioning a Zerops
//     dev service would be redundant and break bootstrap checks.
//  2. Strip `buildFromGit:` from any remaining runtime service —
//     local-mode stage starts in READY_TO_DEPLOY (no upstream auto-
//     seed). The agent's first local deploy is the mandatory bootstrap
//     completion step that verifies the build pipeline end-to-end.
//
// Managed services (no zeropsSetup) pass through unchanged.
//
// Recipes already shaped for local mode (single runtime, no
// `zeropsSetup: dev` block — e.g. `nextjs-ssr-hello-world`) become
// no-ops: the dev-drop finds nothing and the buildFromGit-strip leaves
// them with what they declared. The transform is idempotent.
//
// Uses yaml.Node round-trip so comments and field ordering survive.
// Returns the input verbatim on parse failure with a wrapped error so
// callers can name the recipe in their diagnostics.
func LocalizeRecipeImportYAML(recipe string) (string, error) {
	if recipe == "" {
		return "", nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(recipe), &doc); err != nil {
		return "", fmt.Errorf("local-mode transform: recipe YAML parse: %w", err)
	}

	root := documentRoot(&doc)
	if root == nil {
		return "", errors.New("local-mode transform: empty document")
	}

	servicesNode := mappingValue(root, "services")
	if servicesNode == nil || servicesNode.Kind != yaml.SequenceNode {
		// No services to transform — return verbatim.
		out, err := yaml.Marshal(&doc)
		if err != nil {
			return "", fmt.Errorf("local-mode transform: marshal: %w", err)
		}
		return string(out), nil
	}

	// First pass: collect indices of services with zeropsSetup: dev for
	// removal. Reverse-apply so intermediate indices stay stable.
	var dropIndices []int
	for i, svc := range servicesNode.Content {
		if svc.Kind != yaml.MappingNode {
			continue
		}
		if mappingScalar(svc, "zeropsSetup") == recipeRoleDev {
			dropIndices = append(dropIndices, i)
		}
	}
	for i := len(dropIndices) - 1; i >= 0; i-- {
		idx := dropIndices[i]
		servicesNode.Content = append(servicesNode.Content[:idx], servicesNode.Content[idx+1:]...)
	}

	// Second pass: strip buildFromGit from any remaining runtime
	// service (those with a non-empty zeropsSetup).
	for _, svc := range servicesNode.Content {
		if svc.Kind != yaml.MappingNode {
			continue
		}
		if mappingScalar(svc, "zeropsSetup") == "" {
			// Managed service — pass through unchanged.
			continue
		}
		removeMappingKey(svc, "buildFromGit")
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", fmt.Errorf("local-mode transform: marshal: %w", err)
	}
	return string(out), nil
}

// removeMappingKey deletes the named key (and its value) from a yaml
// mapping node if present. No-op when the key is absent. Comments
// attached to other entries survive because Content is sliced
// pair-by-pair (key+value).
func removeMappingKey(mapNode *yaml.Node, key string) {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Kind == yaml.ScalarNode && mapNode.Content[i].Value == key {
			mapNode.Content = append(mapNode.Content[:i], mapNode.Content[i+2:]...)
			return
		}
	}
}
