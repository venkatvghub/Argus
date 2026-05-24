package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var communityID int

var communityCmd = &cobra.Command{
	Use:   "community",
	Short: "Community graph commands",
}

var communityShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show nodes for a community",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		nodes, err := instance.GetCommunityGraph(cmd.Context(), rootRepoID, communityID)
		if err != nil {
			return fmt.Errorf("get community graph: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(nodes)
	},
}

func init() {
	communityShowCmd.Flags().IntVar(&communityID, "community-id", 0, "community ID to inspect (required)")
	_ = communityShowCmd.MarkFlagRequired("community-id")

	communityCmd.AddCommand(communityShowCmd)
}
