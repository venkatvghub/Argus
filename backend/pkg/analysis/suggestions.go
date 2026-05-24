package analysis

// SuggestionFor returns a static remediation suggestion for the given marker type, or empty string for unknown types.
func SuggestionFor(markerType string) string {
	switch markerType {
	case "brain_method":
		return "Break this method into smaller focused functions. Extract complex conditional blocks, apply Command or Strategy pattern to replace branching, and isolate side-effects."
	case "nested_complexity":
		return "Flatten nesting by extracting inner blocks into helper functions, using early returns/guard clauses, and applying decomposition."
	case "complex_method":
		return "Simplify by extracting helper methods, replacing switch/if chains with a lookup table or polymorphism, and removing redundant conditions."
	case "bumpy_road":
		return "Restructure to reduce alternating nesting levels. Consider a state machine or pipeline pattern."
	case "large_method":
		return "Split into smaller methods at logical boundaries. Each method should do one thing. Consider extracting a class/struct."
	case "primitive_obsession":
		return "Introduce value objects or parameter structs instead of long param lists. Group related parameters into a dedicated type."
	case "dry_violation":
		return "Extract the duplicated logic into a shared utility function or module. Verify both clones evolve together — if diverging intentionally, document why."
	case "untested_hotspot":
		return "Add unit and integration tests for this frequently-changed, high-dependency file. Prioritize edge cases and error paths."
	case "coverage_gap":
		return "Increase test coverage by writing tests for uncovered branches. Focus on business-critical paths first."
	case "developer_congestion":
		return "Assign a clear owner or break the file into smaller owned modules. Consider architectural decomposition to reduce multi-team coupling."
	case "knowledge_loss":
		return "Document key design decisions and ownership. Run a knowledge-transfer session; consider pairing or a code review rotation."
	case "dpdp_pii_exposure":
		return "Remove or mask PII from source code. Store identifiers in secure config; use references, not literals. Review DPDP Act obligations."
	case "dpdp_mobile_exposure":
		return "Remove mobile number literals from source. Use environment variables or a secrets manager. Applies to DPDP-regulated data."
	case "pii_email_exposure":
		return "Remove email literals from source. Use configuration or a contact service instead of hardcoded addresses."
	case "broken_crypto":
		return "Replace deprecated hash/cipher with modern alternatives (SHA-256 or better, AES-GCM). Audit all crypto call sites."
	case "tainted_sql":
		return "Use parameterised queries or an ORM. Never concatenate user input into SQL strings."
	case "ssrf_blind":
		return "Validate and allowlist external URLs before making HTTP calls. Use a proxy with an egress policy."
	case "token_bloat":
		return "Reduce file verbosity: split large files, remove commented-out code, consolidate repetitive patterns. Large token density degrades AI context quality."
	case "dead_code":
		return "Remove unreachable or unused code paths. Dead code increases maintenance burden and can mislead future readers."
	case "zombie_export":
		return "Remove unused exported symbols or make them unexported. Dead exports confuse tooling and bloat public API surface."
	case "pii_mobile_exposure":
		return "Remove mobile number literals from source. Store them in secure config or a secrets manager to comply with data-protection requirements."
	case "dart_setstate_after_await":
		return "Avoid calling setState after an async gap without checking mounted. Guard with 'if (!mounted) return;' before setState to prevent use-after-dispose errors."
	case "dart_context_after_await":
		return "Do not use BuildContext across async gaps. Capture context-dependent values before the await, or use a mounted check and a Navigator/ScaffoldMessenger ref."
	case "dart_broken_crypto":
		return "Replace deprecated or weak Dart/Flutter crypto with modern alternatives. Use package:cryptography with AES-GCM or ChaCha20-Poly1305."
	case "sql_injection_risk":
		return "Use parameterized queries or a query builder. Never interpolate user input directly into SQL strings."
	case "sql_select_star":
		return "Replace SELECT * with explicit column lists. Overly broad queries fetch unnecessary data and break when schema changes."
	case "sql_hardcoded_credential":
		return "Remove hardcoded database credentials from source. Use environment variables or a secrets manager and rotate the leaked credential immediately."
	case "phantom_coupling":
		return "Investigate why these files co-change without a structural dependency. Extract a shared abstraction or document the coupling explicitly."
	case "hallucination_bait":
		return "Rename ambiguous or misleading symbols to clearly describe their purpose. Ambiguous names degrade AI-assisted tooling accuracy."
	case "untracked_consent_mutation":
		return "Document data mutation with explicit consent tracking. Add audit logging to state/database mutations involving user data."
	case "rbi_logger_audit_gap":
		return "Add CorrelationID logging to payment and transaction handlers. RBI compliance requires full audit trails for financial operations."
	case "data_sovereignty_leak":
		return "PII is referenced alongside a non-Indian cloud region. Ensure sensitive data is processed and stored within India per DPDP Act requirements."
	default:
		return ""
	}
}
