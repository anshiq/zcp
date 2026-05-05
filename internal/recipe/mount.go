package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// recipeMountBase is the SSHFS mount base in the ZCP container — the
// directory that hosts per-codebase dev mounts (`apidev/`, `appdev/`,
// `workerdev/`). Recipe outputs MUST nest under it (canonical shape:
// `<recipeMountBase>/zcprecipator/<slug>/`); writing AT the mount base
// or any ancestor would shadow the porter's source tree with stitched
// recipe artifacts.
//
// This duplicates `internal/workflow.recipeMountBase` and
// `internal/ops.mountBase` deliberately — `internal/recipe/` is a peer
// of `internal/workflow/` per the layered-architecture contract in
// CLAUDE.md, so cross-imports between peers are forbidden. The constant
// is small enough to re-declare; the alternative (promoting it to
// `internal/topology/`) would over-elevate a filesystem path that has
// no platform-vocabulary meaning.
const recipeMountBase = "/var/www"

// refuseIfMountBaseAncestor returns a refusal error when the cleaned
// outputRoot equals the SSHFS mount base or is an ancestor of it.
// Substring lookalikes (`/var/www2`, `/var/www-other`) MUST NOT trip
// this check — the trailing-separator comparison is what guards them.
//
// Pins F-28 (run-25 ANALYSIS.md §Axis Z): a recipe deliverable written
// to `/var/www/` collapsed stitched output (tier folders, root README,
// facts.jsonl, plan.json, RAW_SESSION_LOG, .refinement-closed marker)
// into the same tree as the SSHFS-mounted dev codebases.
func refuseIfMountBaseAncestor(outputRoot, slug string) error {
	cleaned := filepath.Clean(outputRoot)
	sep := string(os.PathSeparator)
	// Equal-or-ancestor check: cleaned is an ancestor of (or equals)
	// recipeMountBase iff prefixing both with a trailing separator
	// makes recipeMountBase start with cleaned. The trailing-separator
	// trick is what differentiates "/var/www" from "/var/www2" —
	// without it, HasPrefix("/var/www2", "/var/www") would false-trip.
	//
	// filepath.Clean("/") returns "/", so cleaned+sep would be "//"
	// and the prefix test misses the root case. Handle the root path
	// explicitly: filesystem root is by definition an ancestor of
	// every absolute path, refuse it directly.
	mountBaseWithSep := recipeMountBase + sep
	cleanedWithSep := cleaned + sep
	if cleaned != sep && !strings.HasPrefix(mountBaseWithSep, cleanedWithSep) {
		return nil
	}
	canonical := filepath.Join(recipeMountBase, "zcprecipator", slug)
	return fmt.Errorf(
		"outputRoot %q collides with the SSHFS mount base %q — recipe outputs must nest one level down so they don't shadow dev codebases. Pass outputRoot=%q (canonical: %s/zcprecipator/<slug>/)",
		outputRoot, recipeMountBase, canonical, recipeMountBase,
	)
}
