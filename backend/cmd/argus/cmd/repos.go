package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "Repository management commands",
}

var reposListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all indexed repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		repos, err := instance.ListRepositories(cmd.Context())
		if err != nil {
			return fmt.Errorf("list repositories: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(repos)
	},
}

func init() {
	reposCmd.AddCommand(reposListCmd)
}
