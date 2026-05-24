package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	symbolQuery string
	symbolType  string
)

var symbolsCmd = &cobra.Command{
	Use:   "symbols",
	Short: "Symbol search and listing commands",
}

var symbolsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search for symbols across all indexed repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		results, err := instance.SearchSymbols(cmd.Context(), symbolQuery, symbolType)
		if err != nil {
			return fmt.Errorf("search symbols: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(results)
	},
}

var symbolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all symbols for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		results, err := instance.GetRepoSymbols(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("get repo symbols: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(results)
	},
}

func init() {
	symbolsSearchCmd.Flags().StringVar(&symbolQuery, "query", "", "symbol name to search for")
	symbolsSearchCmd.Flags().StringVar(&symbolType, "type", "", "symbol type filter (function, class, variable, import)")

	symbolsCmd.AddCommand(symbolsSearchCmd)
	symbolsCmd.AddCommand(symbolsListCmd)
}
