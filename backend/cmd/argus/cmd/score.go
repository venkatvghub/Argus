package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var scoreFilePath string

var scoreCmd = &cobra.Command{
	Use:   "score",
	Short: "Scoring commands",
}

var scoreFileCmd = &cobra.Command{
	Use:   "file",
	Short: "Get health score for a specific file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		if scoreFilePath == "" {
			return fmt.Errorf("--file is required")
		}
		fs, err := instance.GetFileScore(cmd.Context(), rootRepoID, scoreFilePath)
		if err != nil {
			return fmt.Errorf("get file score: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(fs)
	},
}

var scoreRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Get aggregate health score for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		score, err := instance.GetRepoScore(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("get repo score: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]float64{"score": score})
	},
}

func init() {
	scoreFileCmd.Flags().StringVar(&scoreFilePath, "file", "", "file path to score (required)")

	scoreCmd.AddCommand(scoreFileCmd)
	scoreCmd.AddCommand(scoreRepoCmd)
}
