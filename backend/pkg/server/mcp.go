package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/constants"
)

type MCPServer struct {
	argus *argus.Instance
}

func NewMCPServer(argus *argus.Instance) *MCPServer {
	return &MCPServer{argus: argus}
}

func (s *MCPServer) Run() error {
	appName := "argus"
	if cfg := s.argus.Config(); cfg != nil && cfg.AppName != "" {
		appName = cfg.AppName
	}
	mcpSrv := server.NewMCPServer(appName, constants.APIVersion)

	mcpSrv.AddTool(mcp.NewTool("list_repos", mcp.WithDescription("Lists indexed repositories")), s.listReposHandler)

	mcpSrv.AddTool(mcp.NewTool("index_repo",
		mcp.WithDescription("Triggers a new ingestion/analysis run for a given path"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Local path to the repository")),
	), s.indexRepoHandler)

	mcpSrv.AddTool(mcp.NewTool("search_symbols",
		mcp.WithDescription("Search for symbols by name or type across repositories"),
		mcp.WithString("query", mcp.Description("Symbol name to search for")),
		mcp.WithString("type", mcp.Description("Symbol type (function, class, variable, import)")),
	), s.searchSymbolsHandler)

	mcpSrv.AddTool(mcp.NewTool("get_file_markers",
		mcp.WithDescription("Retrieve health markers and biomarkers for a specific file"),
		mcp.WithString("repo_id", mcp.Required(), mcp.Description("ID of the repository")),
		mcp.WithString("file_path", mcp.Required(), mcp.Description("Path to the file relative to repo root")),
	), s.getFileMarkersHandler)

	mcpSrv.AddTool(mcp.NewTool("get_community_graph",
		mcp.WithDescription("Return nodes for a specific community ID"),
		mcp.WithString("repo_id", mcp.Required(), mcp.Description("ID of the repository")),
		mcp.WithNumber("community_id", mcp.Required(), mcp.Description("ID of the community")),
	), s.getCommunityGraphHandler)

	mcpSrv.AddTool(mcp.NewTool("get_file_score",
		mcp.WithDescription("Retrieve the computed health score for a specific file"),
		mcp.WithString("repo_id", mcp.Required(), mcp.Description("ID of the repository")),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path to the file relative to repo root")),
	), s.getFileScoreHandler)

	mcpSrv.AddTool(mcp.NewTool("get_repo_score",
		mcp.WithDescription("Retrieve the aggregate health score for a repository"),
		mcp.WithString("repo_id", mcp.Required(), mcp.Description("ID of the repository")),
	), s.getRepoScoreHandler)

	return server.ServeStdio(mcpSrv)
}

func (s *MCPServer) listReposHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repos, err := s.argus.ListRepositories(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%v", repos)), nil
}

func (s *MCPServer) indexRepoHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := req.GetString("path", "")
	jobID, err := s.argus.Analyze(ctx, path)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Analysis queued for %s (jobID: %s)", path, jobID)), nil
}

func (s *MCPServer) searchSymbolsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	symType := req.GetString("type", "")
	symbols, err := s.argus.SearchSymbols(ctx, query, symType)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%v", symbols)), nil
}

func (s *MCPServer) getFileMarkersHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoID := req.GetString("repo_id", "")
	filePath := req.GetString("file_path", "")
	markers, err := s.argus.GetFileMarkers(ctx, repoID, filePath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%v", markers)), nil
}

func (s *MCPServer) getCommunityGraphHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoID := req.GetString("repo_id", "")
	commID := req.GetInt("community_id", 0)
	nodes, err := s.argus.GetCommunityGraph(ctx, repoID, commID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%v", nodes)), nil
}

func (s *MCPServer) getFileScoreHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoID := req.GetString("repo_id", "")
	filePath := req.GetString("path", "")
	score, err := s.argus.GetFileScore(ctx, repoID, filePath)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(score)
	return mcp.NewToolResultText(string(b)), nil
}

type repoScoreResponse struct {
	Score float64 `json:"score"`
}

func (s *MCPServer) getRepoScoreHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoID := req.GetString("repo_id", "")
	score, err := s.argus.GetRepoScore(ctx, repoID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return mcp.NewToolResultError("repository score is not finite"), nil
	}
	result, err := mcp.NewToolResultJSON(repoScoreResponse{Score: score})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return result, nil
}
