package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var reposListJSON bool

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
		if reposListJSON {
			return json.NewEncoder(os.Stdout).Encode(repos)
		}
		if len(repos) == 0 {
			fmt.Fprintln(os.Stderr, "No repositories indexed.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tPATH")
		for _, r := range repos {
			fmt.Fprintf(w, "%s\t%s\t%s\n", r.ID, r.Name, r.Path)
		}
		return w.Flush()
	},
}

func init() {
	reposListCmd.Flags().BoolVar(&reposListJSON, "json", false, "Output as JSON")
	reposCmd.AddCommand(reposListCmd)
}
