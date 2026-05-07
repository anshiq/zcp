package platform

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/zeropsio/zerops-go/apiError"
)

func TestValidateZeropsYaml_InputValidation(t *testing.T) {
	t.Parallel()

	base := ValidateZeropsYamlInput{
		ServiceStackTypeID:      "Ki1HVoEvSruMfR2J8QNCRA",
		ServiceStackTypeVersion: "nodejs@22",
		ServiceStackName:        "app",
		ZeropsYaml:              "zerops:\n  - setup: app\n",
		Operation:               ValidationOperationBuildAndDeploy,
	}

	// Each case strips one required field and asserts ValidateZeropsYaml
	// returns ErrInvalidParameter before the SDK handler is ever called.
	// A nil ZeropsClient is passed because the input check must short-
	// circuit; if it didn't, the nil handler would panic and the test
	// would fail loudly instead of silently passing.
	tests := []struct {
		name  string
		patch func(in *ValidateZeropsYamlInput)
	}{
		{"missing_type_id", func(in *ValidateZeropsYamlInput) { in.ServiceStackTypeID = "" }},
		{"missing_type_version", func(in *ValidateZeropsYamlInput) { in.ServiceStackTypeVersion = "" }},
		{"missing_setup_name", func(in *ValidateZeropsYamlInput) { in.ServiceStackName = "" }},
		{"missing_yaml", func(in *ValidateZeropsYamlInput) { in.ZeropsYaml = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := base
			tt.patch(&in)
			// Use a zero-value ZeropsClient — input checks short-circuit
			// before any SDK call.
			var z ZeropsClient
			err := z.ValidateZeropsYaml(context.Background(), in)
			if err == nil {
				t.Fatal("expected input validation error, got nil")
			}
			pe, ok := err.(*PlatformError)
			if !ok {
				t.Fatalf("expected *PlatformError, got %T: %v", err, err)
			}
			if pe.Code != ErrInvalidParameter {
				t.Errorf("Code = %q, want %q", pe.Code, ErrInvalidParameter)
			}
		})
	}
}

func TestReclassifyValidationError(t *testing.T) {
	t.Parallel()

	mkFromAPI := func(code string, meta []APIMetaItem) error {
		apiErr := apiError.Error{
			HttpStatusCode: 400,
			ErrorCode:      code,
			Message:        "Invalid parameter provided.",
		}
		// Use the real mapAPIError so test mirrors production path.
		pe, ok := mapAPIError(apiErr, "zeropsYaml").(*PlatformError)
		if !ok {
			t.Fatalf("mapAPIError did not return *PlatformError")
		}
		pe.APIMeta = meta // stand in for live API meta
		// In production, apiErr.Meta carries the same items so mapAPIError's
		// 4xx-with-meta branch runs formatAPIMetaActionable and sets Suggestion
		// inline. The test fixture builds apiErr without Meta (the SDK shape
		// requires JsonRawMessage; populating it round-trips through JSON
		// unmarshal which adds noise to every test row), so apply the inline
		// expansion here to keep parity with what reclassifyValidationError
		// sees in production after the e2ec3651 migration.
		if len(meta) > 0 {
			pe.Suggestion = formatAPIMetaActionable(meta)
		}
		return pe
	}

	t.Run("zeropsYamlInvalidParameter_with_meta", func(t *testing.T) {
		t.Parallel()
		input := mkFromAPI("zeropsYamlInvalidParameter", []APIMetaItem{
			{
				Code:  "zeropsYamlInvalidParameter",
				Error: "Invalid parameter provided.",
				Metadata: map[string][]string{
					"build.base": {"unknown base nodejs@99"},
				},
			},
		})
		got := reclassifyValidationError(input)
		pe, ok := got.(*PlatformError)
		if !ok {
			t.Fatalf("expected *PlatformError, got %T", got)
		}
		if pe.Code != ErrInvalidZeropsYml {
			t.Errorf("Code = %q, want %q", pe.Code, ErrInvalidZeropsYml)
		}
		if pe.APICode != "zeropsYamlInvalidParameter" {
			t.Errorf("APICode = %q, want zeropsYamlInvalidParameter", pe.APICode)
		}
		// Suggestion preserves mapAPIError's inline expansion via
		// formatAPIMetaActionable. Pre-e2ec3651 reclassify overwrote
		// Suggestion with a "read apiMeta" pointer that depended on the
		// develop-api-error-meta defense atom (now retired). The
		// actionable inline shape replaces both — reclassify no longer
		// touches Suggestion.
		if strings.Contains(pe.Suggestion, "read apiMeta") {
			t.Errorf("Suggestion still uses pre-e2ec3651 pointer text: %q", pe.Suggestion)
		}
		if !strings.Contains(pe.Suggestion, "build.base") {
			t.Errorf("Suggestion missing inline field path: %q", pe.Suggestion)
		}
		if !strings.Contains(pe.Suggestion, "unknown base nodejs@99") {
			t.Errorf("Suggestion missing inline failure reason: %q", pe.Suggestion)
		}
		if !strings.Contains(pe.Suggestion, "Fix in YAML") {
			t.Errorf("Suggestion missing actionable retry hint: %q", pe.Suggestion)
		}
		// Meta must survive reclassification.
		wantMeta := []APIMetaItem{
			{
				Code:  "zeropsYamlInvalidParameter",
				Error: "Invalid parameter provided.",
				Metadata: map[string][]string{
					"build.base": {"unknown base nodejs@99"},
				},
			},
		}
		if !reflect.DeepEqual(pe.APIMeta, wantMeta) {
			t.Errorf("APIMeta lost during reclassify:\ngot  %+v\nwant %+v", pe.APIMeta, wantMeta)
		}
	})

	t.Run("errorList_with_multi_meta", func(t *testing.T) {
		t.Parallel()
		input := mkFromAPI("errorList", []APIMetaItem{
			{Code: "zeropsYamlInvalidParameter", Metadata: map[string][]string{"build.base": {"unknown"}}},
			{Code: "zeropsYamlInvalidParameter", Metadata: map[string][]string{"run.base": {"unknown"}}},
		})
		got := reclassifyValidationError(input)
		pe := got.(*PlatformError)
		if pe.Code != ErrInvalidZeropsYml {
			t.Errorf("Code = %q, want %q", pe.Code, ErrInvalidZeropsYml)
		}
		if len(pe.APIMeta) != 2 {
			t.Errorf("len(APIMeta) = %d, want 2", len(pe.APIMeta))
		}
		// Multi-field inline expansion preserved from mapAPIError.
		if !strings.Contains(pe.Suggestion, "build.base") || !strings.Contains(pe.Suggestion, "run.base") {
			t.Errorf("Suggestion missing inline multi-field detail: %q", pe.Suggestion)
		}
		if strings.Contains(pe.Suggestion, "read apiMeta") {
			t.Errorf("Suggestion still uses pre-e2ec3651 pointer text: %q", pe.Suggestion)
		}
	})

	t.Run("yamlValidationInvalidYaml_line_error", func(t *testing.T) {
		t.Parallel()
		input := mkFromAPI("yamlValidationInvalidYaml", []APIMetaItem{
			{
				Code: "yamlValidationInvalidYaml",
				Metadata: map[string][]string{
					"reason": {"yaml: line 2: did not find expected ',' or ']'"},
				},
			},
		})
		got := reclassifyValidationError(input)
		pe := got.(*PlatformError)
		if pe.Code != ErrInvalidZeropsYml {
			t.Errorf("Code = %q, want %q", pe.Code, ErrInvalidZeropsYml)
		}
		// Inline expansion surfaces the parser's line-error reason
		// directly in the suggestion (pre-fix it was lost behind the
		// "read apiMeta" pointer).
		if !strings.Contains(pe.Suggestion, "yaml: line 2") {
			t.Errorf("Suggestion missing inline parser detail: %q", pe.Suggestion)
		}
	})

	t.Run("zeropsYamlSetupNotFound", func(t *testing.T) {
		t.Parallel()
		input := mkFromAPI("zeropsYamlSetupNotFound", nil)
		got := reclassifyValidationError(input)
		pe := got.(*PlatformError)
		if pe.Code != ErrInvalidZeropsYml {
			t.Errorf("Code = %q, want %q", pe.Code, ErrInvalidZeropsYml)
		}
		// No meta → mapAPIError's no-meta template ("API rejected the
		// request (code: <APICode>) — check the input parameters") flows
		// through unchanged. Both the "read apiMeta" pointer pattern and
		// the older "see apiMeta" wording are gone since e2ec3651.
		if strings.Contains(pe.Suggestion, "apiMeta") {
			t.Errorf("Suggestion = %q, must not point to apiMeta (no meta on this error)", pe.Suggestion)
		}
		if !strings.Contains(pe.Suggestion, "zeropsYamlSetupNotFound") {
			t.Errorf("Suggestion = %q, want APICode in the no-meta template", pe.Suggestion)
		}
	})

	t.Run("network_error_passes_through_unchanged", func(t *testing.T) {
		t.Parallel()
		input := NewPlatformError(ErrNetworkError, "connection refused", "Check API host and network")
		got := reclassifyValidationError(input)
		pe := got.(*PlatformError)
		if pe.Code != ErrNetworkError {
			t.Errorf("Code = %q, want %q (transport errors must not reclassify)", pe.Code, ErrNetworkError)
		}
	})

	t.Run("auth_error_passes_through_unchanged", func(t *testing.T) {
		t.Parallel()
		apiErr := apiError.Error{HttpStatusCode: http.StatusUnauthorized, ErrorCode: "tokenExpired", Message: "token expired"}
		mapped := mapAPIError(apiErr, "zeropsYaml")
		got := reclassifyValidationError(mapped)
		pe := got.(*PlatformError)
		if pe.Code != ErrAuthTokenExpired {
			t.Errorf("Code = %q, want %q (auth errors must not reclassify)", pe.Code, ErrAuthTokenExpired)
		}
	})

	t.Run("server_5xx_passes_through_unchanged", func(t *testing.T) {
		t.Parallel()
		apiErr := apiError.Error{HttpStatusCode: 502, ErrorCode: "badGateway", Message: "bad gateway"}
		mapped := mapAPIError(apiErr, "zeropsYaml")
		got := reclassifyValidationError(mapped)
		pe := got.(*PlatformError)
		if pe.Code != ErrAPIError {
			t.Errorf("Code = %q, want %q (5xx must not reclassify to invalid-yaml)", pe.Code, ErrAPIError)
		}
	})

	t.Run("nil_error_is_nil", func(t *testing.T) {
		t.Parallel()
		if got := reclassifyValidationError(nil); got != nil {
			t.Errorf("reclassify(nil) = %v, want nil", got)
		}
	})

	t.Run("non_platform_error_passes_through", func(t *testing.T) {
		t.Parallel()
		e := errors.New("some other error type")
		if got := reclassifyValidationError(e); got != e {
			t.Errorf("reclassify pass-through broken for non-platform errors")
		}
	})
}

func TestIsValidationErrorCode(t *testing.T) {
	t.Parallel()
	valid := []string{"zeropsYamlInvalidParameter", "yamlValidationInvalidYaml", "errorList", "zeropsYamlSetupNotFound"}
	invalid := []string{"", "projectImportInvalidParameter", "tokenExpired", "serviceStackTypeNotFound"}
	for _, c := range valid {
		if !isValidationErrorCode(c) {
			t.Errorf("isValidationErrorCode(%q) = false, want true", c)
		}
	}
	for _, c := range invalid {
		if isValidationErrorCode(c) {
			t.Errorf("isValidationErrorCode(%q) = true, want false", c)
		}
	}
}
