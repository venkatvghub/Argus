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
		// Engine not in memory — return empty graph derived from DB files.
		dbFiles, dbErr := i.db.GetRepoFiles(ctx, repoID)
		if dbErr != nil || len(dbFiles) == 0 {
			// Try marker-derived files.
			i.mu.RLock()
			markers, hasMark := i.markers[repoID]
			i.mu.RUnlock()
			if hasMark {
				seen := make(map[string]struct{})
				nodeID := 1
				for _, m := range markers {
					if _, already := seen[m.File]; already {
						continue
					}
					seen[m.File] = struct{}{}
					resp.Nodes = append(resp.Nodes, GraphNode{
						ID:    fmt.Sprintf("%d", nodeID),
						Label: m.File,
						File:  m.File,
					})
					nodeID++
				}
			}
		} else {
			for idx, f := range dbFiles {
				resp.Nodes = append(resp.Nodes, GraphNode{
					ID:    fmt.Sprintf("%d", idx+1),
					Label: f.Path,
					File:  f.Path,
				})
			}
		}
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
