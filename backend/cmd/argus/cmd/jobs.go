package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var jobsRepoID string

// jobsCmd is the root command for wiki generation job management.
var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Wiki generation job management",
}

// jobsListCmd lists wiki generation jobs for a repository.
var jobsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List wiki generation jobs for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if jobsRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		jobs, err := instance.ListWikiJobs(cmd.Context(), jobsRepoID)
		if err != nil {
			return fmt.Errorf("list wiki jobs: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(jobs)
	},
}

// jobsGetCmd retrieves details of a specific wiki generation job.
var jobsGetCmd = &cobra.Command{
	Use:   "get <job-id>",
	Short: "Get details of a wiki generation job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		job, err := instance.GetWikiJob(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("get wiki job: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(job)
	},
}

func init() {
	jobsListCmd.Flags().StringVar(&jobsRepoID, "repo-id", "", "repository ID (required)")
	jobsCmd.AddCommand(jobsListCmd)
	jobsCmd.AddCommand(jobsGetCmd)
}
