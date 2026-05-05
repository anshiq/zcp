package recipe

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenOrCreate_RefusesMountBaseAndAncestors pins F-28: OpenOrCreate
// must refuse outputRoot values that equal or are ancestors of the SSHFS
// mount base ("/var/www"). The mount base hosts dev codebase mounts
// (apidev/, appdev/, workerdev/) — recipe outputs at the mount base
// would collide with the porter's source tree.
func TestOpenOrCreate_RefusesMountBaseAndAncestors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		outputRoot string
	}{
		{name: "exact_mount_base", outputRoot: "/var/www"},
		{name: "trailing_slash", outputRoot: "/var/www/"},
		{name: "parent_var", outputRoot: "/var"},
		{name: "filesystem_root", outputRoot: "/"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := NewStore(t.TempDir())
			_, err := store.OpenOrCreate("synth-showcase", tc.outputRoot)
			if err == nil {
				t.Fatalf("OpenOrCreate(%q) returned nil err; expected refusal", tc.outputRoot)
			}
			msg := err.Error()
			// Error must name the slug + canonical shape so the agent can
			// pivot without re-reading the spec.
			if !strings.Contains(msg, "synth-showcase") {
				t.Errorf("error %q does not name slug", msg)
			}
			if !strings.Contains(msg, "/var/www/zcprecipator/synth-showcase") {
				t.Errorf("error %q does not name canonical outputRoot", msg)
			}
			if !strings.Contains(msg, "/var/www") {
				t.Errorf("error %q does not name mount base", msg)
			}
		})
	}
}

// TestOpenOrCreate_AcceptsMountBaseDescendant pins the canonical-shape
// path: outputRoot one level beneath the mount base is the supported
// happy path.
func TestOpenOrCreate_AcceptsMountBaseDescendant(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	out := filepath.Join(t.TempDir(), "var", "www", "zcprecipator", "synth-showcase")
	// Use a temp-dir-rooted descendant for actual filesystem creation;
	// the refusal logic only refuses paths AT or ABOVE recipeMountBase
	// ("/var/www"), and a descendant under any path is fine.
	sess, err := store.OpenOrCreate("synth-showcase", out)
	if err != nil {
		t.Fatalf("OpenOrCreate descendant: %v", err)
	}
	if sess.Slug != "synth-showcase" {
		t.Errorf("Slug = %q", sess.Slug)
	}
}

// TestOpenOrCreate_AcceptsCanonicalDescendantUnderMountBase verifies the
// refusal does NOT trip on the canonical /var/www/zcprecipator/<slug>/
// shape itself. Uses MkdirAll on a real path under tmp by prefixing,
// since /var/www/zcprecipator may not be writable in CI; the assertion
// is purely about the refusal path returning nil error.
func TestOpenOrCreate_AcceptsCanonicalDescendantUnderMountBase(t *testing.T) {
	t.Parallel()

	// Build a path that is a descendant of /var/www so the refusal logic
	// sees it as "below mount base, fine". Filesystem write goes to a
	// temp-rooted equivalent; we exercise the refusal logic via the
	// canonical-path checker directly to avoid root-write requirements.
	if err := refuseIfMountBaseAncestor("/var/www/zcprecipator/synth-showcase", "synth-showcase"); err != nil {
		t.Fatalf("refused canonical descendant: %v", err)
	}
}

// TestOpenOrCreate_AcceptsLookalikePaths guards against the substring-
// false-positive: paths whose first path-segment shares the "/var/www"
// prefix as a string but is a different directory (/var/www2,
// /var/www-other) MUST be accepted.
func TestOpenOrCreate_AcceptsLookalikePaths(t *testing.T) {
	t.Parallel()

	cases := []string{
		"/var/www2",
		"/var/www-other",
		"/var/wwwx/foo",
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			if err := refuseIfMountBaseAncestor(p, "synth-showcase"); err != nil {
				t.Errorf("refuseIfMountBaseAncestor(%q) returned %v; expected nil (not a real ancestor)", p, err)
			}
		})
	}
}

// TestOpenOrCreate_AcceptsUnrelatedPaths covers the regression-safety
// envelope: paths that have nothing to do with the mount base (typical
// test outputs, /tmp scratch dirs) must keep working.
func TestOpenOrCreate_AcceptsUnrelatedPaths(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	out := filepath.Join(t.TempDir(), "scratch", "synth-showcase")
	if _, err := store.OpenOrCreate("synth-showcase", out); err != nil {
		t.Fatalf("OpenOrCreate unrelated path: %v", err)
	}
}

// TestDispatch_StartRefusesMountBase pins the dispatch-layer surface:
// the engine error reaches the JSON response body so an agent that
// passes outputRoot=/var/www gets a structured refusal at action=start.
func TestDispatch_StartRefusesMountBase(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	res := dispatch(t.Context(), store, RecipeInput{
		Action: "start", Slug: "synth-showcase", OutputRoot: "/var/www",
	})
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if !strings.Contains(res.Error, "/var/www/zcprecipator/synth-showcase") {
		t.Errorf("dispatch error %q does not name canonical outputRoot", res.Error)
	}
}
