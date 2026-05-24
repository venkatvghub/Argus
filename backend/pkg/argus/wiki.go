package argus

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/venkatvghub/argus/pkg/analysis"
	"github.com/venkatvghub/argus/pkg/models"
	"github.com/venkatvghub/argus/pkg/providers"
)

// pageLevel maps page type to its generation level (0 = leaf, higher = depends on lower).
var pageLevel = map[string]int{
	"symbol_spotlight":     0,
	"file_page":            1,
	"api_contract":         2,
	"infra_page":           2,
	"scc_page":             3,
	"module_page":          3,
	"architecture_diagram": 4,
	"repo_overview":        5,
	"onboarding":           6,
}

// wikiPageJob is a unit of work in the wiki generation pipeline.
type wikiPageJob struct {
	id      string // "{type}:{subject}"
	pgType  string
	subject string
	level   int
	tier    string
}

// GenerateWiki runs the wiki generation pipeline for a repository.
// It assembles context per page type, calls the tiered LLM, and checkpoints progress.
// If completedPageIDs is non-nil, pages in the set are skipped (resume mode).
// coveragePct selects the top fraction of pages by graph importance; use 1.0 for all pages.
// OnProgress is called after each page completes with (done, total) counts; may be nil.
func (i *Instance) GenerateWiki(
	ctx context.Context,
	repoID string,
	jobID string,
	plan providers.GenerationPlan,
	router *providers.TieredRouter,
	completedPageIDs map[string]struct{},
	concurrency int,
	coveragePct float64,
	onProgress func(done, total int),
) error {
	if concurrency <= 0 {
		concurrency = 5
	}

	// Load repo path from DB
	repos, err := i.db.ListRepositories(ctx)
	if err != nil {
		return fmt.Errorf("load repo: %w", err)
	}
	var repoPath string
	for _, r := range repos {
		if r.ID == repoID {
			repoPath = r.Path
			break
		}
	}
	if repoPath == "" {
		return ErrRepoNotFound
	}

	// Gather data for context assembly
	i.mu.RLock()
	engine := i.engines[repoID]
	markers := i.markers[repoID]
	i.mu.RUnlock()

	if engine == nil {
		return fmt.Errorf("no graph engine for repo %s: run analysis first", repoID)
	}

	// Build jobs list from plan entries
	var jobs []wikiPageJob
	for _, entry := range plan.Entries {
		pageJobs := buildPageJobs(entry, engine)
		for _, j := range pageJobs {
			if completedPageIDs != nil {
				if _, done := completedPageIDs[j.id]; done {
					continue
				}
			}
			j.tier = entry.Tier
			jobs = append(jobs, j)
		}
	}

	// Group jobs by level
	maxLevel := 0
	for _, j := range jobs {
		if j.level > maxLevel {
			maxLevel = j.level
		}
	}

	sem := make(chan struct{}, concurrency)
	total := len(jobs)
	var done atomic.Int64

	if err := i.db.UpdateWikiJobStatus(ctx, jobID, models.WikiJobRunning); err != nil {
		i.log.Warn("failed to update wiki job status", "error", err)
	}

	for level := 0; level <= maxLevel; level++ {
		var levelJobs []wikiPageJob
		for _, j := range jobs {
			if j.level == level {
				levelJobs = append(levelJobs, j)
			}
		}
		if len(levelJobs) == 0 {
			continue
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		var firstErr error

		for _, job := range levelJobs {
			job := job
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}

				prompt := buildPrompt(job, repoPath, engine, markers)
				content, err := router.ChatTier(ctx, job.tier, prompt)
				if err != nil {
					i.log.Warn("wiki page generation failed", "page_id", job.id, "error", err)
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					if errors.Is(err, providers.ErrCircuitOpen) {
						// Circuit breaker open: pause immediately so --resume can retry later.
						// Use Background context — ctx may already be cancelled.
						_ = i.db.UpdateWikiJobStatus(context.Background(), jobID, models.WikiJobPaused)
					}
					done.Add(1) // count attempt regardless of outcome for accurate progress
					if onProgress != nil {
						onProgress(int(done.Load()), total)
					}
					return
				}

				page := models.WikiPage{
					ID:      job.id,
					RepoID:  repoID,
					JobID:   jobID,
					Type:    job.pgType,
					Subject: job.subject,
					Content: content,
					Level:   job.level,
				}
				if err := i.db.UpsertWikiPage(ctx, page); err != nil {
					i.log.Warn("failed to persist wiki page", "page_id", job.id, "error", err)
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					done.Add(1)
					if onProgress != nil {
						onProgress(int(done.Load()), total)
					}
					return
				}
				if err := i.db.MarkWikiPageComplete(ctx, jobID, job.id); err != nil {
					i.log.Warn("failed to checkpoint wiki page", "page_id", job.id, "error", err)
				}
				n := int(done.Add(1))
				if onProgress != nil {
					onProgress(n, total)
				}
			}()
		}
		wg.Wait()

		if firstErr != nil {
			if !errors.Is(firstErr, providers.ErrCircuitOpen) {
				// Use Background context — ctx may already be cancelled.
				_ = i.db.UpdateWikiJobStatus(context.Background(), jobID, models.WikiJobPaused)
			}
			return fmt.Errorf("wiki generation stopped at level %d: %w", level, firstErr)
		}
	}

	return i.db.UpdateWikiJobStatus(ctx, jobID, models.WikiJobCompleted)
}

// scoreAndRankNodes returns file nodes sorted descending by importance score.
// Score = PageRank*0.7 + normalizedChurn*0.3.
// Churn is normalized to [0,1] by dividing by the max churn across all file nodes.
// If engine is nil, returns nil.
func scoreAndRankNodes(nodes []*analysis.Node) []*analysis.Node {
	if nodes == nil {
		return nil
	}

	// Collect file nodes and compute max churn for normalization.
	var fileNodes []*analysis.Node
	maxChurn := 0
	for _, n := range nodes {
		if n.InternalType() != analysis.NodeTypeFile {
			continue
		}
		f := n.File()
		if f == nil || !f.IsFile {
			continue
		}
		fileNodes = append(fileNodes, n)
		if f.Churn > maxChurn {
			maxChurn = f.Churn
		}
	}

	sort.Slice(fileNodes, func(i, j int) bool {
		si := nodeImportanceScore(fileNodes[i], maxChurn)
		sj := nodeImportanceScore(fileNodes[j], maxChurn)
		if si != sj {
			return si > sj
		}
		return fileNodes[i].File().Path < fileNodes[j].File().Path
	})

	return fileNodes
}

// nodeImportanceScore computes PageRank*0.7 + normalizedChurn*0.3 for a file node.
func nodeImportanceScore(n *analysis.Node, maxChurn int) float64 {
	f := n.File()
	if f == nil {
		return n.PageRank * 0.7
	}
	churnNorm := 0.0
	if maxChurn > 0 {
		churn := f.Churn
		if churn < 0 {
			churn = 0
		}
		churnNorm = float64(churn) / float64(maxChurn)
	}
	return n.PageRank*0.7 + churnNorm*0.3
}

// buildPageJobs expands a PlanEntry into individual page jobs.
// For file_page, api_contract, and infra_page: nodes are ranked by graph importance
// and capped at entry.Count. For symbol_spotlight: symbols are ranked by parent file PageRank
// and capped at entry.Count. Other types are not ranked (their counts are already small).
func buildPageJobs(entry providers.PlanEntry, engine *analysis.GraphEngine) []wikiPageJob {
	var jobs []wikiPageJob
	level := pageLevel[entry.PageType]

	switch entry.PageType {
	case "file_page", "api_contract", "infra_page":
		if engine == nil {
			return nil
		}
		// Get all nodes, filter for this page type, then rank.
		allNodes := engine.GetNodes()
		ranked := scoreAndRankNodes(allNodes)

		count := 0
		for _, n := range ranked {
			if entry.Count > 0 && count >= entry.Count {
				break
			}
			f := n.File()
			if entry.PageType == "infra_page" && !isInfraFile(f.Path) {
				continue
			}
			if entry.PageType == "api_contract" && !isAPIContractFile(f.Path) {
				continue
			}
			jobs = append(jobs, wikiPageJob{
				id:      entry.PageType + ":" + f.Path,
				pgType:  entry.PageType,
				subject: f.Path,
				level:   level,
			})
			count++
		}

	case "symbol_spotlight":
		if engine == nil {
			return nil
		}
		// Build a map of file path -> PageRank for scoring symbols by parent file.
		filePageRank := make(map[string]float64)
		for _, n := range engine.GetNodes() {
			if n.InternalType() == analysis.NodeTypeFile {
				f := n.File()
				if f != nil {
					filePageRank[f.Path] = n.PageRank
				}
			}
		}

		// Collect all symbol nodes.
		type symWithScore struct {
			node  *analysis.Node
			score float64
		}
		var syms []symWithScore
		for _, n := range engine.GetNodes() {
			if n.InternalType() != analysis.NodeTypeSymbol {
				continue
			}
			s := n.Symbol()
			if s == nil {
				continue
			}
			pr := filePageRank[s.FilePath]
			syms = append(syms, symWithScore{node: n, score: pr})
		}
		sort.Slice(syms, func(i, j int) bool {
			if syms[i].score != syms[j].score {
				return syms[i].score > syms[j].score
			}
			si, sj := syms[i].node.Symbol(), syms[j].node.Symbol()
			if si.FilePath != sj.FilePath {
				return si.FilePath < sj.FilePath
			}
			return si.Name < sj.Name
		})

		count := 0
		for _, sw := range syms {
			if entry.Count > 0 && count >= entry.Count {
				break
			}
			s := sw.node.Symbol()
			jobs = append(jobs, wikiPageJob{
				id:      "symbol_spotlight:" + s.FilePath + ":" + s.Name,
				pgType:  entry.PageType,
				subject: s.FilePath + ":" + s.Name,
				level:   level,
			})
			count++
		}

	case "module_page", "scc_page":
		if engine == nil {
			return nil
		}
		// Collect unique keys first, then sort for deterministic page IDs across runs.
		seen := make(map[string]bool)
		var keys []string
		for _, n := range engine.GetNodes() {
			if n.InternalType() != analysis.NodeTypeFile {
				continue
			}
			f := n.File()
			if f == nil {
				continue
			}
			var key string
			if entry.PageType == "module_page" {
				key = filepath.Dir(f.Path)
			} else {
				key = fmt.Sprintf("community_%d", n.CommunityID)
			}
			if key == "." || seen[key] {
				continue
			}
			seen[key] = true
			keys = append(keys, key)
		}
		sort.Strings(keys)
		count := 0
		for _, key := range keys {
			if entry.Count > 0 && count >= entry.Count {
				break
			}
			jobs = append(jobs, wikiPageJob{
				id:      entry.PageType + ":" + key,
				pgType:  entry.PageType,
				subject: key,
				level:   level,
			})
			count++
		}

	case "repo_overview", "architecture_diagram", "onboarding":
		jobs = append(jobs, wikiPageJob{
			id:      entry.PageType + ":root",
			pgType:  entry.PageType,
			subject: "root",
			level:   level,
		})
	}

	return jobs
}

// isInfraFile returns true for infrastructure-related file paths.
func isInfraFile(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(path)
	return base == "Dockerfile" || ext == ".tf" || ext == ".hcl" ||
		strings.Contains(path, ".github/workflows") ||
		base == "Jenkinsfile" || base == "docker-compose.yml" || base == "docker-compose.yaml"
}

// isAPIContractFile returns true for files that are likely to contain exported API symbols.
func isAPIContractFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".go" || ext == ".ts" || ext == ".java" || ext == ".py"
}

// buildPrompt assembles a concise LLM prompt for a wiki page job.
func buildPrompt(job wikiPageJob, repoPath string, engine *analysis.GraphEngine, markers []models.Marker) string {
	var sb strings.Builder

	switch job.pgType {
	case "file_page":
		content := readFileSnippet(filepath.Join(repoPath, job.subject), 200)
		fm := markersForFile(markers, job.subject)
		sb.WriteString(fmt.Sprintf("Write concise markdown documentation for this source file.\n\nFile: %s\n\n", job.subject))
		if content != "" {
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", content))
		}
		if len(fm) > 0 {
			sb.WriteString("Health markers:\n")
			for _, m := range fm {
				sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", m.Type, m.Severity, m.Message))
			}
		}
		sb.WriteString("\nDocument: purpose, key functions/types, usage notes, and any flagged issues.")

	case "symbol_spotlight":
		parts := strings.SplitN(job.subject, ":", 2)
		name := job.subject
		if len(parts) == 2 {
			name = parts[1]
		}
		sb.WriteString(fmt.Sprintf("Write a short markdown explanation of the symbol `%s`.\n", name))
		sb.WriteString("Include: what it does, parameters/return values if applicable, and usage examples.")

	case "module_page":
		sb.WriteString(fmt.Sprintf("Write markdown documentation for the module/package at path: %s\n\n", job.subject))
		sb.WriteString("Summarize: the module's purpose, key files, main exported symbols, and dependencies.")

	case "scc_page":
		sb.WriteString(fmt.Sprintf("Write markdown documentation for code community: %s\n\n", job.subject))
		sb.WriteString("Describe the group of related files, their shared responsibility, and key interactions.")

	case "api_contract":
		content := readFileSnippet(filepath.Join(repoPath, job.subject), 100)
		sb.WriteString(fmt.Sprintf("Document the API contract for: %s\n\n", job.subject))
		if content != "" {
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", content))
		}
		sb.WriteString("List: exported functions/types, their signatures, and expected behavior.")

	case "infra_page":
		content := readFileSnippet(filepath.Join(repoPath, job.subject), 150)
		sb.WriteString(fmt.Sprintf("Document this infrastructure file: %s\n\n", job.subject))
		if content != "" {
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", content))
		}
		sb.WriteString("Explain: the resource being configured, key settings, and operational notes.")

	case "architecture_diagram":
		sb.WriteString("Write a markdown architecture overview for this codebase.\n\n")
		if engine != nil {
			communities := make(map[int]int)
			for _, n := range engine.GetNodes() {
				communities[n.CommunityID]++
			}
			sb.WriteString(fmt.Sprintf("The codebase has %d detected communities. ", len(communities)))
		}
		sb.WriteString("Describe the high-level architecture, major subsystems, data flow, and key dependencies.")

	case "repo_overview":
		sb.WriteString("Write a comprehensive markdown overview for this repository.\n\n")
		if len(markers) > 0 {
			sb.WriteString(fmt.Sprintf("The analysis found %d health markers. ", len(markers)))
		}
		sb.WriteString("Cover: purpose, tech stack, directory structure, setup instructions, and contribution notes.")

	case "onboarding":
		sb.WriteString("Write a developer onboarding guide for this repository.\n\n")
		sb.WriteString("Include: getting started, development environment setup, key concepts to understand, important files, and common workflows.")
	}

	return sb.String()
}

// readFileSnippet reads at most maxLines lines from a file for use in prompts.
func readFileSnippet(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024)
	var lines []string
	extra := 0
	for scanner.Scan() {
		if len(lines) < maxLines {
			lines = append(lines, scanner.Text())
			continue
		}
		extra++
	}
	if err := scanner.Err(); err != nil {
		return ""
	}
	if extra > 0 {
		lines = append(lines, fmt.Sprintf("... (%d more lines)", extra))
	}
	return strings.Join(lines, "\n")
}

// markersForFile returns all markers for a specific file path.
func markersForFile(markers []models.Marker, filePath string) []models.Marker {
	var result []models.Marker
	for _, m := range markers {
		if m.File == filePath {
			result = append(result, m)
		}
	}
	return result
}
