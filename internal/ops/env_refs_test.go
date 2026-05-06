package ops

import (
	"testing"

	"github.com/zeropsio/zcp/internal/platform"
)

func TestEnvRefClassifier_Classify_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		liveHosts   []string
		body        string
		wantHost    string
		wantVar     string
		wantIsCross bool
	}{
		{
			name:        "no live hosts treats every body as lone",
			liveHosts:   nil,
			body:        "db_port",
			wantIsCross: false,
		},
		{
			name:        "single-word hostname classified",
			liveHosts:   []string{"db"},
			body:        "db_port",
			wantHost:    "db",
			wantVar:     "port",
			wantIsCross: true,
		},
		{
			name:        "dash hostname matched via underscore canonical form",
			liveHosts:   []string{"my-db"},
			body:        "my_db_port",
			wantHost:    "my-db",
			wantVar:     "port",
			wantIsCross: true,
		},
		{
			name:        "longest hostname prefix wins over shorter prefix",
			liveHosts:   []string{"a", "a-b"},
			body:        "a_b_c",
			wantHost:    "a-b",
			wantVar:     "c",
			wantIsCross: true,
		},
		{
			name:        "body without matching prefix stays lone",
			liveHosts:   []string{"db"},
			body:        "STAGE_API_URL",
			wantIsCross: false,
		},
		{
			name:        "host-only body without underscore stays lone",
			liveHosts:   []string{"db"},
			body:        "db",
			wantIsCross: false,
		},
		{
			name:        "trailing underscore with empty var stays lone",
			liveHosts:   []string{"db"},
			body:        "db_",
			wantIsCross: false,
		},
		{
			name:        "compound variable name kept intact when host matches",
			liveHosts:   []string{"db"},
			body:        "db_connection_string",
			wantHost:    "db",
			wantVar:     "connection_string",
			wantIsCross: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			services := make([]platform.ServiceStack, len(tt.liveHosts))
			for i, h := range tt.liveHosts {
				services[i] = platform.ServiceStack{Name: h}
			}
			c := NewEnvRefClassifier(services)
			gotHost, gotVar, gotIsCross := c.Classify(tt.body)
			if gotIsCross != tt.wantIsCross {
				t.Fatalf("isCross = %v, want %v", gotIsCross, tt.wantIsCross)
			}
			if gotHost != tt.wantHost {
				t.Errorf("host = %q, want %q", gotHost, tt.wantHost)
			}
			if gotVar != tt.wantVar {
				t.Errorf("var = %q, want %q", gotVar, tt.wantVar)
			}
		})
	}
}

func TestEnvRefClassifier_NilReceiver_ReturnsLone(t *testing.T) {
	t.Parallel()

	var c *EnvRefClassifier
	host, varName, isCross := c.Classify("db_port")
	if isCross {
		t.Fatalf("nil classifier should never report cross-service: got (%q, %q, %v)", host, varName, isCross)
	}
}

func TestFindEnvRefs_ReturnsRawAndBodyInOrder(t *testing.T) {
	t.Parallel()

	const s = "postgresql://${db_user}:${db_password}@${db_hostname}:${db_port}/main"
	matches := FindEnvRefs(s)
	if len(matches) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(matches))
	}
	if matches[0].Raw != "${db_user}" || matches[0].Body != "db_user" {
		t.Errorf("match[0] = (%q, %q), want (${db_user}, db_user)", matches[0].Raw, matches[0].Body)
	}
	if matches[3].Raw != "${db_port}" || matches[3].Body != "db_port" {
		t.Errorf("match[3] = (%q, %q), want (${db_port}, db_port)", matches[3].Raw, matches[3].Body)
	}
}

func TestFindEnvRefs_IgnoresMalformedAndEscaped(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"$$dollar":          0,
		"$notaref":          0,
		"${incomplete":      0,
		"${1bad}":           0,
		"${}":               0,
		"plain string":      0,
		"${ok} ${db_x}":     2,
		"${db_x} and ${db}": 2,
	}
	for input, want := range tests {
		got := len(FindEnvRefs(input))
		if got != want {
			t.Errorf("FindEnvRefs(%q) returned %d matches, want %d", input, got, want)
		}
	}
}
