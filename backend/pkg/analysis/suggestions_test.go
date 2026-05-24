package analysis_test

import (
	"testing"

	"github.com/venkatvghub/argus/pkg/analysis"
)

func TestSuggestionFor_KnownTypes(t *testing.T) {
	tests := []struct {
		markerType string
		name       string
	}{
		{
			name:       "brain_method",
			markerType: "brain_method",
		},
		{
			name:       "nested_complexity",
			markerType: "nested_complexity",
		},
		{
			name:       "complex_method",
			markerType: "complex_method",
		},
		{
			name:       "bumpy_road",
			markerType: "bumpy_road",
		},
		{
			name:       "large_method",
			markerType: "large_method",
		},
		{
			name:       "primitive_obsession",
			markerType: "primitive_obsession",
		},
		{
			name:       "dry_violation",
			markerType: "dry_violation",
		},
		{
			name:       "untested_hotspot",
			markerType: "untested_hotspot",
		},
		{
			name:       "coverage_gap",
			markerType: "coverage_gap",
		},
		{
			name:       "developer_congestion",
			markerType: "developer_congestion",
		},
		{
			name:       "knowledge_loss",
			markerType: "knowledge_loss",
		},
		{
			name:       "dpdp_pii_exposure",
			markerType: "dpdp_pii_exposure",
		},
		{
			name:       "dpdp_mobile_exposure",
			markerType: "dpdp_mobile_exposure",
		},
		{
			name:       "pii_email_exposure",
			markerType: "pii_email_exposure",
		},
		{
			name:       "broken_crypto",
			markerType: "broken_crypto",
		},
		{
			name:       "tainted_sql",
			markerType: "tainted_sql",
		},
		{
			name:       "ssrf_blind",
			markerType: "ssrf_blind",
		},
		{
			name:       "token_bloat",
			markerType: "token_bloat",
		},
		{
			name:       "zombie_export",
			markerType: "zombie_export",
		},
		{
			name:       "phantom_coupling",
			markerType: "phantom_coupling",
		},
		{
			name:       "hallucination_bait",
			markerType: "hallucination_bait",
		},
		{
			name:       "untracked_consent_mutation",
			markerType: "untracked_consent_mutation",
		},
		{
			name:       "rbi_logger_audit_gap",
			markerType: "rbi_logger_audit_gap",
		},
		{
			name:       "data_sovereignty_leak",
			markerType: "data_sovereignty_leak",
		},
		{
			name:       "dead_code",
			markerType: "dead_code",
		},
		{
			name:       "pii_mobile_exposure",
			markerType: "pii_mobile_exposure",
		},
		{
			name:       "dart_setstate_after_await",
			markerType: "dart_setstate_after_await",
		},
		{
			name:       "dart_context_after_await",
			markerType: "dart_context_after_await",
		},
		{
			name:       "dart_broken_crypto",
			markerType: "dart_broken_crypto",
		},
		{
			name:       "sql_injection_risk",
			markerType: "sql_injection_risk",
		},
		{
			name:       "sql_select_star",
			markerType: "sql_select_star",
		},
		{
			name:       "sql_hardcoded_credential",
			markerType: "sql_hardcoded_credential",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion := analysis.SuggestionFor(tt.markerType)
			if suggestion == "" {
				t.Errorf("SuggestionFor(%q) returned empty string, expected non-empty suggestion", tt.markerType)
			}
		})
	}
}

func TestSuggestionFor_UnknownType(t *testing.T) {
	suggestion := analysis.SuggestionFor("nonexistent_marker")
	if suggestion != "" {
		t.Errorf("SuggestionFor(\"nonexistent_marker\") = %q, expected empty string", suggestion)
	}
}

func TestSuggestionFor_EmptyString(t *testing.T) {
	suggestion := analysis.SuggestionFor("")
	if suggestion != "" {
		t.Errorf("SuggestionFor(\"\") = %q, expected empty string", suggestion)
	}
}
