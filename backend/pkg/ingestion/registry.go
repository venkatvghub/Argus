// Package ingestion provides Tree-sitter language registry and parsing utilities.
package ingestion

import (
	"log"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tshcl "github.com/tree-sitter-grammars/tree-sitter-hcl/bindings/go"
	tskotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjava "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// LanguageRegistry maps file extensions and language names to Tree-sitter language parsers.
type LanguageRegistry struct {
	extensions map[string]string
	languages  map[string]*sitter.Language
}

// NewLanguageRegistry creates and initializes a new LanguageRegistry with default and extended language grammars.
func NewLanguageRegistry() *LanguageRegistry {
	r := &LanguageRegistry{
		extensions: map[string]string{
			// Core languages
			".go":  "go",
			".js":  "javascript",
			".mjs": "javascript",
			".cjs": "javascript",
			".ts":  "typescript",
			".tsx": "typescript",
			".java": "java",
			".py":  "python",
			// Extended languages
			".kt":  "kotlin",
			".kts": "kotlin",
			".dart": "dart",
			".tf":  "terraform",
			".hcl": "terraform",
			".sql": "sql",
		},
		languages: make(map[string]*sitter.Language),
	}

	r.registerDefaults()
	r.registerExtended()
	return r
}

// registerDefaults registers Tree-sitter languages for core supported languages.
func (r *LanguageRegistry) registerDefaults() {
	r.languages["go"] = sitter.NewLanguage(tsgo.Language())
	r.languages["javascript"] = sitter.NewLanguage(tsjs.Language())
	r.languages["typescript"] = sitter.NewLanguage(tsts.LanguageTypescript())
	r.languages["java"] = sitter.NewLanguage(tsjava.Language())
	r.languages["python"] = sitter.NewLanguage(tspython.Language())
}

// registerExtended registers additional language grammars for Kotlin, Dart, Terraform/HCL, and SQL via registerWithFallback.
func (r *LanguageRegistry) registerExtended() {
	// Kotlin — github.com/tree-sitter-grammars/tree-sitter-kotlin v1.1.0
	r.registerWithFallback("kotlin", sitter.NewLanguage(tskotlin.Language()))

	// Dart — TODO: no upstream Go binding exists yet.
	// Candidates tried:
	//   github.com/UserNobody14/tree-sitter-dart/bindings/go — module path
	//     mismatch (declares itself as github.com/tree-sitter/tree-sitter-dart).
	//   github.com/tree-sitter/tree-sitter-dart — repository not found.
	// Re-evaluate when https://github.com/nickel-org/tree-sitter-dart or an
	// official tree-sitter org repo ships a compatible /bindings/go package.
	r.registerWithFallback("dart", nil)

	// Terraform / HCL — github.com/tree-sitter-grammars/tree-sitter-hcl v1.2.0
	r.registerWithFallback("terraform", sitter.NewLanguage(tshcl.Language()))

	// SQL — TODO: no upstream Go binding exists yet.
	// Candidates tried:
	//   github.com/DerekStride/tree-sitter-sql/bindings/go — module path
	//     mismatch (declares itself as github.com/tree-sitter/tree-sitter-sql).
	//   github.com/tree-sitter/tree-sitter-sql — repository not found.
	// Re-evaluate when an official tree-sitter org SQL repo ships a compatible
	// /bindings/go package.
	r.registerWithFallback("sql", nil)
}

// registerWithFallback registers a language grammar only when the binding is available, skipping nil entries gracefully.
func (r *LanguageRegistry) registerWithFallback(name string, lang *sitter.Language) {
	if lang == nil {
		log.Printf("tree-sitter: grammar for %q is unavailable; files with matching extensions will not be parsed", name)
		return
	}
	r.languages[name] = lang
}

// GetLanguageByExtension returns the Tree-sitter language and name for a given file extension, or (nil, "") if unknown.
func (r *LanguageRegistry) GetLanguageByExtension(ext string) (*sitter.Language, string) {
	langName, ok := r.extensions[strings.ToLower(ext)]
	if !ok {
		return nil, ""
	}
	return r.languages[langName], langName
}

// GetLanguageByName returns the Tree-sitter language for a given language name (case-insensitive), or nil if unknown.
func (r *LanguageRegistry) GetLanguageByName(name string) *sitter.Language {
	return r.languages[strings.ToLower(name)]
}

// GetLanguageForPath returns the Tree-sitter language and name for a given file path by inspecting its extension.
func (r *LanguageRegistry) GetLanguageForPath(path string) (*sitter.Language, string) {
	return r.GetLanguageByExtension(filepath.Ext(path))
}
