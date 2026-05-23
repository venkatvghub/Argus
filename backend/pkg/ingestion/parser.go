package ingestion

import (
	"context"
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"
	"github.com/venkatvghub/argus/pkg/models"
)

// TreeSitterParser provides AST parsing and querying capabilities.
type TreeSitterParser struct {
	registry *LanguageRegistry
	queries  map[string]map[string]*sitter.Query
}

// NewTreeSitterParser initializes a new parser and pre-compiles biomarker queries.
func NewTreeSitterParser() (*TreeSitterParser, error) {
	registry := NewLanguageRegistry()
	p := &TreeSitterParser{
		registry: registry,
		queries:  make(map[string]map[string]*sitter.Query),
	}

	if err := p.initBiomarkers(); err != nil {
		return nil, fmt.Errorf("failed to init biomarkers: %w", err)
	}

	return p, nil
}

// Parse generates an AST for the given content based on the file path's language.
func (p *TreeSitterParser) Parse(ctx context.Context, content []byte, path string) (*sitter.Tree, string, error) {
	lang, langName := p.registry.GetLanguageForPath(path)
	if lang == nil {
		return nil, "", fmt.Errorf("unsupported language for path: %s", path)
	}

	parser := sitter.NewParser()
	if err := parser.SetLanguage(lang); err != nil {
		return nil, langName, fmt.Errorf("failed to set language for %s: %w", path, err)
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil, langName, fmt.Errorf("failed to parse %s", path)
	}

	return tree, langName, nil
}

// ExecuteBiomarkers runs pre-defined queries against the AST and returns symbols.
func (p *TreeSitterParser) ExecuteBiomarkers(tree *sitter.Tree, langName string, content []byte) ([]models.Symbol, error) {
	langQueries, ok := p.queries[langName]
	if !ok {
		return nil, nil
	}

	var symbols []models.Symbol
	root := tree.RootNode()

	for name, query := range langQueries {
		cursor := sitter.NewQueryCursor()
		matches := cursor.Matches(query, root, content)

		for {
			match := matches.Next()
			if match == nil {
				break
			}

			for _, capture := range match.Captures {
				node := capture.Node
				symbols = append(symbols, models.Symbol{
					Name:     name,
					Type:     models.SymbolType("biomarker"),
					Line:     int(node.StartPosition().Row + 1),
					EndLine:  int(node.EndPosition().Row + 1),
					FilePath: "",
				})
			}
		}
	}

	return symbols, nil
}

func (p *TreeSitterParser) initBiomarkers() error {
	// --- 4.1. Concurrency & Runtime Risk ---
	goConcurrency := map[string]string{
		"unsecured_goroutine":      "(go_statement) @go_routine",
		"goroutine_outer_mutation": "(go_statement (call_expression function: (func_literal parameters: (parameter_list) @params (#eq? @params \"()\")))) @goroutine_outer_mutation",
	}

	jsConcurrency := map[string]string{
		"async_race_condition": "(arrow_function body: (statement_block (expression_statement (assignment_expression)))) @async_race",
		"read_before_await":    "(function_declaration body: (statement_block (variable_declaration (variable_declarator)) (expression_statement (await_expression)))) @read_before_await",
	}

	javaConcurrency := map[string]string{
		"unsynchronized_mutation": "(method_declaration name: (identifier) @method_name (#match? @method_name \"run\") body: (block (expression_statement (assignment_expression)))) @unsynchronized_mutation",
	}

	pyConcurrency := map[string]string{
		"logical_race": "(await (call)) @logical_race",
	}

	// --- 4.4. Structural & AppSec ---
	goAppSec := map[string]string{
		"broken_crypto": "(call_expression function: (selector_expression field: (field_identifier) @method (#match? @method \"^(NewMD5|NewSHA1|NewDES)$\"))) @broken_crypto",
		"tainted_sql":   "(call_expression function: (selector_expression field: (field_identifier) @method (#match? @method \"Query|Exec|QueryRow\")) arguments: (argument_list (binary_expression operator: \"+\"))) @tainted_sql",
		"ssrf_blind":    "(call_expression function: (selector_expression field: (field_identifier) @method (#match? @method \"^(Get|Post|Do)$\")) arguments: (argument_list (identifier) @url)) @ssrf_blind",
		"bypassed_rbac": "(function_declaration name: (identifier) @name (#match? @name \"^(Transfer|Withdraw|Pay|Delete).*$\")) @bypassed_rbac",
	}

	jsAppSec := map[string]string{
		"broken_crypto": "(call_expression function: (member_expression property: (property_identifier) @method (#match? @method \"createHash\")) arguments: (arguments (string) @algo (#match? @algo \"md5|sha1\"))) @broken_crypto",
		"tainted_sql":   "(call_expression function: (member_expression property: (property_identifier) @method (#match? @method \"query\")) arguments: (arguments (binary_expression operator: \"+\"))) @tainted_sql",
		"ssrf_blind":    "(call_expression function: (identifier) @func (#match? @func \"^(fetch|axios)$\") arguments: (arguments (identifier) @url)) @ssrf_blind",
		"bypassed_rbac": "(function_declaration name: (identifier) @name (#match? @name \"^(transfer|withdraw|pay|delete).*$\")) @bypassed_rbac",
	}

	javaAppSec := map[string]string{
		"broken_crypto": "(method_invocation name: (identifier) @method (#match? @method \"^(getInstance)$\") arguments: (argument_list (string_literal) @alg (#match? @alg \"MD5|SHA-1|DES\"))) @broken_crypto",
		"tainted_sql":   "(method_invocation name: (identifier) @method (#match? @method \"^(executeQuery|executeUpdate)$\") arguments: (argument_list (binary_expression operator: \"+\"))) @tainted_sql",
		"ssrf_blind":    "(method_invocation name: (identifier) @method (#match? @method \"^(openStream|openConnection)$\")) @ssrf_blind",
		"bypassed_rbac": "(method_declaration name: (identifier) @name (#match? @name \"^(transfer|withdraw|pay|delete).*$\")) @bypassed_rbac",
	}

	pyAppSec := map[string]string{
		"broken_crypto": "(call function: (attribute attribute: (identifier) @method (#match? @method \"^(md5|sha1)$\"))) @broken_crypto",
		"tainted_sql":   "(call function: (attribute attribute: (identifier) @method (#match? @method \"execute\")) arguments: (argument_list (binary_operator))) @tainted_sql",
		"ssrf_blind":    "(call function: (attribute attribute: (identifier) @method (#match? @method \"^(get|post)$\")) arguments: (argument_list (identifier) @url)) @ssrf_blind",
		"bypassed_rbac": "(function_definition name: (identifier) @name (#match? @name \"^(transfer|withdraw|pay|delete).*$\")) @bypassed_rbac",
	}

	// Redis specific biomarkers
	goAppSec["redis_unbounded_keys_scan"] = "(call_expression function: (selector_expression field: (field_identifier) @method (#match? @method \"(?i)Do|Command|Keys\")) arguments: (argument_list (interpreted_string_literal) @cmd_param (#match? @cmd_param \"(?i)KEYS\"))) @blocking_redis_call"
	// Using a raw string for the js query to avoid double quote escaping issues in this cat command
	jsAppSec["redis_empty_password_auth"] = `(object (pair key: (property_identifier) @prop_name (#eq? @prop_name "password") value: (string) @prop_val (#eq? @prop_val "\"\"")))`

	all := map[string]map[string]string{
		"go":         merge(goConcurrency, goAppSec),
		"javascript": merge(jsConcurrency, jsAppSec),
		"typescript": merge(jsConcurrency, jsAppSec),
		"java":       merge(javaConcurrency, javaAppSec),
		"python":     merge(pyConcurrency, pyAppSec),
	}

	for langName, queries := range all {
		lang := p.registry.GetLanguageByName(langName)
		if lang == nil {
			continue
		}
		p.queries[langName] = make(map[string]*sitter.Query)
		for name, qStr := range queries {
			q, err := sitter.NewQuery(lang, qStr)
			if err != nil {
				fmt.Printf("Warning: failed to compile query %s for %s: %v\n", name, langName, err)
				continue
			}
			p.queries[langName][name] = q
		}
	}

	return nil
}

func merge(m1, m2 map[string]string) map[string]string {
	res := make(map[string]string)
	for k, v := range m1 {
		res[k] = v
	}
	for k, v := range m2 {
		res[k] = v
	}
	return res
}
