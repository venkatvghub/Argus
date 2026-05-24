package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var wikiCmd = &cobra.Command{
	Use:   "wiki",
	Short: "Wiki page commands",
}

var wikiListCmd = &cobra.Command{
	Use:   "list",
	Short: "List generated wiki pages for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		pages, err := instance.ListWikiPages(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("list wiki pages: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(pages)
	},
}

var wikiGetCmd = &cobra.Command{
	Use:   "get <page-id>",
	Short: "Get a wiki page by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		page, err := instance.GetWikiPage(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("get wiki page: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(page)
	},
}

func init() {
	wikiCmd.AddCommand(wikiListCmd)
	wikiCmd.AddCommand(wikiGetCmd)
}
