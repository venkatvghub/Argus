package argus

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OwnerListEntry is the paginated contributor entry for /repos/{id}/owners.
type OwnerListEntry struct {
	Key                string  `json:"key"`
	Name               string  `json:"name"`
	Email              *string `json:"email"`
	FilesOwned         int     `json:"files_owned"`
	HotspotsOwned      int     `json:"hotspots_owned"`
	SiloModules        int     `json:"silo_modules"`
	DeadCodeFilesOwned int     `json:"dead_code_files_owned"`
	DeadCodeLinesOwned int     `json:"dead_code_lines_owned"`
	CommitCount90d     int     `json:"commit_count_90d"`
	LastCommitAt       *string `json:"last_commit_at"`
	BusFactorRiskFiles int     `json:"bus_factor_risk_files"`
}

type rawOwnerStats struct {
	name         string
	fileCommits  map[string]int // path → commit count
	commitCount90d int
	lastCommit   time.Time
}

// buildOwnerStatsFromGit runs git log to collect per-author commit and file stats.
func buildOwnerStatsFromGit(repoPath string, cutoffDays int) (map[string]*rawOwnerStats, error) {
	cmd := exec.Command("git", "log",
		"--name-only",
		"--no-merges",
		"--format=%x1e%ae%x1f%an%x1f%ct",
		"HEAD",
	)
	cmd.Dir = repoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git log pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git log start: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -cutoffDays)
	byEmail := make(map[string]*rawOwnerStats)

	const recSep = "\x1e"
	const fldSep = "\x1f"

	var curEmail string
	var curRecent bool
	var curTime time.Time

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 512*1024), 512*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, recSep) {
			parts := strings.SplitN(line[len(recSep):], fldSep, 3)
			if len(parts) < 3 {
				curEmail = ""
				continue
			}
			curEmail = parts[0]
			name := parts[1]
			ts, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
			if err != nil {
				curEmail = ""
				continue
			}
			curTime = time.Unix(ts, 0)
			curRecent = curTime.After(cutoff)

			if curEmail == "" {
				continue
			}
			o, ok := byEmail[curEmail]
			if !ok {
				o = &rawOwnerStats{
					name:        name,
					fileCommits: make(map[string]int),
				}
				byEmail[curEmail] = o
			}
			if o.name == "" {
				o.name = name
			}
			if curTime.After(o.lastCommit) {
				o.lastCommit = curTime
			}
			if curRecent {
				o.commitCount90d++
			}
			continue
		}
		if line == "" || curEmail == "" {
			continue
		}
		byEmail[curEmail].fileCommits[line]++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("git log scan: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git log wait: %w", err)
	}
	return byEmail, nil
}

// GetOwners returns a paginated, filtered, sorted contributor list for repoID.
func (i *Instance) GetOwners(ctx context.Context, repoID, q, sortKey string, limit, offset int) ([]OwnerListEntry, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	repo, err := i.GetRepository(ctx, repoID)
	if err != nil {
		return nil, 0, err
	}

	files, _ := i.GetRepoFiles(ctx, repoID)

	type fileInfo struct {
		churn       int
		authorCount int
	}
	fileMap := make(map[string]fileInfo, len(files))
	for _, f := range files {
		fileMap[f.Path] = fileInfo{churn: f.Churn, authorCount: f.AuthorCount}
	}

	byEmail, err := buildOwnerStatsFromGit(repo.Path, i.cfg.OwnerDefaultCutoffDaysOrDefault())
	if err != nil {
		return nil, 0, fmt.Errorf("owners git: %w", err)
	}

	// Determine primary owner per file (email with most commits on that file).
	primaryOwner := make(map[string]string) // path → email
	for email, o := range byEmail {
		for path, count := range o.fileCommits {
			cur, ok := primaryOwner[path]
			if !ok || count > byEmail[cur].fileCommits[path] {
				primaryOwner[path] = email
			}
		}
	}

	// Aggregate per-email metrics from file data.
	type ownerMetrics struct {
		filesOwned         int
		hotspotsOwned      int
		busFactorRiskFiles int
	}
	metrics := make(map[string]*ownerMetrics)
	for path, email := range primaryOwner {
		if _, ok := byEmail[email]; !ok {
			continue
		}
		m := metrics[email]
		if m == nil {
			m = &ownerMetrics{}
			metrics[email] = m
		}
		m.filesOwned++
		fi := fileMap[path]
		if fi.churn >= i.cfg.OwnerHotspotChurnThresholdOrDefault() {
			m.hotspotsOwned++
		}
		if fi.authorCount == 1 {
			m.busFactorRiskFiles++
		}
	}

	// Build result entries with optional search filter.
	lower := strings.ToLower(q)
	entries := make([]OwnerListEntry, 0, len(byEmail))
	for email, o := range byEmail {
		if q != "" {
			if !strings.Contains(strings.ToLower(o.name), lower) &&
				!strings.Contains(strings.ToLower(email), lower) {
				continue
			}
		}
		m := metrics[email]
		if m == nil {
			m = &ownerMetrics{}
		}
		var lastCommitAt *string
		if !o.lastCommit.IsZero() {
			s := o.lastCommit.UTC().Format(time.RFC3339)
			lastCommitAt = &s
		}
		e := email
		entries = append(entries, OwnerListEntry{
			Key:                email,
			Name:               o.name,
			Email:              &e,
			FilesOwned:         m.filesOwned,
			HotspotsOwned:      m.hotspotsOwned,
			CommitCount90d:     o.commitCount90d,
			LastCommitAt:       lastCommitAt,
			BusFactorRiskFiles: m.busFactorRiskFiles,
		})
	}

	switch sortKey {
	case "hotspots_owned":
		sort.Slice(entries, func(a, b int) bool { return entries[a].HotspotsOwned > entries[b].HotspotsOwned })
	case "commit_count_90d":
		sort.Slice(entries, func(a, b int) bool { return entries[a].CommitCount90d > entries[b].CommitCount90d })
	case "bus_factor_risk_files":
		sort.Slice(entries, func(a, b int) bool { return entries[a].BusFactorRiskFiles > entries[b].BusFactorRiskFiles })
	default:
		sort.Slice(entries, func(a, b int) bool { return entries[a].FilesOwned > entries[b].FilesOwned })
	}

	total := len(entries)
	if offset >= total {
		return []OwnerListEntry{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return entries[offset:end], total, nil
}
