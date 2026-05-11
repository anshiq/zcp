package sync

import (
	"strings"
	"testing"
)

// Run-40 ENG-2 — SanitizeTimeline strips author-data leaks and
// optionally substitutes a plan-derived service count before
// TIMELINE.md enters the export tarball.

// TestSanitizeTimeline_ProjectID redacts the parenthetical
// `(id `XYZ`)` idiom emitted by the TIMELINE prompt. Pinned against
// the literal run-39 leak: project name + id "7HfLxoquTxiNEg1fD4Xo7w".
func TestSanitizeTimeline_ProjectID(t *testing.T) {
	t.Parallel()
	in := []byte("Project name: `zcprecipator-nestjs-showcase` (id `7HfLxoquTxiNEg1fD4Xo7w`).\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	if strings.Contains(out, "7HfLxoquTxiNEg1fD4Xo7w") {
		t.Errorf("project id leaked through sanitizer: %q", out)
	}
	if !strings.Contains(out, "(id `<project-id>`)") {
		t.Errorf("project id placeholder missing; got %q", out)
	}
}

// TestSanitizeTimeline_HostnameHash redacts the Zerops-generated
// subdomain pattern. The project-hash digits identify the author's
// project; the rest of the URL shape is preserved so porters still
// see the format they'll encounter on their own deploys.
func TestSanitizeTimeline_HostnameHash(t *testing.T) {
	t.Parallel()
	in := []byte("Access the dashboard at https://apidev-2304-3000.prg1.zerops.app/api/status.\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	if strings.Contains(out, "2304-3000") {
		t.Errorf("hostname hash leaked: %q", out)
	}
	if !strings.Contains(out, "apidev-<id>-3000.prg1.zerops.app") {
		t.Errorf("expected `apidev-<id>-3000.prg1.zerops.app`; got %q", out)
	}
}

// TestSanitizeTimeline_HostnameHash_MultipleHostsAndPorts covers the
// per-codebase fan-out: appdev, apistage, workerstage each render
// with their own hash + port.
func TestSanitizeTimeline_HostnameHash_MultipleHostsAndPorts(t *testing.T) {
	t.Parallel()
	in := []byte(`URLs:
  - apidev-2304-3000.prg1.zerops.app
  - appdev-2304-5173.prg1.zerops.app
  - workerstage-2304-9229.prg1.zerops.app
`)
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	for _, leaked := range []string{"-2304-3000", "-2304-5173", "-2304-9229"} {
		if strings.Contains(out, leaked) {
			t.Errorf("hostname hash %q leaked through sanitizer; got %q", leaked, out)
		}
	}
	if !strings.Contains(out, "apidev-<id>-3000") || !strings.Contains(out, "appdev-<id>-5173") || !strings.Contains(out, "workerstage-<id>-9229") {
		t.Errorf("expected all three host placeholders; got %q", out)
	}
}

// TestSanitizeTimeline_ZcprecipatorPath redacts the author's
// machine-side output-root path.
func TestSanitizeTimeline_ZcprecipatorPath(t *testing.T) {
	t.Parallel()
	in := []byte("Output root: /var/www/zcprecipator/nestjs-showcase/environments/.\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	if strings.Contains(out, "/var/www/zcprecipator/nestjs-showcase/") {
		t.Errorf("author output-root leaked: %q", out)
	}
	if !strings.Contains(out, "<output-root>/environments/") {
		t.Errorf("expected <output-root>/ placeholder; got %q", out)
	}
}

// TestSanitizeTimeline_UsersPath redacts macOS dev paths.
func TestSanitizeTimeline_UsersPath(t *testing.T) {
	t.Parallel()
	in := []byte("Author session captured under /Users/fxck/www/zcp/.\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	if strings.Contains(out, "/Users/fxck/") {
		t.Errorf("author home path leaked: %q", out)
	}
	if !strings.Contains(out, "<machine-path>/www/zcp/") {
		t.Errorf("expected <machine-path>/ placeholder; got %q", out)
	}
}

// TestSanitizeTimeline_ServiceCountSubstitution rewrites the
// agent-authored claim to the plan-derived count. Pinned against the
// run-39 miscount (TIMELINE said 14; real was 11).
func TestSanitizeTimeline_ServiceCountSubstitution(t *testing.T) {
	t.Parallel()
	in := []byte("`zerops_import` provisioned 14 services in a single batch.\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{ServiceCount: 11}))
	if !strings.Contains(out, "provisioned 11 services") {
		t.Errorf("expected count substitution to 11; got %q", out)
	}
	if strings.Contains(out, "provisioned 14 services") {
		t.Errorf("pre-substitution count still present: %q", out)
	}
}

// TestSanitizeTimeline_ServiceCount_ZeroSkipsSubstitution — when no
// plan is available (ServiceCount=0) the sanitizer leaves the count
// alone; only the always-on redactions fire.
func TestSanitizeTimeline_ServiceCount_ZeroSkipsSubstitution(t *testing.T) {
	t.Parallel()
	in := []byte("provisioned 14 services\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	if !strings.Contains(out, "14 services") {
		t.Errorf("count should be preserved when ServiceCount=0; got %q", out)
	}
}

// TestSanitizeTimeline_Idempotent — running the sanitizer twice
// produces the same output as running it once. Placeholders are
// chosen to not re-trigger their own patterns.
func TestSanitizeTimeline_Idempotent(t *testing.T) {
	t.Parallel()
	in := []byte(`Project name: ` + "`zcprecipator-nestjs-showcase`" + ` (id ` + "`7HfLxoquTxiNEg1fD4Xo7w`" + `).
Dashboard: apidev-2304-3000.prg1.zerops.app
Output root: /var/www/zcprecipator/nestjs-showcase/
Author cwd: /Users/fxck/www/zcp/
provisioned 14 services
`)
	once := SanitizeTimeline(in, SanitizeTimelineOpts{ServiceCount: 11})
	twice := SanitizeTimeline(once, SanitizeTimelineOpts{ServiceCount: 11})
	if string(once) != string(twice) {
		t.Errorf("sanitizer not idempotent.\nOnce:\n%s\nTwice:\n%s", once, twice)
	}
}

// TestSanitizeTimeline_FullRun39Fixture covers a synthetic excerpt of
// the run-39 TIMELINE.md leaks end-to-end. Every redaction must land
// in a single pass.
func TestSanitizeTimeline_FullRun39Fixture(t *testing.T) {
	t.Parallel()
	in := []byte(`# Run-39 TIMELINE

Recipe engine: zcprecipator3.
Output root: /var/www/zcprecipator/nestjs-showcase/.

## 1. Research
Parent recipe absent; outputRoot=/var/www/zcprecipator/nestjs-showcase/.

## 2. Provision
` + "`zerops_import` provisioned 14 services in a single batch." + `

## 6. Close
Project name: ` + "`zcprecipator-nestjs-showcase`" + ` (id ` + "`7HfLxoquTxiNEg1fD4Xo7w`" + `).
Dashboard: https://apidev-2304-3000.prg1.zerops.app/api/status
`)
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{ServiceCount: 11}))

	forbidden := []string{
		"7HfLxoquTxiNEg1fD4Xo7w",
		"apidev-2304-3000",
		"/var/www/zcprecipator/nestjs-showcase/",
		"provisioned 14 services",
	}
	for _, leak := range forbidden {
		if strings.Contains(out, leak) {
			t.Errorf("forbidden leak %q still in output:\n%s", leak, out)
		}
	}
	required := []string{
		"(id `<project-id>`)",
		"apidev-<id>-3000.prg1.zerops.app",
		"<output-root>/",
		"provisioned 11 services",
	}
	for _, want := range required {
		if !strings.Contains(out, want) {
			t.Errorf("expected placeholder %q missing from output:\n%s", want, out)
		}
	}
}

// TestSanitizeTimeline_NonZeropsHostUnchanged — the hostname-hash
// regex anchors on `.zerops.app`; external hostnames must pass
// through. Catches over-eager regex regressions.
func TestSanitizeTimeline_NonZeropsHostUnchanged(t *testing.T) {
	t.Parallel()
	in := []byte("Refer to https://nestjs.com/docs and https://example.com/api-2304-3000 for details.\n")
	out := string(SanitizeTimeline(in, SanitizeTimelineOpts{}))
	if !strings.Contains(out, "example.com/api-2304-3000") {
		t.Errorf("non-zerops host should pass through unchanged; got %q", out)
	}
	if !strings.Contains(out, "nestjs.com/docs") {
		t.Errorf("framework doc URL should pass through unchanged; got %q", out)
	}
}
