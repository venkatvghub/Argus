package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/models"
)

var (
	markersFilePath     string
	markersFormat       string
	markersFilterType   string
	markersFilterSev    string
	markersFilterCat    string
	markersTopN         int
)

var markersCmd = &cobra.Command{
	Use:   "markers",
	Short: "Biomarker inspection commands",
}

var markersFileCmd = &cobra.Command{
	Use:   "file",
	Short: "Get biomarkers for a specific file",
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
		return printMarkers(applyMarkerFilters(results), markersFormat)
	},
}

var markersRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Get all biomarkers for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		results, err := instance.GetRepoMarkers(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("get repo markers: %w", err)
		}
		return printMarkers(applyMarkerFilters(results), markersFormat)
	},
}

var markersSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Summarize biomarker counts grouped by type, severity, and category",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		results, err := instance.GetRepoMarkers(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("get repo markers: %w", err)
		}
		results = applyMarkerFilters(results)
		return printMarkerSummary(results, markersFormat)
	},
}

var markersTopFilesCmd = &cobra.Command{
	Use:   "top-files",
	Short: "List files with the most biomarker deductions (worst first)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		results, err := instance.GetRepoMarkers(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("get repo markers: %w", err)
		}
		results = applyMarkerFilters(results)
		return printTopFiles(results, markersTopN, markersFormat)
	},
}

// applyMarkerFilters filters markers by --type, --severity, --category flags.
func applyMarkerFilters(markers []models.Marker) []models.Marker {
	if markersFilterType == "" && markersFilterSev == "" && markersFilterCat == "" {
		return markers
	}
	out := markers[:0:0]
	for _, m := range markers {
		if markersFilterType != "" && !strings.EqualFold(m.Type, markersFilterType) {
			continue
		}
		if markersFilterSev != "" && !strings.EqualFold(m.Severity, markersFilterSev) {
			continue
		}
		if markersFilterCat != "" && !strings.EqualFold(string(m.Category), markersFilterCat) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// printMarkers outputs markers in json or table format.
func printMarkers(markers []models.Marker, format string) error {
	switch format {
	case "table":
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "FILE\tTYPE\tSEV\tLINE\tDEDUCTION\tCATEGORY")
		for _, m := range markers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.2f\t%s\n",
				truncate(m.File, 40), truncate(m.Type, 22), m.Severity, m.Line, m.Deduction, m.Category)
			if m.Suggestion != "" {
				fmt.Fprintf(w, "  → %s\n", wordWrap(m.Suggestion, 100, "    "))
			}
		}
		return w.Flush()
	default: // "json"
		return json.NewEncoder(os.Stdout).Encode(markers)
	}
}

type markerSummary struct {
	Total      int                    `json:"total"`
	BySeverity map[string]int         `json:"by_severity"`
	ByType     map[string]int         `json:"by_type"`
	ByCategory map[string]int         `json:"by_category"`
	TotalDeduction float64            `json:"total_deduction"`
}

func printMarkerSummary(markers []models.Marker, format string) error {
	s := markerSummary{
		Total:      len(markers),
		BySeverity: map[string]int{},
		ByType:     map[string]int{},
		ByCategory: map[string]int{},
	}
	for _, m := range markers {
		s.BySeverity[m.Severity]++
		s.ByType[m.Type]++
		s.ByCategory[string(m.Category)]++
		s.TotalDeduction += m.Deduction
	}

	if format == "json" {
		return json.NewEncoder(os.Stdout).Encode(s)
	}

	// table
	fmt.Fprintf(os.Stdout, "Total markers: %d   Total deduction: %.2f\n\n", s.Total, s.TotalDeduction)

	printCountTable(os.Stdout, "SEVERITY", s.BySeverity)
	fmt.Fprintln(os.Stdout)
	printCountTable(os.Stdout, "TYPE", s.ByType)
	fmt.Fprintln(os.Stdout)
	printCountTable(os.Stdout, "CATEGORY", s.ByCategory)
	return nil
}

func printCountTable(out *os.File, header string, counts map[string]int) {
	type kv struct{ k string; v int }
	rows := make([]kv, 0, len(counts))
	for k, v := range counts {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].v > rows[j].v })
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%s\tCOUNT\n", header)
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%d\n", r.k, r.v)
	}
	w.Flush()
}

type fileRisk struct {
	File       string  `json:"file"`
	Count      int     `json:"marker_count"`
	Deduction  float64 `json:"total_deduction"`
}

func printTopFiles(markers []models.Marker, n int, format string) error {
	byFile := map[string]*fileRisk{}
	for _, m := range markers {
		r := byFile[m.File]
		if r == nil {
			r = &fileRisk{File: m.File}
			byFile[m.File] = r
		}
		r.Count++
		r.Deduction += m.Deduction
	}

	ranked := make([]*fileRisk, 0, len(byFile))
	for _, r := range byFile {
		ranked = append(ranked, r)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Deduction != ranked[j].Deduction {
			return ranked[i].Deduction > ranked[j].Deduction
		}
		return ranked[i].Count > ranked[j].Count
	})
	if n > 0 && n < len(ranked) {
		ranked = ranked[:n]
	}

	if format == "json" {
		return json.NewEncoder(os.Stdout).Encode(ranked)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FILE\tMARKERS\tDEDUCTION")
	for _, r := range ranked {
		fmt.Fprintf(w, "%s\t%d\t%.2f\n", r.File, r.Count, r.Deduction)
	}
	return w.Flush()
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
	// shared filter flags wired to all subcommands that fetch markers
	for _, sub := range []*cobra.Command{markersFileCmd, markersRepoCmd, markersSummaryCmd, markersTopFilesCmd} {
		sub.Flags().StringVar(&markersFilterType, "type", "", "filter by marker type (e.g. pii, sql_injection)")
		sub.Flags().StringVar(&markersFilterSev, "severity", "", "filter by severity: low, medium, high, critical")
		sub.Flags().StringVar(&markersFilterCat, "category", "", "filter by score category (e.g. compliance_appsec)")
		sub.Flags().StringVar(&markersFormat, "format", "json", "output format: json or table")
	}
	markersFileCmd.Flags().StringVar(&markersFilePath, "file", "", "file path to inspect (required)")
	markersTopFilesCmd.Flags().IntVar(&markersTopN, "top", 20, "number of files to show (0 = all)")

	markersCmd.AddCommand(markersFileCmd)
	markersCmd.AddCommand(markersRepoCmd)
	markersCmd.AddCommand(markersSummaryCmd)
	markersCmd.AddCommand(markersTopFilesCmd)
}
