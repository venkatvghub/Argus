package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestRootCmd_IsNotNil(t *testing.T) {
	assert.NotNil(t, rootCmd)
}

func TestRootCmd_HasCorrectUse(t *testing.T) {
	assert.Equal(t, "argus", rootCmd.Use)
}

func TestRootCmd_HasCorrectShortDesc(t *testing.T) {
	assert.Equal(t, "Argus — deep codebase intelligence", rootCmd.Short)
}

func TestRootCmd_PersistentFlagsRegistered(t *testing.T) {
	tests := []struct {
		name     string
		flagName string
	}{
		{"log-level flag", "log-level"},
		{"repo-id flag", "repo-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			assert.NotNil(t, flag, "flag %s should exist", tt.flagName)
		})
	}
}

func TestRootCmd_HasPersistentPreRunE(t *testing.T) {
	assert.NotNil(t, rootCmd.PersistentPreRunE)
}

func TestRootCmd_HasPersistentPostRunE(t *testing.T) {
	assert.NotNil(t, rootCmd.PersistentPostRunE)
}

func TestRootCmd_AllSubcommandsRegistered(t *testing.T) {
	tests := []struct {
		name       string
		cmdName    string
		expectedUse string
	}{
		{"version command", "version", "version"},
		{"analyze command", "analyze", "analyze <repo-path>"},
		{"repos command", "repos", "repos"},
		{"symbols command", "symbols", "symbols"},
		{"markers command", "markers", "markers"},
		{"score command", "score", "score"},
		{"community command", "community", "community"},
		{"serve command", "serve", "serve"},
		{"init command", "init", "init <repo-path>"},
		{"jobs command", "jobs", "jobs"},
		{"wiki command", "wiki", "wiki"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := rootCmd.Find([]string{tt.cmdName})
			assert.NoError(t, err, "should find command %s", tt.cmdName)
			assert.NotNil(t, cmd, "command %s should not be nil", tt.cmdName)
			assert.Equal(t, tt.expectedUse, cmd.Use, "command Use field should match")
		})
	}
}

func TestReposCmd_HasListChild(t *testing.T) {
	found := false
	for _, cmd := range reposCmd.Commands() {
		if cmd.Use == "list" {
			found = true
			assert.Equal(t, "List all indexed repositories", cmd.Short)
			break
		}
	}
	assert.True(t, found, "repos command should have 'list' child")
}

func TestSymbolsCmd_HasSearchAndListChildren(t *testing.T) {
	expectedChildren := map[string]string{
		"search": "Search for symbols across all indexed repositories",
		"list":   "List all symbols for a repository",
	}

	actualChildren := make(map[string]string)
	for _, cmd := range symbolsCmd.Commands() {
		actualChildren[cmd.Use] = cmd.Short
	}

	for expectedUse, expectedShort := range expectedChildren {
		actualShort, found := actualChildren[expectedUse]
		assert.True(t, found, "symbols command should have '%s' child", expectedUse)
		assert.Equal(t, expectedShort, actualShort, "symbols %s short description should match", expectedUse)
	}
}

func TestMarkersCmd_HasFileAndRepoChildren(t *testing.T) {
	expectedChildren := map[string]string{
		"file": "Get biomarkers for a specific file",
		"repo": "Get all biomarkers for a repository",
	}

	actualChildren := make(map[string]string)
	for _, cmd := range markersCmd.Commands() {
		actualChildren[cmd.Use] = cmd.Short
	}

	for expectedUse, expectedShort := range expectedChildren {
		actualShort, found := actualChildren[expectedUse]
		assert.True(t, found, "markers command should have '%s' child", expectedUse)
		assert.Equal(t, expectedShort, actualShort, "markers %s short description should match", expectedUse)
	}
}

func TestScoreCmd_HasFileAndRepoChildren(t *testing.T) {
	expectedChildren := map[string]string{
		"file": "Get health score for a specific file",
		"repo": "Get aggregate health score for a repository",
	}

	actualChildren := make(map[string]string)
	for _, cmd := range scoreCmd.Commands() {
		actualChildren[cmd.Use] = cmd.Short
	}

	for expectedUse, expectedShort := range expectedChildren {
		actualShort, found := actualChildren[expectedUse]
		assert.True(t, found, "score command should have '%s' child", expectedUse)
		assert.Equal(t, expectedShort, actualShort, "score %s short description should match", expectedUse)
	}
}

func TestCommunityCmd_HasShowChild(t *testing.T) {
	found := false
	for _, cmd := range communityCmd.Commands() {
		if cmd.Use == "show" {
			found = true
			assert.Equal(t, "Show nodes for a community", cmd.Short)
			break
		}
	}
	assert.True(t, found, "community command should have 'show' child")
}

func TestServeCmd_HasRestAndMCPChildren(t *testing.T) {
	expectedChildren := map[string]string{
		"rest": "Start the REST HTTP server",
		"mcp":  "Start the MCP stdio server",
	}

	actualChildren := make(map[string]string)
	for _, cmd := range serveCmd.Commands() {
		actualChildren[cmd.Use] = cmd.Short
	}

	for expectedUse, expectedShort := range expectedChildren {
		actualShort, found := actualChildren[expectedUse]
		assert.True(t, found, "serve command should have '%s' child", expectedUse)
		assert.Equal(t, expectedShort, actualShort, "serve %s short description should match", expectedUse)
	}
}

func TestAnalyzeCmd_HasCorrectArgs(t *testing.T) {
	assert.Equal(t, "analyze <repo-path>", analyzeCmd.Use)
	assert.Equal(t, "Analyze a repository", analyzeCmd.Short)
}

func TestAnalyzeCmd_HasWaitFlag(t *testing.T) {
	flag := analyzeCmd.Flags().Lookup("wait")
	assert.NotNil(t, flag, "analyze command should have 'wait' flag")
	assert.Equal(t, "bool", flag.Value.Type())
}

func TestSymbolsSearchCmd_HasQueryAndTypeFlags(t *testing.T) {
	flagTests := []struct {
		flagName    string
		expectedType string
	}{
		{"query", "string"},
		{"type", "string"},
	}

	for _, tt := range flagTests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := symbolsSearchCmd.Flags().Lookup(tt.flagName)
			assert.NotNil(t, flag, "symbols search should have %s flag", tt.flagName)
			assert.Equal(t, tt.expectedType, flag.Value.Type())
		})
	}
}

func TestMarkersFileCmd_HasFileFlag(t *testing.T) {
	flag := markersFileCmd.Flags().Lookup("file")
	assert.NotNil(t, flag, "markers file should have 'file' flag")
	assert.Equal(t, "string", flag.Value.Type())
}

func TestScoreFileCmd_HasFileFlag(t *testing.T) {
	flag := scoreFileCmd.Flags().Lookup("file")
	assert.NotNil(t, flag, "score file should have 'file' flag")
	assert.Equal(t, "string", flag.Value.Type())
}

func TestCommunityShowCmd_HasCommunityIDFlag(t *testing.T) {
	flag := communityShowCmd.Flags().Lookup("community-id")
	assert.NotNil(t, flag, "community show should have 'community-id' flag")
	assert.Equal(t, "int", flag.Value.Type())
}

func TestServeRESTCmd_HasAddrFlag(t *testing.T) {
	flag := serveRESTCmd.Flags().Lookup("addr")
	assert.NotNil(t, flag, "serve rest should have 'addr' flag")
	assert.Equal(t, "string", flag.Value.Type())
}

func TestRootCmd_CommandsHaveShortDescriptions(t *testing.T) {
	// All commands should have non-empty Short descriptions
	walkCommands(t, rootCmd, func(cmd *cobra.Command) {
		if len(cmd.Commands()) > 0 || cmd.RunE != nil {
			assert.NotEmpty(t, cmd.Short, "command %s should have a Short description", cmd.Use)
		}
	})
}

// walkCommands recursively visits all commands and their children
func walkCommands(t *testing.T, cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, child := range cmd.Commands() {
		walkCommands(t, child, fn)
	}
}
