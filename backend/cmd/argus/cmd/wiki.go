package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var wikiCmd = &cobra.Command{
	Use:   "wiki",
	Short: "Wiki page commands",
}

var wikiListCmd = &cobra.Command{
	Use:   "list",
	Short: "List generated wiki pages for a repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		if rootRepoID == "" {
			return fmt.Errorf("--repo-id is required")
		}
		pages, err := instance.ListWikiPages(cmd.Context(), rootRepoID)
		if err != nil {
			return fmt.Errorf("list wiki pages: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(pages)
	},
}

var wikiGetCmd = &cobra.Command{
	Use:   "get <page-id>",
	Short: "Get a wiki page by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		page, err := instance.GetWikiPage(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("get wiki page: %w", err)
		}
		return json.NewEncoder(os.Stdout).Encode(page)
	},
}

var wikiExportCmd = &cobra.Command{
	Use:   "export <repo-id> <output-dir>",
	Short: "Export wiki pages for a repository to markdown files",
	Long: `Export all wiki pages for a repository as markdown files.

Files are written to <output-dir>/<page-type>/<subject>.md.
Use 'argus repos list' to find the repo ID.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoID := args[0]
		outDir := args[1]

		pages, err := instance.ListWikiPages(cmd.Context(), repoID)
		if err != nil {
			return fmt.Errorf("list wiki pages: %w", err)
		}
		if len(pages) == 0 {
			fmt.Fprintf(os.Stderr, "No wiki pages found for repo %s.\n", repoID)
			return nil
		}

		written := 0
		skipped := 0
		byType := map[string]int{}

		for _, p := range pages {
			pageType := sanitizePathComponent(p.Type)
			if pageType == "" {
				pageType = "misc"
			}
			dir := filepath.Join(outDir, pageType)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", dir, err)
			}

			subject := sanitizePathComponent(p.Subject)
			if subject == "" {
				subject = p.ID
			}
			path := filepath.Join(dir, subject+".md")

			if err := os.WriteFile(path, []byte(p.Content), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "skip %s: %v\n", path, err)
				skipped++
				continue
			}
			written++
			byType[pageType]++
		}

		w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
		fmt.Fprintf(os.Stderr, "Exported %d pages to %s\n\n", written, outDir)
		fmt.Fprintln(w, "TYPE\tCOUNT")
		for t, n := range byType {
			fmt.Fprintf(w, "%s\t%d\n", t, n)
		}
		w.Flush()
		if skipped > 0 {
			fmt.Fprintf(os.Stderr, "\n%d pages skipped (write errors)\n", skipped)
		}
		return nil
	},
}

var nonAlphanumRE = regexp.MustCompile(`[^a-zA-Z0-9_\-]+`)

// sanitizePathComponent converts a string into a safe filename component.
func sanitizePathComponent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, string(os.PathSeparator), "_")
	s = nonAlphanumRE.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}

func init() {
	wikiCmd.AddCommand(wikiListCmd)
	wikiCmd.AddCommand(wikiGetCmd)
	wikiCmd.AddCommand(wikiExportCmd)
}
