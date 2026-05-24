package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/models"
)

var markersFilePath string
var markersFormat string

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
		return printMarkers(results, markersFormat)
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
		return printMarkers(results, markersFormat)
	},
}

// printMarkers outputs markers in json or table format.
func printMarkers(markers []models.Marker, format string) error {
	switch format {
	case "table":
		fmt.Fprintf(os.Stdout, "%-26s %-20s %-8s %5s  %9s\n", "FILE", "TYPE", "SEV", "LINE", "DEDUCTION")
		for _, m := range markers {
			fmt.Fprintf(os.Stdout, "%-26s %-20s %-8s %5d  %9.2f\n",
				truncate(m.File, 26), truncate(m.Type, 20), m.Severity, m.Line, m.Deduction)
			if m.Suggestion != "" {
				fmt.Fprintf(os.Stdout, "  → %s\n", wordWrap(m.Suggestion, 100, "    "))
			}
		}
		return nil
	default: // "json"
		return json.NewEncoder(os.Stdout).Encode(markers)
	}
}

// truncate shortens string s to at most n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// wordWrap wraps text at maxWidth characters, indenting continuation lines.
func wordWrap(text string, maxWidth int, indent string) string {
	// First line budget: maxWidth minus the "  → " prefix (4 chars).
	firstLineBudget := maxWidth - 4
	if firstLineBudget <= 0 {
		firstLineBudget = maxWidth
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	var current strings.Builder
	budget := firstLineBudget

	for _, w := range words {
		if current.Len() == 0 {
			current.WriteString(w)
		} else if current.Len()+1+len(w) <= budget {
			current.WriteByte(' ')
			current.WriteString(w)
		} else {
			lines = append(lines, current.String())
			current.Reset()
			current.WriteString(w)
			budget = maxWidth - len(indent)
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return strings.Join(lines, "\n"+indent)
}

func init() {
	markersFileCmd.Flags().StringVar(&markersFilePath, "file", "", "file path to inspect (required)")
	markersFileCmd.Flags().StringVar(&markersFormat, "format", "json", "output format: json or table")
	markersRepoCmd.Flags().StringVar(&markersFormat, "format", "json", "output format: json or table")

	markersCmd.AddCommand(markersFileCmd)
	markersCmd.AddCommand(markersRepoCmd)
}
