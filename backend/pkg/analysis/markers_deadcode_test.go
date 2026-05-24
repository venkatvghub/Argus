package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

// TestDetectDeadCode_UnexportedSymbol verifies dead_code marker for unexported symbol with only file-contains edge.
func TestDetectDeadCode_UnexportedSymbol(t *testing.T) {
	graph := NewGraphEngine()

	// Create an unexported symbol without file "contains" edges so it has zero incoming edges.
	symbols := []models.Symbol{
		{
			Name:     "myHelper",
			Type:     models.SymbolFunction,
			Line:     10,
			FilePath: "helper.go",
		},
	}

	for i := range symbols {
		s := symbols[i]
		node := &Node{
			id:       graph.g.NewNode().ID(),
			Name:     s.Name,
			Type:     string(s.Type),
			nodeType: NodeTypeSymbol,
			symbol:   &s,
		}
		graph.nodes[symbolKey(s.FilePath, s.Name)] = node
		graph.g.AddNode(node)
	}

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	require.NotEmpty(t, markers)
	found := false
	for _, m := range markers {
		if m.Type == "dead_code" && m.File == "helper.go" {
			found = true
			assert.Equal(t, "low", m.Severity)
			assert.Equal(t, models.ScoreCatDeadCode, m.Category)
			assert.Equal(t, 10, m.Line)
			break
		}
	}
	assert.True(t, found, "dead_code marker not found for unused unexported symbol")
}

// TestDetectDeadCode_ExportedSymbol verifies dead_code marker for exported symbol with no incoming edges.
func TestDetectDeadCode_ExportedSymbol(t *testing.T) {
	graph := NewGraphEngine()

	// Create an exported symbol - same approach as UnexportedSymbol test
	symbols := []models.Symbol{
		{
			Name:     "UnusedHandler",
			Type:     models.SymbolFunction,
			Line:     20,
			FilePath: "api.go",
		},
	}

	// Manually add symbol node without any edges
	for i := range symbols {
		s := symbols[i]
		node := &Node{
			id:       graph.g.NewNode().ID(),
			Name:     s.Name,
			Type:     string(s.Type),
			nodeType: NodeTypeSymbol,
			symbol:   &s,
		}
		graph.nodes[symbolKey(s.FilePath, s.Name)] = node
		graph.g.AddNode(node)
	}

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	require.NotEmpty(t, markers)
	found := false
	for _, m := range markers {
		if m.Type == "dead_code" && m.File == "api.go" {
			found = true
			assert.Equal(t, "low", m.Severity)
			assert.Equal(t, 20, m.Line)
			break
		}
	}
	assert.True(t, found, "dead_code marker not found for unused exported symbol")
}

// TestDetectDeadCode_UsedSymbol verifies no marker for symbol with incoming edges.
func TestDetectDeadCode_UsedSymbol(t *testing.T) {
	graph := NewGraphEngine()

	files := []models.FileNode{
		{
			Path:   "handler.go",
			IsFile: true,
		},
		{
			Path:   "main.go",
			IsFile: true,
		},
	}

	symbols := []models.Symbol{
		{
			Name:     "Handler",
			Type:     models.SymbolFunction,
			Line:     10,
			FilePath: "handler.go",
		},
		{
			Name:     "Run",
			Type:     models.SymbolFunction,
			Line:     20,
			FilePath: "main.go",
		},
	}

	require.NoError(t, graph.BuildGraph(files, symbols, nil))

	// Add a call edge from Run to Handler (meaning Handler is called)
	require.NoError(t, graph.AddCallEdge("main.go", "Run", "handler.go", "Handler"))

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	// Handler should NOT have a dead_code marker (it's called by Run)
	for _, m := range markers {
		if m.Type == "dead_code" && m.File == "handler.go" && m.Line == 10 {
			t.Fatalf("dead_code marker should not be emitted for used symbol Handler")
		}
	}

	// Run should have a dead_code marker (only incoming call edges prevent dead_code, not contains edges)
	foundRun := false
	for _, m := range markers {
		if m.Type == "dead_code" && m.File == "main.go" && m.Line == 20 {
			foundRun = true
			break
		}
	}
	assert.True(t, foundRun, "dead_code marker should be emitted for Run with no incoming call edges")
}

// TestDetectDeadCode_MultipleUnused verifies markers for multiple unused symbols.
func TestDetectDeadCode_MultipleUnused(t *testing.T) {
	graph := NewGraphEngine()

	symbols := []models.Symbol{
		{
			Name:     "unusedHelper1",
			Type:     models.SymbolFunction,
			Line:     5,
			FilePath: "util.go",
		},
		{
			Name:     "unusedHelper2",
			Type:     models.SymbolFunction,
			Line:     15,
			FilePath: "util.go",
		},
		{
			Name:     "usedHelper",
			Type:     models.SymbolFunction,
			Line:     25,
			FilePath: "util.go",
		},
	}

	// Manually add all symbol nodes without file "contains" edges
	for i := range symbols {
		s := symbols[i]
		node := &Node{
			id:       graph.g.NewNode().ID(),
			Name:     s.Name,
			Type:     string(s.Type),
			nodeType: NodeTypeSymbol,
			symbol:   &s,
		}
		graph.nodes[symbolKey(s.FilePath, s.Name)] = node
		graph.g.AddNode(node)
	}

	// Add a call edge from unusedHelper1 to usedHelper
	// This means unusedHelper1 has an OUTGOING edge but NO incoming edge, so it's still dead
	require.NoError(t, graph.AddCallEdge("util.go", "unusedHelper1", "util.go", "usedHelper"))

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	deadCodeCount := 0
	for _, m := range markers {
		if m.Type == "dead_code" {
			deadCodeCount++
		}
	}

	// Should have 2 dead code markers: unusedHelper1 (has outgoing call but no incoming)
	// and unusedHelper2 (no edges at all)
	assert.Equal(t, 2, deadCodeCount, "should have dead code markers for both unused symbols")
}

// TestDetectDeadCode_EmptyGraph verifies empty graph returns no markers.
func TestDetectDeadCode_EmptyGraph(t *testing.T) {
	graph := NewGraphEngine()

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	assert.Empty(t, markers)
}

// TestDetectDeadCode_OnlyFileNodes verifies no markers when only file nodes exist.
func TestDetectDeadCode_OnlyFileNodes(t *testing.T) {
	graph := NewGraphEngine()

	files := []models.FileNode{
		{
			Path:   "main.go",
			IsFile: true,
		},
		{
			Path:   "util.go",
			IsFile: true,
		},
	}

	require.NoError(t, graph.BuildGraph(files, nil, nil))

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	assert.Empty(t, markers, "no dead_code markers for file-only graph")
}

// TestDetectDeadCode_SymbolWithIncomingCall verifies symbol with incoming call is not marked dead.
func TestDetectDeadCode_SymbolWithIncomingCall(t *testing.T) {
	graph := NewGraphEngine()

	symbols := []models.Symbol{
		{
			Name:     "Caller",
			Type:     models.SymbolFunction,
			Line:     5,
			FilePath: "recursive.go",
		},
		{
			Name:     "Callee",
			Type:     models.SymbolFunction,
			Line:     10,
			FilePath: "recursive.go",
		},
	}

	// Manually add symbol nodes
	for i := range symbols {
		s := symbols[i]
		node := &Node{
			id:       graph.g.NewNode().ID(),
			Name:     s.Name,
			Type:     string(s.Type),
			nodeType: NodeTypeSymbol,
			symbol:   &s,
		}
		graph.nodes[symbolKey(s.FilePath, s.Name)] = node
		graph.g.AddNode(node)
	}

	// Add a call edge from Caller to Callee
	require.NoError(t, graph.AddCallEdge("recursive.go", "Caller", "recursive.go", "Callee"))

	markers := NewMarkerEngine("", nil).detectZombieExports(graph)

	// Callee has incoming edge and should NOT be marked dead
	for _, m := range markers {
		if m.Type == "dead_code" && m.File == "recursive.go" && m.Line == 10 {
			t.Fatalf("dead_code marker should not be emitted for symbol with incoming call")
		}
	}

	// Caller has no incoming edge and SHOULD be marked dead
	found := false
	for _, m := range markers {
		if m.Type == "dead_code" && m.File == "recursive.go" && m.Line == 5 {
			found = true
			break
		}
	}
	assert.True(t, found, "dead_code marker should be emitted for symbol with no incoming calls")
}
