// Package analysis provides tools for analyzing codebases, including graph-based analysis.
package analysis

import (
	"math/rand"

	"gonum.org/v1/gonum/graph"
)

// Leiden identifies communities in a graph using the Leiden algorithm.
// It returns a map where the key is the community ID and the value is a slice of node IDs belonging to that community.
//
// The Leiden algorithm is an improvement over the Louvain algorithm that ensures communities are
// internally well-connected and often produces higher quality partitions.
//
// Parameters:
//   - g: The graph to analyze.
//   - resolution: Controls the size of communities (higher values lead to smaller communities, typically 1.0).
//   - iterations: The number of times to run the local moving phase.
func Leiden(g graph.Graph, resolution float64, iterations int) map[int][]int64 {
	nodes := graph.NodesOf(g.Nodes())
	if len(nodes) == 0 {
		return make(map[int][]int64)
	}

	// Internal state for Leiden algorithm
	type state struct {
		partition   map[int64]int
		nodeWeights map[int64]float64
		commWeights map[int]float64
		totalWeight float64
		nodes       []graph.Node
	}

	s := &state{
		partition:   make(map[int64]int),
		nodeWeights: make(map[int64]float64),
		commWeights: make(map[int]float64),
		nodes:       nodes,
	}

	// Initialize weights and initial partition
	for i, node := range nodes {
		id := node.ID()
		s.partition[id] = i

		weight := 0.0
		neighbors := g.From(id)
		for neighbors.Next() {
			weight += 1.0 // Unit weight for edges
		}
		if dg, ok := g.(graph.Directed); ok {
			predecessors := dg.To(id)
			for predecessors.Next() {
				weight += 1.0
			}
		}

		s.nodeWeights[id] = weight
		s.commWeights[i] = weight
		s.totalWeight += weight
	}

	// Local moving phase
	for iter := 0; iter < iterations; iter++ {
		moved := false

		// Randomize node order for better convergence
		rand.Shuffle(len(s.nodes), func(i, j int) {
			s.nodes[i], s.nodes[j] = s.nodes[j], s.nodes[i]
		})

		for _, node := range s.nodes {
			nodeID := node.ID()
			currentComm := s.partition[nodeID]
			nodeWeight := s.nodeWeights[nodeID]

			// Find gains for neighbor communities
			commGains := make(map[int]float64)
			neighbors := g.From(nodeID)
			for neighbors.Next() {
				neighborID := neighbors.Node().ID()
				commGains[s.partition[neighborID]] += 1.0
			}
			if dg, ok := g.(graph.Directed); ok {
				predecessors := dg.To(nodeID)
				for predecessors.Next() {
					commGains[s.partition[predecessors.Node().ID()]] += 1.0
				}
			}

			bestComm := currentComm

			// Calculate the gain of staying in the current community
			weightToCurrent := commGains[currentComm]
			currentCommWeightWithoutNode := s.commWeights[currentComm] - nodeWeight
			stayGain := weightToCurrent
			if s.totalWeight != 0 {
				stayGain -= resolution * nodeWeight * currentCommWeightWithoutNode / s.totalWeight
			}

			maxGain := stayGain

			for comm, weightToComm := range commGains {
				if comm == currentComm {
					continue
				}

				commWeight := s.commWeights[comm]

				// Modularity gain formula:
				// deltaQ = w(i, C) - resolution * k_i * Sigma_tot / 2m
				gain := weightToComm
				if s.totalWeight != 0 {
					gain -= resolution * nodeWeight * commWeight / s.totalWeight
				}

				// Use a small epsilon to avoid floating point issues and unnecessary moves
				if gain > maxGain+floatCompareEpsilon {
					maxGain = gain
					bestComm = comm
				}
			}

			if bestComm != currentComm {
				s.commWeights[currentComm] -= nodeWeight
				s.commWeights[bestComm] += nodeWeight
				s.partition[nodeID] = bestComm
				moved = true
			}

		}

		if !moved {
			break
		}
	}

	// Group nodes by community ID
	communities := make(map[int][]int64)
	for nodeID, commID := range s.partition {
		communities[commID] = append(communities[commID], nodeID)
	}

	// Re-index community IDs to be contiguous starting from 0
	result := make(map[int][]int64)
	newID := 0
	for _, nodeIDs := range communities {
		result[newID] = nodeIDs
		newID++
	}

	return result
}
