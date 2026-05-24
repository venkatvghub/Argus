package argus

import (
	"context"
	"fmt"

	"github.com/venkatvghub/argus/pkg/analysis"
)

// GraphNode is the REST API response shape for a code graph node.
type GraphNode struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	File      string  `json:"file"`
	Symbol    string  `json:"symbol"`
	Community int     `json:"community"`
	PageRank  float64 `json:"page_rank"`
}

// GraphExportResponse is the full graph for the dashboard.
type GraphExportResponse struct {
	RepoID string      `json:"repo_id"`
	Nodes  []GraphNode `json:"nodes"`
	Edges  []struct {
		From string `json:"from"`
		To   string `json:"to"`
		Type string `json:"type"`
	} `json:"edges"`
}

// CommunityInfo summarises one graph community.
type CommunityInfo struct {
	ID    int      `json:"id"`
	Size  int      `json:"size"`
	Files []string `json:"files"`
}

// GetGraphExport returns the full graph structure for a repository.
func (i *Instance) GetGraphExport(ctx context.Context, repoID string) (GraphExportResponse, error) {
	i.mu.RLock()
	engine, ok := i.engines[repoID]
	i.mu.RUnlock()

	resp := GraphExportResponse{
		RepoID: repoID,
		Nodes:  []GraphNode{},
		Edges:  []struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		}{},
	}
	if !ok {
		return resp, nil
	}

	for _, n := range engine.GetNodes() {
		var file, symbol string
		if n.InternalType() == analysis.NodeTypeFile {
			if f := n.File(); f != nil {
				file = f.Path
			}
		} else {
			if s := n.Symbol(); s != nil {
				symbol = s.Name
				file = s.FilePath
			}
		}
		id := fmt.Sprintf("%d", n.ID())
		resp.Nodes = append(resp.Nodes, GraphNode{
			ID:        id,
			Label:     n.Name,
			File:      file,
			Symbol:    symbol,
			Community: n.CommunityID,
			PageRank:  n.PageRank,
		})
	}

	for _, edge := range engine.Edges() {
		te, ok := edge.(analysis.TypedEdge)
		if !ok {
			continue
		}
		resp.Edges = append(resp.Edges, struct {
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		}{
			From: fmt.Sprintf("%d", te.From().ID()),
			To:   fmt.Sprintf("%d", te.To().ID()),
			Type: te.Type(),
		})
	}

	return resp, nil
}

// GetCommunities returns community summaries for a repository.
func (i *Instance) GetCommunities(ctx context.Context, repoID string) ([]CommunityInfo, error) {
	i.mu.RLock()
	engine, ok := i.engines[repoID]
	i.mu.RUnlock()

	if !ok {
		return []CommunityInfo{}, nil
	}

	byID := make(map[int][]string)
	for _, n := range engine.GetNodes() {
		if n.InternalType() == analysis.NodeTypeFile {
			if f := n.File(); f != nil {
				byID[n.CommunityID] = append(byID[n.CommunityID], f.Path)
			}
		}
	}

	result := make([]CommunityInfo, 0, len(byID))
	for id, files := range byID {
		result = append(result, CommunityInfo{
			ID:    id,
			Size:  len(files),
			Files: files,
		})
	}
	return result, nil
}
