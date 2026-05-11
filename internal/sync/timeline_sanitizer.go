package sync

import (
	"fmt"
	"regexp"
)

// Run-40 ENG-2 — TIMELINE.md sanitizer.
//
// The recipe agent free-authors TIMELINE.md from its session-log
// metadata, which carries the author's real Zerops project ID,
// hostname hashes, machine paths, and machine-path-anchored URLs.
// Run-39 shipped these verbatim to the porter-facing tarball:
// `7HfLxoquTxiNEg1fD4Xo7w` (project ID), `apidev-2304-3000.prg1.zerops.app`
// (hostname-with-project-hash), `/var/www/zcprecipator/nestjs-showcase/`
// (author machine path). The deliverable is meant to generalize across
// porters; embedding author data leaks identity and breaks the
// "recipe ships to any project" guarantee.
//
// SanitizeTimeline rewrites these patterns to anonymized placeholders
// before the file enters the export tarball. The on-disk TIMELINE.md
// at the recipe-author's location is not mutated — sanitization
// happens between read and tar-write so the user's local copy is
// preserved while the shipped artifact is clean.
//
// Service count substitution (S8 closure): when ServiceCount is
// non-zero the sanitizer replaces "provisioned N services" prose
// claims with the plan-derived count. Run-39 claimed "14 services"
// when 11 actually shipped — the agent miscounted free-text. The
// engine has the correct number; trust the plan.
//
// Diagnosed in plans/run-40-evidence-grounded-plan.md §"S2-2", §"S3-1",
// §"S3-2", §"S8".

// SanitizeTimelineOpts carries optional substitutions the sanitizer
// applies on top of the always-on redactions.
type SanitizeTimelineOpts struct {
	// ServiceCount, when non-zero, substitutes any "provisioned N
	// services" prose with the canonical count derived from
	// plan.Services + plan.Codebases.
	ServiceCount int
}

// SanitizeTimeline returns a copy of body with author-data leaks and
// engine-vocabulary redacted. Idempotent — running it twice yields
// the same output.
//
// Redactions, in order:
//  1. Project IDs in `(id `xxx`)` form — Zerops projects use 22-char
//     base62 identifiers; the parenthetical-id idiom is what the
//     TIMELINE prompt asks the agent to emit, so the regex anchors
//     on that exact shape rather than naked 22-char strings (which
//     would catch unrelated hashes).
//  2. Hostname-with-project-hash pattern `<host>(dev|stage)-<digits>-
//     <port>.<zone>.zerops.app`. The middle number is the Zerops
//     project hash; collapsing it to <id> keeps the URL shape porters
//     read but strips the author's identity.
//  3. Author machine paths under `/var/www/zcprecipator/<slug>/` —
//     the zcprecipator output-root convention. Replaced with
//     `<output-root>/`.
//  4. Author machine paths under `/Users/<name>/` — macOS dev paths.
//     Replaced with `<machine-path>/`.
//  5. Optional: service count substitution (see SanitizeTimelineOpts).
func SanitizeTimeline(body []byte, opts SanitizeTimelineOpts) []byte {
	out := string(body)

	out = projectIDRedactor.ReplaceAllString(out, "(id `<project-id>`)")
	out = hostnameHashRedactor.ReplaceAllString(out, "${1}<id>${3}${4}")
	out = zcprecipatorPathRedactor.ReplaceAllString(out, "<output-root>/")
	out = usersPathRedactor.ReplaceAllString(out, "<machine-path>/")

	if opts.ServiceCount > 0 {
		out = provisionedServicesRedactor.ReplaceAllString(out, fmt.Sprintf("provisioned %d services", opts.ServiceCount))
	}

	return []byte(out)
}

// projectIDRedactor matches the parenthetical project-id idiom from
// the TIMELINE prompt: `(id `<22 base62 chars>`)`. Some Zerops IDs use
// _ or - so the character class is base62 + URL-safe punctuation.
var projectIDRedactor = regexp.MustCompile("\\(id `[A-Za-z0-9_-]{20,28}`\\)")

// hostnameHashRedactor matches the Zerops generated-subdomain shape
// `<host>(dev|stage)-<project-hash>-<port>.<zone>.zerops.app`. Project
// hash is 3-5 numeric digits; zone is 3-8 alphanumerics.
//
// Capture groups:
//
//	1 — host + role suffix (e.g. "apidev-")
//	2 — project hash digits (redacted)
//	3 — "-<port>" (e.g. "-3000")
//	4 — ".<zone>.zerops.app"
var hostnameHashRedactor = regexp.MustCompile(`([a-z][a-z0-9-]+(?:dev|stage)-)(\d{3,5})(-\d{2,5})(\.[a-z0-9]+\.zerops\.app)`)

// zcprecipatorPathRedactor matches `/var/www/zcprecipator/<slug>/`
// (the zcprecipator engine's output-root convention). The slug
// segment is 2-64 chars of recipe-name shape (lowercase + hyphens).
var zcprecipatorPathRedactor = regexp.MustCompile(`/var/www/zcprecipator/[a-z][a-z0-9-]{1,63}/`)

// usersPathRedactor matches macOS dev paths `/Users/<name>/`. The
// name segment is 1-32 chars of unix-username shape.
var usersPathRedactor = regexp.MustCompile(`/Users/[a-zA-Z][a-zA-Z0-9._-]{0,31}/`)

// provisionedServicesRedactor matches the free-text service-count
// claim from the TIMELINE prompt's provision section: "provisioned N
// services" where N is a small integer. Run-39 wrote "14" when the
// real count was 11; the engine substitutes the plan-derived count.
var provisionedServicesRedactor = regexp.MustCompile(`\bprovisioned \d{1,3} services\b`)
