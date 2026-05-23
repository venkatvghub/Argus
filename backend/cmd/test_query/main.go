// test_query is a local dev utility for testing tree-sitter query patterns
// against Go source snippets. Build and run with:
//
//	cd backend && go run ./cmd/test_query
//
// It prints the S-expression of the parsed AST for the hardcoded snippet.
// Not intended for CI; the ingestion package tests cover production parsing.
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

	fmt.Println(tree.RootNode().ToSexp())
}
