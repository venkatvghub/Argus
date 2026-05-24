package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/models"
)

var analyzeWait bool

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

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

		frame := 0
		lastPhase := ""
		start := time.Now()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-cmd.Context().Done():
				fmt.Fprintln(os.Stderr)
				return cmd.Context().Err()
			case <-ticker.C:
			}

			job, ok := instance.Jobs.GetJob(jobID)
			if !ok {
				fmt.Fprintln(os.Stderr)
				return fmt.Errorf("job %s not found", jobID)
			}

			current := job.Progress
			spinner := spinnerFrames[frame%len(spinnerFrames)]
			frame++

			if phase(current) != phase(lastPhase) && lastPhase != "" {
				fmt.Fprintf(os.Stderr, "\r\033[K  \033[32m✓\033[0m  %s\n", phaseLabel(lastPhase))
			}
			lastPhase = current

			switch job.Status {
			case models.JobStatusCompleted:
				elapsed := time.Since(start).Round(time.Millisecond)
				fmt.Fprintf(os.Stderr, "\r\033[K  \033[32m✓\033[0m  %s\n", phaseLabel(current))
				fmt.Fprintf(os.Stderr, "  \033[2mdone in %s\033[0m\n", elapsed)
				return nil
			case models.JobStatusFailed:
				fmt.Fprintf(os.Stderr, "\r\033[K  \033[31m✗\033[0m  Failed: %s\n", job.Error)
				return fmt.Errorf("analysis failed: %s", job.Error)
			default:
				line := renderProgressLine(spinner, current)
				fmt.Fprintf(os.Stderr, "\r\033[K  \033[33m%s\033[0m  %s", spinner, line)
			}
		}
	},
}

// renderProgressLine builds the display string for one tick.
// If current contains a "done/total" counter (e.g. "Parsing files... (12/478)"),
// it renders a compact progress bar + percentage.
// Otherwise it returns the label as-is.
func renderProgressLine(spinner, current string) string {
	done, total, label, ok := parseCounter(current)
	if !ok || total == 0 {
		return current
	}

	pct := done * 100 / total
	bar := progressBar(done, total, 20)
	return fmt.Sprintf("%-30s %s  %d/%d  (%d%%)", label, bar, done, total, pct)
}

// progressBar renders a Unicode block bar of barWidth chars.
// e.g. [████████░░░░░░░░░░░░] for 40%
func progressBar(done, total, barWidth int) string {
	if total <= 0 {
		return ""
	}
	filled := done * barWidth / total
	if filled > barWidth {
		filled = barWidth
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
}

// parseCounter extracts done and total from strings like "Parsing files... (12/478)".
func parseCounter(s string) (done, total int, label string, ok bool) {
	open := strings.LastIndex(s, "(")
	close := strings.LastIndex(s, ")")
	if open < 0 || close <= open {
		return
	}
	inner := s[open+1 : close]
	slash := strings.Index(inner, "/")
	if slash < 0 {
		return
	}
	d, err1 := strconv.Atoi(strings.TrimSpace(inner[:slash]))
	t, err2 := strconv.Atoi(strings.TrimSpace(inner[slash+1:]))
	if err1 != nil || err2 != nil {
		return
	}
	return d, t, strings.TrimSpace(s[:open]), true
}

// phase strips the counter suffix so phase transitions fire once per logical phase.
func phase(s string) string {
	for i, c := range s {
		if c == '(' {
			return s[:i]
		}
	}
	return s
}

// phaseLabel returns a clean label for the completed-phase stamp.
func phaseLabel(s string) string {
	return strings.TrimRight(phase(s), " .")
}

func init() {
	analyzeCmd.Flags().BoolVar(&analyzeWait, "wait", false, "wait for analysis to complete")
}
