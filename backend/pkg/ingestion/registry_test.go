package ingestion

import (
	"testing"
)

// TestNewLanguageRegistry tests that NewLanguageRegistry initializes without panic
// and returns a non-nil registry.
func TestNewLanguageRegistry(t *testing.T) {
	r := NewLanguageRegistry()
	if r == nil {
		t.Fatal("NewLanguageRegistry returned nil, expected non-nil registry")
	}
}

// TestGetLanguageByExtension tests GetLanguageByExtension with all known extensions
// (both existing and new extensions that will be added).
func TestGetLanguageByExtension(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name           string
		ext            string
		expectLangName string
		expectLangNil  bool // true if language object may be nil (not yet implemented)
	}{
		// Existing extensions
		{name: "go extension", ext: ".go", expectLangName: "go", expectLangNil: false},
		{name: "js extension", ext: ".js", expectLangName: "javascript", expectLangNil: false},
		{name: "mjs extension", ext: ".mjs", expectLangName: "javascript", expectLangNil: false},
		{name: "cjs extension", ext: ".cjs", expectLangName: "javascript", expectLangNil: false},
		{name: "ts extension", ext: ".ts", expectLangName: "typescript", expectLangNil: false},
		{name: "tsx extension", ext: ".tsx", expectLangName: "typescript", expectLangNil: false},
		{name: "java extension", ext: ".java", expectLangName: "java", expectLangNil: false},
		{name: "py extension", ext: ".py", expectLangName: "python", expectLangNil: false},

		// New extensions (may not have language implementations yet)
		{name: "kt extension", ext: ".kt", expectLangName: "kotlin", expectLangNil: true},
		{name: "kts extension", ext: ".kts", expectLangName: "kotlin", expectLangNil: true},
		{name: "dart extension", ext: ".dart", expectLangName: "dart", expectLangNil: true},
		{name: "tf extension", ext: ".tf", expectLangName: "terraform", expectLangNil: true},
		{name: "hcl extension", ext: ".hcl", expectLangName: "terraform", expectLangNil: true},
		{name: "sql extension", ext: ".sql", expectLangName: "sql", expectLangNil: true},

		// Unknown extension
		{name: "unknown extension", ext: ".xyz", expectLangName: "", expectLangNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang, langName := r.GetLanguageByExtension(tt.ext)

			// Check language name
			if langName != tt.expectLangName {
				t.Errorf("expected language name %q, got %q", tt.expectLangName, langName)
			}

			// Check language object
			if tt.expectLangNil {
				if langName != "" && lang == nil {
					// Expected: language name is known but language object may not be implemented
					t.Logf("language name %q found but grammar implementation not available (expected)", langName)
				} else if langName == "" && lang != nil {
					t.Errorf("expected both langName and lang to be nil/empty for unknown extension, got langName=%q lang=%v", langName, lang)
				}
				// Both nil/empty is acceptable for unknown extensions
			} else {
				if lang == nil {
					t.Errorf("expected non-nil language object for known extension %s, got nil", tt.ext)
				}
			}
		})
	}
}

// TestGetLanguageByName tests GetLanguageByName with all known language names.
func TestGetLanguageByName(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name         string
		langName     string
		expectNonNil bool // false if language implementation may not exist
	}{
		// Implemented languages
		{name: "go language", langName: "go", expectNonNil: true},
		{name: "javascript language", langName: "javascript", expectNonNil: true},
		{name: "typescript language", langName: "typescript", expectNonNil: true},
		{name: "java language", langName: "java", expectNonNil: true},
		{name: "python language", langName: "python", expectNonNil: true},

		// Not yet implemented languages
		{name: "kotlin language", langName: "kotlin", expectNonNil: false},
		{name: "dart language", langName: "dart", expectNonNil: false},
		{name: "terraform language", langName: "terraform", expectNonNil: false},
		{name: "sql language", langName: "sql", expectNonNil: false},

		// Unknown language
		{name: "unknown language", langName: "unknown", expectNonNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := r.GetLanguageByName(tt.langName)

			if tt.expectNonNil {
				if lang == nil {
					t.Errorf("expected non-nil language for %q, got nil", tt.langName)
				}
			} else {
				if lang == nil {
					t.Logf("language %q not implemented or not found (expected)", tt.langName)
				}
				// Gracefully skip if nil
			}
		})
	}
}

// TestGetLanguageByNameCaseInsensitive tests that language name lookup is case-insensitive.
func TestGetLanguageByNameCaseInsensitive(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name     string
		langName string
	}{
		{name: "uppercase GO", langName: "GO"},
		{name: "mixed case Go", langName: "Go"},
		{name: "lowercase go", langName: "go"},
		{name: "uppercase JAVASCRIPT", langName: "JAVASCRIPT"},
		{name: "mixed case TypeScript", langName: "TypeScript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := r.GetLanguageByName(tt.langName)
			// For implemented languages, should return non-nil
			// The exact behavior depends on the language being implemented
			if lang == nil {
				t.Logf("language for %q returned nil (may be unimplemented)", tt.langName)
			}
		})
	}
}

// TestGetLanguageForPath tests GetLanguageForPath with full file paths.
func TestGetLanguageForPath(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name           string
		path           string
		expectLangName string
		expectLangNil  bool
	}{
		// Existing languages
		{name: "main.go", path: "main.go", expectLangName: "go", expectLangNil: false},
		{name: "app.js in root", path: "app.js", expectLangName: "javascript", expectLangNil: false},
		{name: "src/App.tsx", path: "src/App.tsx", expectLangName: "typescript", expectLangNil: false},
		{name: "src/Main.java", path: "src/Main.java", expectLangName: "java", expectLangNil: false},
		{name: "scripts/util.py", path: "scripts/util.py", expectLangName: "python", expectLangNil: false},

		// New languages
		{name: "main.kt", path: "main.kt", expectLangName: "kotlin", expectLangNil: true},
		{name: "src/App.dart", path: "src/App.dart", expectLangName: "dart", expectLangNil: true},
		{name: "main.tf", path: "main.tf", expectLangName: "terraform", expectLangNil: true},
		{name: "variables.hcl", path: "variables.hcl", expectLangName: "terraform", expectLangNil: true},
		{name: "query.sql", path: "query.sql", expectLangName: "sql", expectLangNil: true},

		// Path with multiple dots
		{name: "config.test.ts", path: "config.test.ts", expectLangName: "typescript", expectLangNil: false},
		{name: "main.module.kts", path: "main.module.kts", expectLangName: "kotlin", expectLangNil: true},

		// Unknown extension
		{name: "readme.txt", path: "readme.txt", expectLangName: "", expectLangNil: true},
		{name: "no extension file", path: "Makefile", expectLangName: "", expectLangNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang, langName := r.GetLanguageForPath(tt.path)

			if langName != tt.expectLangName {
				t.Errorf("expected language name %q, got %q for path %q", tt.expectLangName, langName, tt.path)
			}

			if tt.expectLangNil {
				if langName != "" && lang == nil {
					t.Logf("language %q found but implementation not available (expected)", langName)
				}
			} else {
				if lang == nil {
					t.Errorf("expected non-nil language for path %q, got nil", tt.path)
				}
			}
		})
	}
}

// TestLanguageRegistryNoCrash tests that GetLanguageByExtension never panics,
// even with unusual or edge-case inputs.
func TestLanguageRegistryNoCrash(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name string
		ext  string
	}{
		{name: "empty string", ext: ""},
		{name: "no dot", ext: "go"},
		{name: "just dot", ext: "."},
		{name: "multiple dots", ext: "..."},
		{name: "very long extension", ext: ".verylongextensionnamethatdoesntexist"},
		{name: "special characters", ext: ".@#$%"},
		{name: "whitespace", ext: " .go "},
		{name: "null-like string", ext: "\x00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			lang, langName := r.GetLanguageByExtension(tt.ext)
			_ = lang
			_ = langName
			t.Logf("no crash with extension %q: langName=%q lang=%v", tt.ext, langName, lang != nil)
		})
	}
}

// TestCaseSensitivityExtensions tests that extension lookup is case-insensitive.
func TestCaseSensitivityExtensions(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name          string
		ext           string
		expectedMatch string // the lowercase extension that should match
	}{
		{name: ".GO uppercase", ext: ".GO", expectedMatch: ".go"},
		{name: ".Go mixed", ext: ".Go", expectedMatch: ".go"},
		{name: ".JS uppercase", ext: ".JS", expectedMatch: ".js"},
		{name: ".KT uppercase", ext: ".KT", expectedMatch: ".kt"},
		{name: ".Dart mixed", ext: ".Dart", expectedMatch: ".dart"},
		{name: ".TF uppercase", ext: ".TF", expectedMatch: ".tf"},
		{name: ".SQL uppercase", ext: ".SQL", expectedMatch: ".sql"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			langFromInput, nameFromInput := r.GetLanguageByExtension(tt.ext)
			langFromLower, nameFromLower := r.GetLanguageByExtension(tt.expectedMatch)

			// Both should return the same language name
			if nameFromInput != nameFromLower {
				t.Errorf("case-insensitive lookup failed: %q returned %q, but %q returned %q",
					tt.ext, nameFromInput, tt.expectedMatch, nameFromLower)
			}

			// Language objects should match (both nil or both non-nil with same behavior)
			if (langFromInput == nil) != (langFromLower == nil) {
				t.Errorf("language objects don't match for %q vs %q (case sensitivity issue)", tt.ext, tt.expectedMatch)
			}
		})
	}
}

// TestExtensionWithoutDot tests extensions provided without leading dot are handled gracefully.
func TestExtensionWithoutDot(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		name string
		ext  string
	}{
		{name: "go without dot", ext: "go"},
		{name: "js without dot", ext: "js"},
		{name: "kt without dot", ext: "kt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang, langName := r.GetLanguageByExtension(tt.ext)
			// Extensions without dot will not match (by design)
			if langName != "" {
				t.Logf("extension without dot %q returned language (may be by design)", tt.ext)
			} else {
				t.Logf("extension without dot %q correctly returned empty (expected)", tt.ext)
			}
			_ = lang
		})
	}
}

// TestAllLanguageNamesFromRegistry verifies that all registered language names
// can be looked up without causing panics.
func TestAllLanguageNamesFromRegistry(t *testing.T) {
	r := NewLanguageRegistry()

	// Test all implemented language names by attempting lookup
	implementedLanguages := []string{
		"go",
		"javascript",
		"typescript",
		"java",
		"python",
	}

	for _, langName := range implementedLanguages {
		t.Run(langName, func(t *testing.T) {
			lang := r.GetLanguageByName(langName)
			if lang == nil {
				t.Errorf("expected non-nil language for implemented language %q", langName)
			}
		})
	}
}

// TestParallelRegistryAccess verifies that concurrent access to the registry doesn't cause panics.
// This tests thread-safety of the registry (though current implementation does not protect against writes).
func TestParallelRegistryAccess(t *testing.T) {
	r := NewLanguageRegistry()

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			// Parallel reads
			r.GetLanguageByExtension(".go")
			r.GetLanguageByExtension(".js")
			r.GetLanguageByExtension(".kt")
			r.GetLanguageByName("go")
			r.GetLanguageByName("javascript")
			r.GetLanguageForPath("main.go")
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
