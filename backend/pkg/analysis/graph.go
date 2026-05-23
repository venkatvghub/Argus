// Package analysis provides tools for analyzing codebases, including graph-based analysis.
package analysis

import (
	"fmt"

	"github.com/venkatvghub/argus/pkg/models"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/network"
	"gonum.org/v1/gonum/graph/simple"
)

// NodeType identifies whether a node is a file or a symbol.
type NodeType string

const (
	// NodeTypeFile represents a source file node.
	NodeTypeFile NodeType = "file"
	// NodeTypeSymbol represents a code symbol node (function, class, etc.).
	NodeTypeSymbol NodeType = "symbol"
)

// Node represents a single node in the graph, wrapping the gonum simple.Node.
// It can represent either a file or a symbol.
type Node struct {
	id int64
	// Name is the display name of the node (file path or symbol name).
	Name string
	// Type is the category of the node (e.g., "file", "function").
	Type string
	// PageRank is the calculated importance score for the node.
	PageRank float64
	// CommunityID is the identifier of the community this node belongs to.
	CommunityID int
	nodeType    NodeType
	file        *models.FileNode
	symbol      *models.Symbol
}

// ID returns the unique identifier for the node.
func (n *Node) ID() int64 {
	return n.id
}

// InternalType returns the internal NodeType (file or symbol).
func (n *Node) InternalType() NodeType {
	return n.nodeType
}

// File returns the associated FileNode if the node is of type NodeTypeFile.
func (n *Node) File() *models.FileNode {
	return n.file
}

// Symbol returns the associated Symbol if the node is of type NodeTypeSymbol.
func (n *Node) Symbol() *models.Symbol {
	return n.symbol
}

// TypedEdge represents a directed relationship between two nodes with a specific type.
type TypedEdge struct {
	from    graph.Node
	to      graph.Node
	relType string
}

// From returns the source node of the edge.
func (e TypedEdge) From() graph.Node { return e.from }

// To returns the destination node of the edge.
func (e TypedEdge) To() graph.Node { return e.to }

// ReversedEdge returns the edge reversal of the receiver.
func (e TypedEdge) ReversedEdge() graph.Edge {
	return TypedEdge{from: e.to, to: e.from, relType: e.relType}
}

// Type returns the relationship type of the edge.
func (e TypedEdge) Type() string { return e.relType }

// GraphEngine provides the core graph analytics using gonum.
// It builds and maintains a directed graph of files and symbols to identify
// important components and relationships.
type GraphEngine struct {
	g     *simple.DirectedGraph
	nodes map[string]*Node
}

// NewGraphEngine initializes a new GraphEngine with an empty graph.
func NewGraphEngine() *GraphEngine {
	return &GraphEngine{
		g:     simple.NewDirectedGraph(),
		nodes: make(map[string]*Node),
	}
}

// fileKey generates a unique key for a file node in the internal map.
func fileKey(path string) string {
	return path
}

// symbolKey generates a unique key for a symbol node in the internal map.
func symbolKey(filePath, name string) string {
	return fmt.Sprintf("%s:%s", filePath, name)
}

// BuildGraph populates the graph using the provided files and symbols.
// It automatically creates "contains" edges from files to their constituent symbols,
// adds provided relationships, and calculates PageRank for all nodes.
func (ge *GraphEngine) BuildGraph(files []models.FileNode, symbols []models.Symbol, relations []models.Relation) error {
	// Add File Nodes
	for i := range files {
		f := files[i]
		key := fileKey(f.Path)
		if _, ok := ge.nodes[key]; ok {
			continue
		}
		node := &Node{
			id:       ge.g.NewNode().ID(),
			Name:     f.Path,
			Type:     "file",
			nodeType: NodeTypeFile,
			file:     &f,
		}
		ge.nodes[key] = node
		ge.g.AddNode(node)
	}

	// Add Symbol Nodes and "contains" edges
	for i := range symbols {
		s := symbols[i]
		key := symbolKey(s.FilePath, s.Name)
		if _, ok := ge.nodes[key]; ok {
			continue
		}
		node := &Node{
			id:       ge.g.NewNode().ID(),
			Name:     s.Name,
			Type:     string(s.Type),
			nodeType: NodeTypeSymbol,
			symbol:   &s,
		}
		ge.nodes[key] = node
		ge.g.AddNode(node)

		// Edge from File to Symbol ("contains")
		if fileNode, ok := ge.nodes[fileKey(s.FilePath)]; ok {
			ge.g.SetEdge(TypedEdge{from: fileNode, to: node, relType: "contains"})
		}
	}

	// Add provided relations
	ge.AddRelations(relations)

	// Calculate PageRank so it's available in nodes
	ge.CalculatePageRank()

	return nil
}

// AddRelations adds multiple relationships to the graph.
func (ge *GraphEngine) AddRelations(relations []models.Relation) {
	for _, rel := range relations {
		fromNode, ok1 := ge.nodes[rel.From]
		toNode, ok2 := ge.nodes[rel.To]
		if ok1 && ok2 {
			ge.g.SetEdge(TypedEdge{from: fromNode, to: toNode, relType: rel.Type})
		}
	}
}

// GetNode returns a node by its key (path for files, path:name for symbols).
func (ge *GraphEngine) GetNode(key string) (*Node, bool) {
	n, ok := ge.nodes[key]
	return n, ok
}

// GetNodeByPath returns the Node for the given file path and a boolean indicating presence.
func (ge *GraphEngine) GetNodeByPath(path string) (*Node, bool) {
	n, ok := ge.nodes[fileKey(path)]
	return n, ok
}

// AddCallEdge adds a directed "calls" relationship between two symbols.
// It returns an error if either node does not exist in the graph.
func (ge *GraphEngine) AddCallEdge(fromFilePath, fromSymbol, toFilePath, toSymbol string) error {
	fromKey := symbolKey(fromFilePath, fromSymbol)
	toKey := symbolKey(toFilePath, toSymbol)

	fromNode, ok1 := ge.nodes[fromKey]
	toNode, ok2 := ge.nodes[toKey]

	if !ok1 || !ok2 {
		return fmt.Errorf("node not found: %s or %s", fromKey, toKey)
	}

	ge.g.SetEdge(TypedEdge{from: fromNode, to: toNode, relType: "calls"})
	return nil
}

// AddCoChangeEdge adds a bidirectional "co-change" relationship between two files.
// It returns an error if either file node does not exist in the graph.
func (ge *GraphEngine) AddCoChangeEdge(filePath1, filePath2 string) error {
	key1 := fileKey(filePath1)
	key2 := fileKey(filePath2)

	n1, ok1 := ge.nodes[key1]
	n2, ok2 := ge.nodes[key2]

	if !ok1 || !ok2 {
		return fmt.Errorf("node not found: %s or %s", key1, key2)
	}

	ge.g.SetEdge(TypedEdge{from: n1, to: n2, relType: "co-change"})
	ge.g.SetEdge(TypedEdge{from: n2, to: n1, relType: "co-change"})
	return nil
}

// CalculatePageRank computes the PageRank for all nodes in the graph and updates their PageRank field.
// It returns a map of node IDs to their respective PageRank scores.
func (ge *GraphEngine) CalculatePageRank() map[int64]float64 {
	if ge.g.Nodes().Len() == 0 {
		return make(map[int64]float64)
	}
	// 0.85 is the standard damping factor, 1e-6 is the convergence tolerance.
	scores := network.PageRank(ge.g, 0.85, 1e-6)

	// Update nodes with scores
	for id, score := range scores {
		if gNode := ge.g.Node(id); gNode != nil {
			if n, ok := gNode.(*Node); ok {
				n.PageRank = score
			}
		}
	}

	return scores
}

// GetNodes returns all nodes currently present in the graph.
func (ge *GraphEngine) GetNodes() []*Node {
	nodes := ge.g.Nodes()
	var result []*Node
	for nodes.Next() {
		if n, ok := nodes.Node().(*Node); ok {
			result = append(result, n)
		}
	}
	return result
}

// DetectCommunities identifies communities of nodes using the Leiden algorithm.
// It updates the CommunityID of each node and returns a map of community IDs to lists of node IDs.
func (ge *GraphEngine) DetectCommunities() map[int][]int64 {
	// Call the Leiden implementation
	communities := Leiden(ge.g, 1.0, 10)

	// Update nodes with community IDs
	result := make(map[int][]int64)
	for communityID, nodeIDs := range communities {
		result[communityID] = nodeIDs
		for _, nodeID := range nodeIDs {
			if gNode := ge.g.Node(nodeID); gNode != nil {
				if n, ok := gNode.(*Node); ok {
					n.CommunityID = communityID
				}
			}
		}
	}

	return result
}

// Edges returns all edges in the graph.
func (ge *GraphEngine) Edges() []graph.Edge {
	edges := ge.g.Edges()
	var result []graph.Edge
	for edges.Next() {
		result = append(result, edges.Edge())
	}
	return result
}
