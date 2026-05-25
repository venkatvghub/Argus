package argus

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/venkatvghub/argus/pkg/analysis"
)

// GraphNodeResponse is the REST API response shape for a code graph node.
// Field names match the frontend GraphNodeResponse type.
type GraphNodeResponse struct {
	NodeID       string  `json:"node_id"`
	NodeType     string  `json:"node_type"`
	Language     string  `json:"language"`
	SymbolCount  int     `json:"symbol_count"`
	PageRank     float64 `json:"pagerank"`
	Betweenness  float64 `json:"betweenness"`
	CommunityID  int     `json:"community_id"`
	IsTest       bool    `json:"is_test"`
	IsEntryPoint bool    `json:"is_entry_point"`
	HasDoc       bool    `json:"has_doc"`
}

// GraphEdgeResponse is the REST API response shape for a graph edge.
type GraphEdgeResponse struct {
	Source        string   `json:"source"`
	Target        string   `json:"target"`
	ImportedNames []string `json:"imported_names,omitempty"`
}

// GraphExportResponse is the full graph for the dashboard.
type GraphExportResponse struct {
	RepoID string              `json:"repo_id"`
	Nodes  []GraphNodeResponse `json:"nodes"`
	Links  []GraphEdgeResponse `json:"links"`
}

// CommunityInfo summarises one graph community.
type CommunityInfo struct {
	ID    int      `json:"id"`
	Size  int      `json:"size"`
	Files []string `json:"files"`
}

// isTestPath returns true for common test file naming conventions.
func isTestPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.tsx") {
		return true
	}
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/test/") ||
		strings.Contains(normalized, "/tests/") ||
		strings.Contains(normalized, "/__tests__/")
}

// GetGraphExport returns the full graph structure for a repository.
func (i *Instance) GetGraphExport(ctx context.Context, repoID string) (GraphExportResponse, error) {
	i.mu.RLock()
	engine, ok := i.engines[repoID]
	i.mu.RUnlock()

	resp := GraphExportResponse{
		RepoID: repoID,
		Nodes:  []GraphNodeResponse{},
		Links:  []GraphEdgeResponse{},
	}

	if !ok {
		// Engine not in memory — derive graph from DB files.
		dbFiles, dbErr := i.db.GetRepoFiles(ctx, repoID)
		if dbErr != nil || len(dbFiles) == 0 {
			// Fall back to marker-derived file paths.
			i.mu.RLock()
			markers, hasMark := i.markers[repoID]
			i.mu.RUnlock()
			if hasMark {
				seen := make(map[string]struct{})
				for _, m := range markers {
					if _, already := seen[m.File]; already {
						continue
					}
					seen[m.File] = struct{}{}
					resp.Nodes = append(resp.Nodes, GraphNodeResponse{
						NodeID:   m.File,
						NodeType: "file",
						IsTest:   isTestPath(m.File),
					})
				}
			}
		} else {
			for _, f := range dbFiles {
				resp.Nodes = append(resp.Nodes, GraphNodeResponse{
					NodeID:   f.Path,
					NodeType: "file",
					Language: f.Language,
					IsTest:   isTestPath(f.Path),
				})
			}
		}
		return resp, nil
	}

	// Engine is in memory — use analysis graph.
	// Only emit file nodes; symbol nodes are aggregated into symbol_count.
	symbolCount := make(map[string]int) // file path → count of symbols
	for _, n := range engine.GetNodes() {
		if n.InternalType() == analysis.NodeTypeSymbol {
			if s := n.Symbol(); s != nil {
				symbolCount[s.FilePath]++
			}
		}
	}

	for _, n := range engine.GetNodes() {
		if n.InternalType() != analysis.NodeTypeFile {
			continue
		}
		f := n.File()
		if f == nil {
			continue
		}
		resp.Nodes = append(resp.Nodes, GraphNodeResponse{
			NodeID:      f.Path,
			NodeType:    "file",
			Language:    f.Language,
			SymbolCount: symbolCount[f.Path],
			PageRank:    n.PageRank,
			CommunityID: n.CommunityID,
			IsTest:      isTestPath(f.Path),
		})
	}

	// Emit edges between file nodes only.
	for _, edge := range engine.Edges() {
		te, ok := edge.(analysis.TypedEdge)
		if !ok {
			continue
		}
		fromNode, ok1 := te.From().(*analysis.Node)
		toNode, ok2 := te.To().(*analysis.Node)
		if !ok1 || !ok2 {
			continue
		}
		if fromNode.InternalType() != analysis.NodeTypeFile || toNode.InternalType() != analysis.NodeTypeFile {
			continue
		}
		fromFile := fromNode.File()
		toFile := toNode.File()
		if fromFile == nil || toFile == nil {
			continue
		}
		resp.Links = append(resp.Links, GraphEdgeResponse{
			Source: fromFile.Path,
			Target: toFile.Path,
		})
	}

	return resp, nil
}

// ModuleNodeResponse is the API shape for a module-level graph node.
type ModuleNodeResponse struct {
	ModuleID       string  `json:"module_id"`
	FileCount      int     `json:"file_count"`
	SymbolCount    int     `json:"symbol_count"`
	AvgPageRank    float64 `json:"avg_pagerank"`
	DocCoveragePct float64 `json:"doc_coverage_pct"`
}

// ModuleEdgeResponse is the API shape for a cross-module edge.
type ModuleEdgeResponse struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	EdgeCount int    `json:"edge_count"`
}

// ModuleGraphResponse is the full module-level graph.
type ModuleGraphResponse struct {
	Nodes []ModuleNodeResponse `json:"nodes"`
	Edges []ModuleEdgeResponse `json:"edges"`
}

// modulePrefix returns the top-level directory component of a file path.
func modulePrefix(path string) string {
	for i, c := range path {
		if (c == '/' || c == '\\') && i > 0 {
			return path[:i]
		}
	}
	return path // single-component path → the file is its own module
}

// GetModuleGraph aggregates file-level graph data into module nodes and cross-module edges.
func (i *Instance) GetModuleGraph(ctx context.Context, repoID string) (ModuleGraphResponse, error) {
	export, err := i.GetGraphExport(ctx, repoID)
	if err != nil {
		return ModuleGraphResponse{Nodes: []ModuleNodeResponse{}, Edges: []ModuleEdgeResponse{}}, err
	}

	type modAcc struct {
		fileCount   int
		symbolCount int
		pagerankSum float64
		docCount    int
	}
	mods := make(map[string]*modAcc)
	fileToMod := make(map[string]string)

	for _, n := range export.Nodes {
		mod := modulePrefix(n.NodeID)
		fileToMod[n.NodeID] = mod
		acc := mods[mod]
		if acc == nil {
			acc = &modAcc{}
			mods[mod] = acc
		}
		acc.fileCount++
		acc.symbolCount += n.SymbolCount
		acc.pagerankSum += n.PageRank
		if n.HasDoc {
			acc.docCount++
		}
	}

	nodes := make([]ModuleNodeResponse, 0, len(mods))
	for id, acc := range mods {
		avg := 0.0
		if acc.fileCount > 0 {
			avg = acc.pagerankSum / float64(acc.fileCount)
		}
		docPct := 0.0
		if acc.fileCount > 0 {
			docPct = float64(acc.docCount) / float64(acc.fileCount) * 100
		}
		nodes = append(nodes, ModuleNodeResponse{
			ModuleID:       id,
			FileCount:      acc.fileCount,
			SymbolCount:    acc.symbolCount,
			AvgPageRank:    avg,
			DocCoveragePct: docPct,
		})
	}

	// Aggregate cross-module edges.
	edgeCounts := make(map[[2]string]int)
	for _, e := range export.Links {
		src := fileToMod[e.Source]
		tgt := fileToMod[e.Target]
		if src != "" && tgt != "" && src != tgt {
			edgeCounts[[2]string{src, tgt}]++
		}
	}
	edges := make([]ModuleEdgeResponse, 0, len(edgeCounts))
	for pair, count := range edgeCounts {
		edges = append(edges, ModuleEdgeResponse{
			Source:    pair[0],
			Target:    pair[1],
			EdgeCount: count,
		})
	}

	return ModuleGraphResponse{Nodes: nodes, Edges: edges}, nil
}

// C4 response types — mirror the frontend's types.ts shapes exactly.

type C4System struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type C4Container struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Path         string `json:"path"`
	Language     string `json:"language"`
	FileCount    int    `json:"file_count"`
	SymbolCount  int    `json:"symbol_count"`
	HotspotCount int    `json:"hotspot_count"`
	DeadCount    int    `json:"dead_count"`
}

type C4Component struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	ContainerID string `json:"container_id"`
	FileCount   int    `json:"file_count"`
	SymbolCount int    `json:"symbol_count"`
}

type C4Relation struct {
	SourceID  string   `json:"source_id"`
	TargetID  string   `json:"target_id"`
	Label     string   `json:"label"`
	EdgeCount int      `json:"edge_count"`
	EdgeTypes []string `json:"edge_types"`
}

type C4L1Response struct {
	System          C4System      `json:"system"`
	People          []any         `json:"people"`
	ExternalSystems []any         `json:"external_systems"`
	Relations       []C4Relation  `json:"relations"`
}

type C4L2Response struct {
	Containers      []C4Container `json:"containers"`
	ExternalSystems []any         `json:"external_systems"`
	Relations       []C4Relation  `json:"relations"`
}

type C4L3Response struct {
	Container       *C4Container   `json:"container"`
	Components      []C4Component  `json:"components"`
	ExternalSystems []any          `json:"external_systems"`
	Relations       []C4Relation   `json:"relations"`
}

const c4HotspotChurnThreshold = 10

// GetC4L1 returns the system-context (L1) C4 view for a repository.
func (i *Instance) GetC4L1(ctx context.Context, repoID string) (C4L1Response, error) {
	repo, err := i.GetRepository(ctx, repoID)
	name := repoID
	if err == nil {
		name = repo.Name
	}
	return C4L1Response{
		System:          C4System{ID: repoID, Name: name, Description: ""},
		People:          []any{},
		ExternalSystems: []any{},
		Relations:       []C4Relation{},
	}, nil
}

// GetC4L2 returns the container (L2) C4 view — top-level directories as containers.
func (i *Instance) GetC4L2(ctx context.Context, repoID string) (C4L2Response, error) {
	graph, err := i.GetGraphExport(ctx, repoID)
	if err != nil {
		return C4L2Response{Containers: []C4Container{}, ExternalSystems: []any{}, Relations: []C4Relation{}}, err
	}

	files, _ := i.GetRepoFiles(ctx, repoID)
	fileChurn := make(map[string]int, len(files))
	for _, f := range files {
		fileChurn[f.Path] = f.Churn
	}

	type modAcc struct {
		symbolCount  int
		hotspotCount int
		langCount    map[string]int
	}
	mods := make(map[string]*modAcc)
	fileToMod := make(map[string]string)

	for _, n := range graph.Nodes {
		mod := modulePrefix(n.NodeID)
		fileToMod[n.NodeID] = mod
		acc := mods[mod]
		if acc == nil {
			acc = &modAcc{langCount: make(map[string]int)}
			mods[mod] = acc
		}
		acc.symbolCount += n.SymbolCount
		if churn, ok := fileChurn[n.NodeID]; ok && churn >= c4HotspotChurnThreshold {
			acc.hotspotCount++
		}
		if n.Language != "" {
			acc.langCount[n.Language]++
		}
	}

	containers := make([]C4Container, 0, len(mods))
	fileCountByMod := make(map[string]int)
	for _, n := range graph.Nodes {
		fileCountByMod[fileToMod[n.NodeID]]++
	}
	for id, acc := range mods {
		topLang := ""
		maxCount := 0
		for lang, cnt := range acc.langCount {
			if cnt > maxCount || (cnt == maxCount && lang < topLang) {
				topLang = lang
				maxCount = cnt
			}
		}
		containers = append(containers, C4Container{
			ID:           id,
			Name:         filepath.Base(id),
			Path:         id,
			Language:     topLang,
			FileCount:    fileCountByMod[id],
			SymbolCount:  acc.symbolCount,
			HotspotCount: acc.hotspotCount,
			DeadCount:    0,
		})
	}

	edgeCounts := make(map[[2]string]int)
	for _, e := range graph.Links {
		src, tgt := fileToMod[e.Source], fileToMod[e.Target]
		if src != "" && tgt != "" && src != tgt {
			edgeCounts[[2]string{src, tgt}]++
		}
	}
	relations := make([]C4Relation, 0, len(edgeCounts))
	for pair, count := range edgeCounts {
		relations = append(relations, C4Relation{
			SourceID:  pair[0],
			TargetID:  pair[1],
			Label:     "imports",
			EdgeCount: count,
			EdgeTypes: []string{"import"},
		})
	}

	return C4L2Response{
		Containers:      containers,
		ExternalSystems: []any{},
		Relations:       relations,
	}, nil
}

// GetC4L3 returns the component (L3) C4 view for a specific container (directory).
func (i *Instance) GetC4L3(ctx context.Context, repoID, containerID string) (C4L3Response, error) {
	empty := C4L3Response{Container: nil, Components: []C4Component{}, ExternalSystems: []any{}, Relations: []C4Relation{}}

	graph, err := i.GetGraphExport(ctx, repoID)
	if err != nil {
		return empty, err
	}

	files, _ := i.GetRepoFiles(ctx, repoID)
	fileChurn := make(map[string]int, len(files))
	for _, f := range files {
		fileChurn[f.Path] = f.Churn
	}

	var container *C4Container
	symbolCountByMod := make(map[string]int)
	fileCountByMod := make(map[string]int)
	hotspotByMod := make(map[string]int)
	langCountByMod := make(map[string]map[string]int)

	for _, n := range graph.Nodes {
		mod := modulePrefix(n.NodeID)
		symbolCountByMod[mod] += n.SymbolCount
		fileCountByMod[mod]++
		if churn, ok := fileChurn[n.NodeID]; ok && churn >= c4HotspotChurnThreshold {
			hotspotByMod[mod]++
		}
		if langCountByMod[mod] == nil {
			langCountByMod[mod] = make(map[string]int)
		}
		if n.Language != "" {
			langCountByMod[mod][n.Language]++
		}
	}

	if _, exists := symbolCountByMod[containerID]; !exists {
		return empty, nil
	}
	topLang := ""
	maxCount := 0
	for lang, cnt := range langCountByMod[containerID] {
		if cnt > maxCount || (cnt == maxCount && lang < topLang) {
			topLang = lang
			maxCount = cnt
		}
	}
	container = &C4Container{
		ID:           containerID,
		Name:         filepath.Base(containerID),
		Path:         containerID,
		Language:     topLang,
		FileCount:    fileCountByMod[containerID],
		SymbolCount:  symbolCountByMod[containerID],
		HotspotCount: hotspotByMod[containerID],
		DeadCount:    0,
	}

	components := make([]C4Component, 0)
	fileSymbols := make(map[string]int)
	for _, n := range graph.Nodes {
		if modulePrefix(n.NodeID) == containerID {
			fileSymbols[n.NodeID] = n.SymbolCount
		}
	}
	for filePath, symCount := range fileSymbols {
		components = append(components, C4Component{
			ID:          filePath,
			Name:        filepath.Base(filePath),
			Path:        filePath,
			ContainerID: containerID,
			FileCount:   1,
			SymbolCount: symCount,
		})
	}

	// File-to-file edges within this container.
	edgeCounts := make(map[[2]string]int)
	for _, e := range graph.Links {
		srcMod := modulePrefix(e.Source)
		tgtMod := modulePrefix(e.Target)
		if srcMod == containerID && tgtMod == containerID && e.Source != e.Target {
			edgeCounts[[2]string{e.Source, e.Target}]++
		}
	}
	relations := make([]C4Relation, 0, len(edgeCounts))
	for pair, count := range edgeCounts {
		relations = append(relations, C4Relation{
			SourceID:  pair[0],
			TargetID:  pair[1],
			Label:     "imports",
			EdgeCount: count,
			EdgeTypes: []string{"import"},
		})
	}

	return C4L3Response{
		Container:       container,
		Components:      components,
		ExternalSystems: []any{},
		Relations:       relations,
	}, nil
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
