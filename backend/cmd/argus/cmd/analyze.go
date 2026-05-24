package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/models"
)

var analyzeWait bool

var analyzeCmd = &cobra.Command{
	Use:   "analyze <repo-path>",
	Short: "Analyze a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID, err := instance.Analyze(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("analyze: %w", err)
		}

		if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
			"job_id":  jobID,
			"message": "analysis started",
		}); err != nil {
			return err
		}

		if !analyzeWait {
			return nil
		}

		// Poll until terminal state.
		for {
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			default:
			}

			job, ok := instance.Jobs.GetJob(jobID)
			if !ok {
				return fmt.Errorf("job %s not found", jobID)
			}

			fmt.Fprintf(os.Stderr, "status: %s  progress: %s\n", job.Status, job.Progress)

			if job.Status == models.JobStatusCompleted {
				fmt.Fprintf(os.Stderr, "analysis complete\n")
				return nil
			}
			if job.Status == models.JobStatusFailed {
				return fmt.Errorf("analysis failed: %s", job.Error)
			}

			time.Sleep(500 * time.Millisecond)
		}
	},
}

func init() {
	analyzeCmd.Flags().BoolVar(&analyzeWait, "wait", false, "wait for analysis to complete")
}
