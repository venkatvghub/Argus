package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/venkatvghub/argus/pkg/models"
)

// funcInfo captures per-function structural metrics extracted from source text.
type funcInfo struct {
	name       string
	startLine  int
	endLine    int
	nloc       int
	cyclomatic int
	maxNesting int
	params     []string
}

// Primitive type sets per language extension.
var (
	goPrimTypes = map[string]bool{
		"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"float32": true, "float64": true, "bool": true, "string": true,
		"byte": true, "rune": true, "complex64": true, "complex128": true, "uintptr": true,
	}
	jsPrimTypes = map[string]bool{
		"string": true, "number": true, "boolean": true, "any": true,
		"undefined": true, "null": true, "never": true, "void": true,
		"bigint": true, "symbol": true,
	}
	javaPrimTypes = map[string]bool{
		"int": true, "long": true, "short": true, "byte": true, "double": true,
		"float": true, "boolean": true, "char": true, "String": true, "Integer": true,
		"Long": true, "Short": true, "Double": true, "Float": true, "Boolean": true,
	}
	pyPrimTypes = map[string]bool{
		"int": true, "float": true, "str": true, "bool": true,
		"bytes": true, "complex": true,
	}
)

func primTypesForExt(ext string) map[string]bool {
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx":
		return jsPrimTypes
	case ".java", ".kt":
		return javaPrimTypes
	case ".py":
		return pyPrimTypes
	default:
		return goPrimTypes
	}
}

func largeMethodThreshold(ext string) int {
	switch ext {
	case ".java", ".py", ".kt":
		return largeMethodNLOCJavaPython
	default:
		return largeMethodNLOCDefault
	}
}

func supportedStructuralExt(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".java", ".kt", ".py":
		return true
	}
	return false
}

// goFuncRe matches Go function declarations (methods and plain functions).
var goFuncRe = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s+)?(\w+)\s*\(([^)]*)\)`)

// extractGoFunctions parses Go source using brace counting and returns per-function metrics.
func extractGoFunctions(content string) []funcInfo {
	lines := strings.Split(content, "\n")
	var funcs []funcInfo
	inFunc := false
	depth := 0
	var current funcInfo
	var bodyLines []string

	for i, line := range lines {
		lineno := i + 1
		if !inFunc {
			m := goFuncRe.FindStringSubmatch(line)
			if m != nil {
				current = funcInfo{
					name:       m[1],
					startLine:  lineno,
					params:     parseGoParams(m[2]),
					cyclomatic: 1,
				}
				bodyLines = nil
				inFunc = true
				depth = 0
			}
		}
		if inFunc {
			inStr := false
			for _, ch := range line {
				if ch == '"' || ch == '\'' {
					inStr = !inStr
				}
				if inStr {
					continue
				}
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
				}
			}
			bodyLines = append(bodyLines, line)
			if depth <= 0 && len(bodyLines) > 1 {
				current.endLine = lineno
				current.nloc = countNLOC(bodyLines)
				current.cyclomatic += countCyclomatic(bodyLines)
				current.maxNesting = maxNestingDepth(bodyLines)
				funcs = append(funcs, current)
				inFunc = false
				bodyLines = nil
			}
		}
	}
	return funcs
}

// funcStartRe returns a function-start regex for non-Go languages.
func funcStartRe(ext string) *regexp.Regexp {
	switch ext {
	case ".java", ".kt":
		return regexp.MustCompile(`(?:public|private|protected|static|\s)+[\w<>\[\]]+\s+(\w+)\s*\(([^)]*)\)\s*(?:throws\s+[\w,\s]+)?\s*\{`)
	case ".ts", ".tsx", ".js", ".jsx":
		return regexp.MustCompile(`(?:function\s+(\w+)|(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?(?:function|\())\s*\(([^)]*)`)
	case ".py":
		return regexp.MustCompile(`^def\s+(\w+)\s*\(([^)]*)\)\s*:`)
	}
	return nil
}

// extractGenericFunctions extracts function info for non-Go languages.
func extractGenericFunctions(content, ext string) []funcInfo {
	re := funcStartRe(ext)
	if re == nil {
		return nil
	}
	lines := strings.Split(content, "\n")
	var funcs []funcInfo
	inFunc := false
	var current funcInfo
	var bodyLines []string
	depth := 0
	baseIndent := 0

	for i, line := range lines {
		lineno := i + 1
		if !inFunc {
			m := re.FindStringSubmatch(line)
			if m != nil {
				name := extractMatchedName(m, ext)
				if name == "" {
					continue
				}
				current = funcInfo{name: name, startLine: lineno, cyclomatic: 1}
				if ext == ".py" {
					baseIndent = lineIndent(line)
				}
				bodyLines = nil
				inFunc = true
				depth = 0
			}
		}
		if inFunc {
			bodyLines = append(bodyLines, line)
			if ext == ".py" {
				trimmed := strings.TrimSpace(line)
				if len(bodyLines) > 1 && trimmed != "" {
					if lineIndent(line) <= baseIndent {
						end := len(bodyLines) - 1
						current.endLine = lineno - 1
						current.nloc = countNLOC(bodyLines[:end])
						current.cyclomatic += countCyclomatic(bodyLines[:end])
						current.maxNesting = maxNestingDepth(bodyLines[:end])
						funcs = append(funcs, current)
						inFunc = false
						bodyLines = nil
						// recheck this line as a new function start
						if m2 := re.FindStringSubmatch(line); m2 != nil {
							if name := extractMatchedName(m2, ext); name != "" {
								current = funcInfo{name: name, startLine: lineno, cyclomatic: 1}
								baseIndent = lineIndent(line)
								bodyLines = []string{line}
								inFunc = true
							}
						}
					}
				}
			} else {
				for _, ch := range line {
					if ch == '{' {
						depth++
					} else if ch == '}' {
						depth--
					}
				}
				if depth <= 0 && len(bodyLines) > 1 {
					current.endLine = lineno
					current.nloc = countNLOC(bodyLines)
					current.cyclomatic += countCyclomatic(bodyLines)
					current.maxNesting = maxNestingDepth(bodyLines)
					funcs = append(funcs, current)
					inFunc = false
					bodyLines = nil
				}
			}
		}
	}
	if inFunc && len(bodyLines) > 0 {
		current.endLine = len(lines)
		current.nloc = countNLOC(bodyLines)
		current.cyclomatic += countCyclomatic(bodyLines)
		current.maxNesting = maxNestingDepth(bodyLines)
		funcs = append(funcs, current)
	}
	return funcs
}

func extractMatchedName(m []string, ext string) string {
	switch ext {
	case ".py", ".java", ".kt":
		return m[1]
	case ".ts", ".tsx", ".js", ".jsx":
		if m[1] != "" {
			return m[1]
		}
		return m[2]
	}
	return ""
}

func lineIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// parseGoParams extracts base type names from a Go parameter list.
func parseGoParams(paramStr string) []string {
	paramStr = strings.TrimSpace(paramStr)
	if paramStr == "" {
		return nil
	}
	var types []string
	for _, part := range strings.Split(paramStr, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		t := fields[len(fields)-1]
		t = strings.TrimPrefix(t, "*")
		t = strings.TrimPrefix(t, "...")
		t = strings.TrimPrefix(t, "[]")
		types = append(types, t)
	}
	return types
}

// countNLOC counts non-blank, non-comment lines.
func countNLOC(lines []string) int {
	count := 0
	inBlock := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if inBlock {
			if strings.Contains(t, "*/") {
				inBlock = false
			}
			continue
		}
		if strings.HasPrefix(t, "/*") {
			inBlock = !strings.Contains(t, "*/")
			continue
		}
		if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
			continue
		}
		count++
	}
	return count
}

// branchRe matches branch/loop keywords that add cyclomatic complexity.
var branchRe = regexp.MustCompile(`\bif\b|\belse if\b|\bfor\b|\bcase\b|\bcatch\b|\b&&\b|\|\|`)

// countCyclomatic counts additional branch points in function body lines.
func countCyclomatic(lines []string) int {
	count := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
			continue
		}
		count += len(branchRe.FindAllString(line, -1))
	}
	return count
}

// maxNestingDepth returns the maximum brace-based nesting depth in lines.
func maxNestingDepth(lines []string) int {
	depth, maxD := 0, 0
	for _, line := range lines {
		for _, ch := range line {
			switch ch {
			case '{':
				depth++
				if depth > maxD {
					maxD = depth
				}
			case '}':
				if depth > 0 {
					depth--
				}
			}
		}
	}
	return maxD
}

// branchLineRe detects branch-start lines for bumpy-road detection.
var branchLineRe = regexp.MustCompile(`^(\s*)(?:if\s|case\s|case\t|default:)`)

// hasBumpyRoad returns true if there are ≥bumbyRoadBranchMin sequential branch
// lines at the same indentation level within the given lines slice.
func hasBumpyRoad(lines []string) bool {
	seq, maxSeq, lastIndent := 0, 0, -1
	for _, line := range lines {
		m := branchLineRe.FindStringSubmatch(line)
		if m != nil {
			indent := len(m[1])
			if indent == lastIndent {
				seq++
			} else {
				seq = 1
				lastIndent = indent
			}
			if seq > maxSeq {
				maxSeq = seq
			}
		} else if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "//") {
			seq = 0
			lastIndent = -1
		}
	}
	return maxSeq >= bumbyRoadBranchMin
}

// computePageRankThreshold returns the PageRank value at the (1-topPct) percentile
// for file nodes, i.e. the minimum score to be in the top topPct fraction.
func computePageRankThreshold(graph *GraphEngine, topPct float64) float64 {
	if graph == nil {
		return 0
	}
	var scores []float64
	for _, n := range graph.GetNodes() {
		if n.InternalType() == NodeTypeFile {
			scores = append(scores, n.PageRank)
		}
	}
	if len(scores) == 0 {
		return 0
	}
	sort.Float64s(scores)
	idx := int(float64(len(scores)) * (1.0 - topPct))
	if idx >= len(scores) {
		idx = len(scores) - 1
	}
	return scores[idx]
}

// extractFuncs dispatches to the correct per-language extractor.
func extractFuncs(content, ext string) []funcInfo {
	if ext == ".go" {
		return extractGoFunctions(content)
	}
	return extractGenericFunctions(content, ext)
}

// checkStructuralAndSizeMarkers runs Phase 5.1 and 5.2 markers in a single
// function-extraction pass.
func (me *MarkerEngine) checkStructuralAndSizeMarkers(
	filePath, content, ext string,
	prThreshold float64,
	fileNode *Node,
) []models.Marker {
	var markers []models.Marker
	funcs := extractFuncs(content, ext)
	if len(funcs) == 0 {
		return nil
	}

	isTopPageRank := fileNode != nil && fileNode.PageRank >= prThreshold && prThreshold > 0
	nlThreshold := largeMethodThreshold(ext)
	primTypes := primTypesForExt(ext)
	allLines := strings.Split(content, "\n")

	for _, fn := range funcs {
		// Phase 5.1: brain_method
		if fn.nloc > brainMethodNLOCThreshold &&
			fn.cyclomatic >= brainMethodCyclomaticMin &&
			fn.maxNesting >= brainMethodNestingMin &&
			isTopPageRank {
			markers = append(markers, models.Marker{
				Type:      "brain_method",
				Severity:  "high",
				Message:   fmt.Sprintf("Brain method '%s': NLOC=%d cyclomatic=%d nesting=%d", fn.name, fn.nloc, fn.cyclomatic, fn.maxNesting),
				File:      filePath,
				Line:      fn.startLine,
				Deduction: 1.5,
				Category:  models.ScoreCatStructural,
			})
		}

		// Phase 5.1: nested_complexity
		if fn.maxNesting >= nestedComplexityDepthMin {
			markers = append(markers, models.Marker{
				Type:      "nested_complexity",
				Severity:  "medium",
				Message:   fmt.Sprintf("Nested complexity in '%s': max depth=%d", fn.name, fn.maxNesting),
				File:      filePath,
				Line:      fn.startLine,
				Deduction: 1.0,
				Category:  models.ScoreCatStructural,
			})
		}

		// Phase 5.1: bumpy_road
		start := fn.startLine - 1
		end := fn.endLine
		if start < 0 {
			start = 0
		}
		if end > len(allLines) {
			end = len(allLines)
		}
		if hasBumpyRoad(allLines[start:end]) {
			markers = append(markers, models.Marker{
				Type:      "bumpy_road",
				Severity:  "medium",
				Message:   fmt.Sprintf("Bumpy road in '%s': ≥%d sequential branches at same level", fn.name, bumbyRoadBranchMin),
				File:      filePath,
				Line:      fn.startLine,
				Deduction: 1.0,
				Category:  models.ScoreCatStructural,
			})
		}

		// Phase 5.2: complex_method
		if fn.cyclomatic >= complexMethodCyclomaticMin {
			markers = append(markers, models.Marker{
				Type:      "complex_method",
				Severity:  "medium",
				Message:   fmt.Sprintf("Complex method '%s': cyclomatic=%d", fn.name, fn.cyclomatic),
				File:      filePath,
				Line:      fn.startLine,
				Deduction: 0.8,
				Category:  models.ScoreCatSize,
			})
		}

		// Phase 5.2: large_method
		if fn.nloc > nlThreshold {
			markers = append(markers, models.Marker{
				Type:      "large_method",
				Severity:  "low",
				Message:   fmt.Sprintf("Large method '%s': %d NLOC (threshold %d)", fn.name, fn.nloc, nlThreshold),
				File:      filePath,
				Line:      fn.startLine,
				Deduction: 0.6,
				Category:  models.ScoreCatSize,
			})
		}

		// Phase 5.2: primitive_obsession
		primCount := 0
		for _, p := range fn.params {
			if primTypes[p] {
				primCount++
			}
		}
		if primCount >= primitiveObsessionParamMin {
			markers = append(markers, models.Marker{
				Type:      "primitive_obsession",
				Severity:  "low",
				Message:   fmt.Sprintf("Primitive obsession in '%s': %d primitive params", fn.name, primCount),
				File:      filePath,
				Line:      fn.startLine,
				Deduction: 0.6,
				Category:  models.ScoreCatSize,
			})
		}
	}
	return markers
}

// windowEntry records a rolling-hash window location.
type windowEntry struct {
	file    string
	lineNum int
	tokens  []string
	lastMod time.Time
}

// checkDRYViolations runs Rabin–Karp rolling hash across files to detect clones.
// Phase 5.3: dry_violation.
func (me *MarkerEngine) checkDRYViolations(files []models.FileNode) []models.Marker {
	hashMap := make(map[uint64]*windowEntry)
	seen := make(map[string]bool) // file-pair keys already marked
	var markers []models.Marker
	cutoff := time.Now().AddDate(0, 0, -dryActiveDaysThreshold)

	for _, f := range files {
		if !f.IsFile {
			continue
		}
		ext := filepath.Ext(f.Path)
		if !supportedStructuralExt(ext) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(me.repoPath, f.Path))
		if err != nil {
			continue
		}

		tokens := tokenizeSource(string(content))
		if len(tokens) < dryHashWindow {
			continue
		}
		lines := strings.Split(string(content), "\n")

		for i := 0; i <= len(tokens)-dryHashWindow; i += dryHashStride {
			window := tokens[i : i+dryHashWindow]
			h := rollingHash(window)
			lineNum := estimateLineNum(tokens, i, lines)

			prev, exists := hashMap[h]
			if exists && prev.file != f.Path {
				sim := jaccard(window, prev.tokens)
				if sim >= dryHashMinSimilarity {
					pairKey := pairKey(f.Path, prev.file)
					if !seen[pairKey] {
						seen[pairKey] = true
						deduction := 1.5
						if f.LastMod.After(cutoff) && prev.lastMod.After(cutoff) {
							deduction = min(1.5, deduction*dryActiveDeductionMultiplier)
						}
						markers = append(markers, models.Marker{
							Type:      "dry_violation",
							Severity:  "medium",
							Message:   fmt.Sprintf("Clone detected: similar code to '%s' (line ~%d)", prev.file, prev.lineNum),
							File:      f.Path,
							Line:      lineNum,
							Deduction: deduction,
							Category:  models.ScoreCatDuplication,
						})
					}
				}
			} else if !exists {
				hashMap[h] = &windowEntry{
					file:    f.Path,
					lineNum: lineNum,
					tokens:  window,
					lastMod: f.LastMod,
				}
			}
		}
	}
	return markers
}

func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// tokenizeSource splits source into word/identifier tokens.
func tokenizeSource(content string) []string {
	var tokens []string
	var cur strings.Builder
	for _, ch := range content {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
			cur.WriteRune(ch)
		} else {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// rollingHash computes a polynomial hash over a token window.
func rollingHash(tokens []string) uint64 {
	const prime = 31
	var h uint64
	for _, t := range tokens {
		h = h*prime + strHash(t)
	}
	return h
}

func strHash(s string) uint64 {
	var h uint64
	for _, c := range s {
		h = h*31 + uint64(c)
	}
	return h
}

// jaccard computes the Jaccard similarity coefficient between two token slices.
func jaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	setA := make(map[string]bool, len(a))
	for _, t := range a {
		setA[t] = true
	}
	setB := make(map[string]bool, len(b))
	for _, t := range b {
		setB[t] = true
	}
	inter := 0
	for t := range setA {
		if setB[t] {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// estimateLineNum maps the token at idx back to an approximate line number.
func estimateLineNum(tokens []string, idx int, lines []string) int {
	if idx >= len(tokens) || len(lines) == 0 {
		return 1
	}
	target := tokens[idx]
	charPos := 0
	for li, line := range lines {
		if pos := strings.Index(line, target); pos >= 0 {
			_ = charPos
			return li + 1
		}
		charPos += len(line) + 1
	}
	return 1
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
