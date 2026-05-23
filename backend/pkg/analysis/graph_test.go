package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestBuildGraph(t *testing.T) {
	ge := NewGraphEngine()

	files := []models.FileNode{
		{Path: "main.go", IsFile: true},
		{Path: "utils.go", IsFile: true},
	}

	symbols := []models.Symbol{
		{Name: "main", Type: models.SymbolFunction, FilePath: "main.go"},
		{Name: "Helper", Type: models.SymbolFunction, FilePath: "utils.go"},
	}

	// Relation from main function to Helper function
	relations := []models.Relation{
		{From: "main.go:main", To: "utils.go:Helper", Type: "calls"},
	}

	err := ge.BuildGraph(files, symbols, relations)
	assert.NoError(t, err)

	// 1. Verify Node Existence
	mainFileNode, ok := ge.GetNode("main.go")
	assert.True(t, ok)
	assert.Equal(t, "main.go", mainFileNode.Name)
	assert.Equal(t, "file", mainFileNode.Type)

	mainFuncNode, ok := ge.GetNode("main.go:main")
	assert.True(t, ok)
	assert.Equal(t, "main", mainFuncNode.Name)
	assert.Equal(t, "function", mainFuncNode.Type)

	helperFuncNode, ok := ge.GetNode("utils.go:Helper")
	assert.True(t, ok)
	assert.Equal(t, "Helper", helperFuncNode.Name)

	// 2. Verify PageRank (Importance)
	// main.go -> main.go:main -> utils.go:Helper
	// utils.go -> utils.go:Helper
	// Helper has 2 incoming edges, main function has 1, files have 0.
	assert.Greater(t, helperFuncNode.PageRank, mainFuncNode.PageRank, "Helper should be more important than main function")
	assert.Greater(t, mainFuncNode.PageRank, mainFileNode.PageRank, "main function should be more important than main.go file (which has no incoming)")
}

func TestEmptyGraph(t *testing.T) {
	ge := NewGraphEngine()
	err := ge.BuildGraph(nil, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(ge.nodes))
}

func TestRelationshipContains(t *testing.T) {
	ge := NewGraphEngine()

	files := []models.FileNode{{Path: "pkg/api.go", IsFile: true}}
	symbols := []models.Symbol{
		{Name: "Handler", Type: models.SymbolFunction, FilePath: "pkg/api.go"},
	}

	// BuildGraph automatically adds "contains" relations
	err := ge.BuildGraph(files, symbols, nil)
	assert.NoError(t, err)

	fileNode, _ := ge.GetNode("pkg/api.go")
	symbolNode, _ := ge.GetNode("pkg/api.go:Handler")

	// symbolNode should have higher PageRank than fileNode because of the "contains" edge
	assert.Greater(t, symbolNode.PageRank, fileNode.PageRank)
}
