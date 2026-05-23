package analysis

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestDetectCommunities_Clusters(t *testing.T) {
	ge := NewGraphEngine()

	// Create 3 clusters:
	// Cluster 1: A1, A2, A3
	// Cluster 2: B1, B2, B3
	// Cluster 3: C1, C2, C3

	files := []models.FileNode{
		{Path: "A1.go"}, {Path: "A2.go"}, {Path: "A3.go"},
		{Path: "B1.go"}, {Path: "B2.go"}, {Path: "B3.go"},
		{Path: "C1.go"}, {Path: "C2.go"}, {Path: "C3.go"},
	}

	err := ge.BuildGraph(files, nil, nil)
	assert.NoError(t, err)

	// Internal edges for Cluster A
	assert.NoError(t, ge.AddCoChangeEdge("A1.go", "A2.go"))
	assert.NoError(t, ge.AddCoChangeEdge("A2.go", "A3.go"))
	assert.NoError(t, ge.AddCoChangeEdge("A3.go", "A1.go"))

	// Internal edges for Cluster B
	assert.NoError(t, ge.AddCoChangeEdge("B1.go", "B2.go"))
	assert.NoError(t, ge.AddCoChangeEdge("B2.go", "B3.go"))
	assert.NoError(t, ge.AddCoChangeEdge("B3.go", "B1.go"))

	// Internal edges for Cluster C
	assert.NoError(t, ge.AddCoChangeEdge("C1.go", "C2.go"))
	assert.NoError(t, ge.AddCoChangeEdge("C2.go", "C3.go"))
	assert.NoError(t, ge.AddCoChangeEdge("C3.go", "C1.go"))

	// Add some weak bridges
	assert.NoError(t, ge.AddCoChangeEdge("A1.go", "B1.go"))
	assert.NoError(t, ge.AddCoChangeEdge("B1.go", "C1.go"))

	communities := ge.DetectCommunities()

	// We expect 3 main communities
	assert.GreaterOrEqual(t, len(communities), 3)

	// Verify nodes in same cluster have same CommunityID
	nA1, _ := ge.GetNode("A1.go")
	nA2, _ := ge.GetNode("A2.go")
	nA3, _ := ge.GetNode("A3.go")
	assert.Equal(t, nA1.CommunityID, nA2.CommunityID)
	assert.Equal(t, nA2.CommunityID, nA3.CommunityID)

	nB1, _ := ge.GetNode("B1.go")
	nB2, _ := ge.GetNode("B2.go")
	nB3, _ := ge.GetNode("B3.go")
	assert.Equal(t, nB1.CommunityID, nB2.CommunityID)
	assert.Equal(t, nB2.CommunityID, nB3.CommunityID)

	nC1, _ := ge.GetNode("C1.go")
	nC2, _ := ge.GetNode("C2.go")
	nC3, _ := ge.GetNode("C3.go")
	assert.Equal(t, nC1.CommunityID, nC2.CommunityID)
	assert.Equal(t, nC2.CommunityID, nC3.CommunityID)

	// Verify clusters are different
	assert.NotEqual(t, nA1.CommunityID, nB1.CommunityID)
	assert.NotEqual(t, nB1.CommunityID, nC1.CommunityID)
	assert.NotEqual(t, nA1.CommunityID, nC1.CommunityID)
}

func TestDetectCommunities_Sparse(t *testing.T) {
	ge := NewGraphEngine()

	// 10 isolated nodes
	var files []models.FileNode
	for i := 0; i < 10; i++ {
		files = append(files, models.FileNode{Path: fmt.Sprintf("file%d.go", i)})
	}

	err := ge.BuildGraph(files, nil, nil)
	assert.NoError(t, err)

	communities := ge.DetectCommunities()

	// In a sparse graph with no edges, every node is its own community
	assert.Equal(t, 10, len(communities))
}

func TestDetectCommunities_Dense(t *testing.T) {
	ge := NewGraphEngine()

	// 5 nodes, fully connected
	var files []models.FileNode
	for i := 0; i < 5; i++ {
		files = append(files, models.FileNode{Path: fmt.Sprintf("file%d.go", i)})
	}

	err := ge.BuildGraph(files, nil, nil)
	assert.NoError(t, err)

	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			assert.NoError(t, ge.AddCoChangeEdge(fmt.Sprintf("file%d.go", i), fmt.Sprintf("file%d.go", j)))
		}
	}

	communities := ge.DetectCommunities()

	// In a fully connected graph, they should all be in one community
	assert.Equal(t, 1, len(communities))

	// Verify all nodes have the same CommunityID
	firstNode, _ := ge.GetNode("file0.go")
	expectedCommID := firstNode.CommunityID
	for i := 1; i < 5; i++ {
		n, _ := ge.GetNode(fmt.Sprintf("file%d.go", i))
		assert.Equal(t, expectedCommID, n.CommunityID)
	}
}

func TestDetectCommunities_EdgeDensities(t *testing.T) {
	// Test with a larger graph and varying densities
	ge := NewGraphEngine()

	numNodes := 20
	var files []models.FileNode
	for i := 0; i < numNodes; i++ {
		files = append(files, models.FileNode{Path: fmt.Sprintf("node%d.go", i)})
	}
	err := ge.BuildGraph(files, nil, nil)
	assert.NoError(t, err)

	// Create two clusters with high internal density and low external density
	// Cluster 1: 0-9
	// Cluster 2: 10-19
	for i := 0; i < 10; i++ {
		for j := i + 1; j < 10; j++ {
			assert.NoError(t, ge.AddCoChangeEdge(fmt.Sprintf("node%d.go", i), fmt.Sprintf("node%d.go", j)))
		}
	}
	for i := 10; i < 20; i++ {
		for j := i + 1; j < 20; j++ {
			assert.NoError(t, ge.AddCoChangeEdge(fmt.Sprintf("node%d.go", i), fmt.Sprintf("node%d.go", j)))
		}
	}

	// Add one bridge edge
	assert.NoError(t, ge.AddCoChangeEdge("node0.go", "node10.go"))

	communities := ge.DetectCommunities()

	assert.Equal(t, 2, len(communities), "Should detect exactly 2 communities")

	n0, _ := ge.GetNode("node0.go")
	n9, _ := ge.GetNode("node9.go")
	n10, _ := ge.GetNode("node10.go")
	n19, _ := ge.GetNode("node19.go")

	assert.Equal(t, n0.CommunityID, n9.CommunityID)
	assert.Equal(t, n10.CommunityID, n19.CommunityID)
	assert.NotEqual(t, n0.CommunityID, n10.CommunityID)
}
