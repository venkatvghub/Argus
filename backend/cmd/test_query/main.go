package main

import (
	"fmt"
	"log"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func main() {
	code := []byte(`
package main

func main() {
	go func() {
		println("empty")
	}()

	go func(x int) {
		println(x)
	}(1)
}
`)
	lang := sitter.NewLanguage(tsgo.Language())
	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(lang); err != nil {
		log.Fatalf("set language: %v", err)
	}

	tree := parser.Parse(code, nil)
	if tree == nil {
		log.Fatal("parse returned nil tree")
	}
	defer tree.Close()

	// Test queries (uncomment when ready to run against the tree):
	// queryStr := `(go_statement (call_expression function: (parenthesized_expression (func_literal parameters: (parameter_list))))) @match`
	// queryStr2 := `(go_statement (call_expression function: (func_literal parameters: (parameter_list) @params (#eq? @params "()")))) @match`

	fmt.Println(tree.RootNode().ToSexp())
}
