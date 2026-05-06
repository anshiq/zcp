package ops

import (
	"regexp"
	"strings"

	"github.com/zeropsio/zcp/internal/platform"
)

// envRefPattern matches a single ${...} placeholder. The body must start
// with a letter so YAML literal `$$dollar`, partial `${incomplete`, and
// numeric leaders `${1bad}` are not picked up.
var envRefPattern = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)

// EnvRefMatch describes one ${...} reference inside a value. Start/End are
// indices into the source string so callers that rebuild the string in
// place (refExpander) can reuse them; callers that only need the ref body
// (ValidateEnvReferences) can ignore them.
type EnvRefMatch struct {
	Raw   string
	Body  string
	Start int
	End   int
}

// FindEnvRefs returns every ${...} reference in s, in source order.
func FindEnvRefs(s string) []EnvRefMatch {
	idx := envRefPattern.FindAllStringSubmatchIndex(s, -1)
	out := make([]EnvRefMatch, len(idx))
	for i, m := range idx {
		out[i] = EnvRefMatch{
			Raw:   s[m[0]:m[1]],
			Body:  s[m[2]:m[3]],
			Start: m[0],
			End:   m[1],
		}
	}
	return out
}

// EnvRefClassifier resolves ${...} ref bodies against the live service
// hostnames of a project.
//
// Zerops env-var ref syntax uses an underscore between hostname and var
// name (`${db_connectionString}` → service "db", var "connectionString").
// When the hostname itself contains a dash (`my-db`) the wire form swaps
// dashes for underscores: `${my_db_port}`. Splitting on the first
// underscore would parse this as host="my", var="db_port" — wrong. The
// classifier matches the longest live-hostname prefix instead, so the
// same syntax works whether the hostname is `db`, `my-db`, or
// `my-very-long-name`.
//
// Bodies that match no live hostname are returned with crossService=false
// so the caller decides how to treat the ref. EnvGenerateDotenv leaves
// top-level lone refs literal (project-level vars / runtime placeholders
// the platform resolves at deploy time) and treats lone refs inside a
// recursive expansion as siblings of the source service. ValidateEnvReferences
// ignores lone refs entirely (a project-level var that escapes the
// validator surfaces as a deploy-time error, not a preflight one).
type EnvRefClassifier struct {
	// hostsByCanonical maps the underscore-canonical form of each live
	// hostname back to its wire form. So a service "my-db" registers as
	// hostsByCanonical["my_db"] = "my-db".
	hostsByCanonical map[string]string
}

// NewEnvRefClassifier builds a classifier from a slice of live service
// stacks. Hostnames containing dashes register their underscore-canonical
// form as well so refs like ${my_db_port} resolve.
func NewEnvRefClassifier(services []platform.ServiceStack) *EnvRefClassifier {
	c := &EnvRefClassifier{
		hostsByCanonical: make(map[string]string, len(services)),
	}
	for _, s := range services {
		if s.Name == "" {
			continue
		}
		canonical := strings.ReplaceAll(s.Name, "-", "_")
		c.hostsByCanonical[canonical] = s.Name
	}
	return c
}

// Classify returns (hostname, varName, true) when body starts with a
// live-hostname canonical prefix followed by an underscore and a non-
// empty var name. Returns ("", "", false) for lone refs (no matching
// prefix, or matching prefix with empty var part).
//
// Longest-match wins: with hostnames "a" and "a-b" both live, body
// "a_b_c" classifies as ("a-b", "c", true) — not ("a", "b_c", true).
func (c *EnvRefClassifier) Classify(body string) (string, string, bool) {
	if c == nil || len(c.hostsByCanonical) == 0 {
		return "", "", false
	}
	var bestCanonical string
	for canonical := range c.hostsByCanonical {
		if !strings.HasPrefix(body, canonical+"_") {
			continue
		}
		if len(canonical) > len(bestCanonical) {
			bestCanonical = canonical
		}
	}
	if bestCanonical == "" {
		return "", "", false
	}
	varName := body[len(bestCanonical)+1:]
	if varName == "" {
		return "", "", false
	}
	return c.hostsByCanonical[bestCanonical], varName, true
}
