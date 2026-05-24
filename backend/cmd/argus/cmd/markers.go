package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var markersFilePath string

var markersCmd = &cobra.Command{
	Use:   "markers",
	Short: "Marker inspection commands",
}

var markersFileCmd = &cobra.Command{
	Use:   "file",
	Short: "Get markers for a specific file",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		if markersFilePath == "" {
			return fmt.Errorf("--file is required")
		}
		results, err := instance.GetFileMarkers(cmd.Context(), rootRepoID, markersFilePath)
		if err != nil {
			return fmt.Errorf("get file markers: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(results)
	},
}

var markersRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Get all markers for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		results, err := instance.GetRepoMarkers(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("get repo markers: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(results)
	},
}

func init() {
	markersFileCmd.Flags().StringVar(&markersFilePath, "file", "", "file path to inspect (required)")

	markersCmd.AddCommand(markersFileCmd)
	markersCmd.AddCommand(markersRepoCmd)
}
