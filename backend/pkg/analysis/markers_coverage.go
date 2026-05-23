package analysis

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/models"
)

// loadCoverage attempts to load coverage data from a configured or auto-discovered file.
// Returns nil, nil if no coverage file is found.
func loadCoverage(repoPath string, cfg *config.Config) (map[string]float64, error) {
	candidates := []string{}

	if cfg != nil && cfg.CoverageFile != "" {
		candidates = append(candidates, cfg.CoverageFile)
	}

	candidates = append(candidates,
		filepath.Join(repoPath, "lcov.info"),
		filepath.Join(repoPath, "coverage.xml"),
		filepath.Join(repoPath, "clover.xml"),
	)

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		switch filepath.Base(path) {
		case "lcov.info":
			return parseLCOV(data), nil
		case "coverage.xml":
			return parseCobertura(data), nil
		case "clover.xml":
			return parseClover(data), nil
		default:
			// For ARGUS_COVERAGE_FILE, detect format by content
			if bytes.Contains(data, []byte("SF:")) {
				return parseLCOV(data), nil
			}
			if bytes.Contains(data, []byte("line-rate")) {
				return parseCobertura(data), nil
			}
			if bytes.Contains(data, []byte("<clover")) {
				return parseClover(data), nil
			}
		}
	}

	return nil, nil
}

// parseLCOV parses lcov.info format and returns filepath → line coverage %.
func parseLCOV(data []byte) map[string]float64 {
	result := make(map[string]float64)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	var currentFile string
	var linesHit, linesTotal int

	flush := func() {
		if currentFile != "" && linesTotal > 0 {
			result[currentFile] = float64(linesHit) / float64(linesTotal) * 100.0
		} else if currentFile != "" {
			result[currentFile] = 0.0
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "SF:"):
			flush()
			currentFile = strings.TrimPrefix(line, "SF:")
			linesHit = 0
			linesTotal = 0
		case strings.HasPrefix(line, "DA:"):
			// DA:<line_number>,<execution_count>
			parts := strings.SplitN(strings.TrimPrefix(line, "DA:"), ",", 2)
			if len(parts) == 2 {
				linesTotal++
				count, err := strconv.Atoi(parts[1])
				if err == nil && count > 0 {
					linesHit++
				}
			}
		case line == "end_of_record":
			flush()
			currentFile = ""
			linesHit = 0
			linesTotal = 0
		}
	}
	flush()
	return result
}

// coberturaXML is the XML structure for Cobertura coverage.xml.
type coberturaXML struct {
	Packages []coberturaPackage `xml:"packages>package"`
}

type coberturaPackage struct {
	Classes []coberturaClass `xml:"classes>class"`
}

type coberturaClass struct {
	Filename string  `xml:"filename,attr"`
	LineRate float64 `xml:"line-rate,attr"`
}

// parseCobertura parses Cobertura coverage.xml and returns filepath → line coverage %.
func parseCobertura(data []byte) map[string]float64 {
	result := make(map[string]float64)
	var cov coberturaXML
	if err := xml.Unmarshal(data, &cov); err != nil {
		return result
	}
	for _, pkg := range cov.Packages {
		for _, cls := range pkg.Classes {
			if cls.Filename != "" {
				result[cls.Filename] = cls.LineRate * 100.0
			}
		}
	}
	return result
}

// cloverXML is the XML structure for Clover clover.xml.
type cloverXML struct {
	Files []cloverFile `xml:"project>file"`
}

type cloverFile struct {
	Name    string       `xml:"name,attr"`
	Metrics cloverMetrics `xml:"metrics"`
}

type cloverMetrics struct {
	Covered  int `xml:"coveredstatements,attr"`
	Elements int `xml:"statements,attr"`
}

// parseClover parses Clover clover.xml and returns filepath → line coverage %.
func parseClover(data []byte) map[string]float64 {
	result := make(map[string]float64)
	var cov cloverXML
	if err := xml.Unmarshal(data, &cov); err != nil {
		return result
	}
	for _, f := range cov.Files {
		if f.Name == "" {
			continue
		}
		if f.Metrics.Elements > 0 {
			result[f.Name] = float64(f.Metrics.Covered) / float64(f.Metrics.Elements) * 100.0
		} else {
			result[f.Name] = 0.0
		}
	}
	return result
}

// checkCoverageMarkers checks untested_hotspot and coverage_gap for each file.
func (me *MarkerEngine) checkCoverageMarkers(files []models.FileNode, coverage map[string]float64, graph *GraphEngine, prThreshold float64) []models.Marker {
	if coverage == nil {
		return nil
	}

	var markers []models.Marker

	for _, file := range files {
		if !file.IsFile {
			continue
		}

		cov, ok := lookupCoverage(coverage, file.Path)
		if !ok {
			continue
		}

		isTestFile := strings.Contains(file.Path, "_test") || strings.Contains(file.Path, "test_")

		// untested_hotspot: high churn + high PageRank + very low coverage
		if file.Churn >= coverageUntestedChurnThreshold && cov < coverageUntestedThreshold {
			inTopPageRank := false
			if graph != nil && prThreshold > 0 {
				if node, ok2 := graph.GetNodeByPath(file.Path); ok2 {
					inTopPageRank = node.PageRank >= prThreshold
				}
			}
			if inTopPageRank {
				markers = append(markers, models.Marker{
					Type:      "untested_hotspot",
					Severity:  "high",
					Message:   "High-churn, high-PageRank file has < 20% line coverage",
					File:      file.Path,
					Deduction: coverageDeductionUntested,
					Category:  models.ScoreCatTestCoverage,
				})
				continue
			}
		}

		// coverage_gap: < 60% coverage, skip test files
		if !isTestFile && cov < coverageGapThreshold {
			deduction := (coverageGapThreshold - cov) / coverageGapThreshold * coverageDeductionGapMax
			markers = append(markers, models.Marker{
				Type:      "coverage_gap",
				Severity:  "medium",
				Message:   "File has < 60% line coverage",
				File:      file.Path,
				Deduction: deduction,
				Category:  models.ScoreCatTestCoverage,
			})
		}
	}

	return markers
}

// lookupCoverage finds coverage for a file path, trying exact match then suffix match.
func lookupCoverage(coverage map[string]float64, filePath string) (float64, bool) {
	if v, ok := coverage[filePath]; ok {
		return v, true
	}
	// Try matching by suffix (coverage maps may store absolute or repo-relative paths)
	for k, v := range coverage {
		if strings.HasSuffix(k, filePath) || strings.HasSuffix(filePath, k) {
			return v, true
		}
	}
	return 0, false
}
