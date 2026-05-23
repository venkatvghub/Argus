package analysis

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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
	case ".go":
		return largeMethodNLOCGo
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

// scanLineBraceDepth adjusts depth for '{' and '}' on line, skipping braces inside
// double-quoted and single-quoted literals. When allowRaw is true, backtick strings
// are also treated as literal. If maxD is non-nil, it tracks the peak depth seen.
func scanLineBraceDepth(line string, depth int, allowRaw bool, maxD *int, inDouble, inSingle, inRaw, prevEscape *bool) int {
	for _, ch := range line {
		if *inRaw {
			if ch == '`' {
				*inRaw = false
			}
			continue
		}
		if *inDouble {
			if *prevEscape {
				*prevEscape = false
				continue
			}
			if ch == '\\' {
				*prevEscape = true
				continue
			}
			if ch == '"' {
				*inDouble = false
			}
			continue
		}
		if *inSingle {
			if *prevEscape {
				*prevEscape = false
				continue
			}
			if ch == '\\' {
				*prevEscape = true
				continue
			}
			if ch == '\'' {
				*inSingle = false
			}
			continue
		}
		*prevEscape = false
		switch ch {
		case '"':
			*inDouble = true
		case '\'':
			*inSingle = true
		case '`':
			if allowRaw {
				*inRaw = true
			}
		case '{':
			depth++
			if maxD != nil && depth > *maxD {
				*maxD = depth
			}
		case '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

// updateBraceDepthIgnoreStrings adjusts depth for '{' and '}' on line, skipping braces
// inside double-quoted and single-quoted literals. When allowRaw is true, backtick
// strings are also treated as literal (JS/TS/Go).
func updateBraceDepthIgnoreStrings(line string, depth int, allowRaw bool, inDouble, inSingle, inRaw, prevEscape *bool) int {
	return scanLineBraceDepth(line, depth, allowRaw, nil, inDouble, inSingle, inRaw, prevEscape)
}

// extractGoFunctions parses Go source using brace counting and returns per-function metrics.
func extractGoFunctions(content string) []funcInfo {
	lines := strings.Split(content, "\n")
	var funcs []funcInfo
	inFunc := false
	depth := 0
	var inDouble, inSingle, inRaw, prevEscape bool
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
				inDouble, inSingle, inRaw, prevEscape = false, false, false, false
			}
		}
		if inFunc {
			depth = updateBraceDepthIgnoreStrings(line, depth, true, &inDouble, &inSingle, &inRaw, &prevEscape)
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

var (
	pythonDefRe     = regexp.MustCompile(`\bdef\s+(\w+)\s*\(`)
	jsNamedFuncRe   = regexp.MustCompile(`function\s+(\w+)\s*\(`)
	jsVarFuncRe     = regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?function\s*\(`)
	jsArrowAssignRe = regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s+)?\(`)
	javaMethodRe    = regexp.MustCompile(`(?:public|private|protected|static|\s)+[\w<>\[\],.]+\s+(\w+)\s*\(`)
	kotlinFunRe     = regexp.MustCompile(`\bfun\s+(\w+)\s*\(`)
	jsArrowTailRe   = regexp.MustCompile(`=>`)
)

// extractGenericFunctions extracts function info for non-Go languages.
func extractGenericFunctions(content, ext string) []funcInfo {
	switch ext {
	case ".java", ".kt", ".ts", ".tsx", ".js", ".jsx", ".py":
	default:
		return nil
	}
	lines := strings.Split(content, "\n")
	var funcs []funcInfo
	inFunc := false
	var current funcInfo
	var bodyLines []string
	depth := 0
	baseIndent := 0
	var inDouble, inSingle, inRaw, prevEscape bool
	allowRaw := ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx"

	beginFunc := func(name string, lineno int, line string) {
		current = funcInfo{name: name, startLine: lineno, cyclomatic: 1}
		if ext == ".py" {
			baseIndent = lineIndent(line)
		}
		bodyLines = nil
		inFunc = true
		depth = 0
		inDouble, inSingle, inRaw, prevEscape = false, false, false, false
	}

	for i, line := range lines {
		lineno := i + 1
		if !inFunc {
			if name, ok := tryMatchGenericFunc(lines, i, ext); ok {
				beginFunc(name, lineno, line)
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
						if name, ok := tryMatchGenericFunc(lines, i, ext); ok {
							beginFunc(name, lineno, line)
							bodyLines = []string{line}
						}
					}
				}
			} else {
				depth = updateBraceDepthIgnoreStrings(line, depth, allowRaw, &inDouble, &inSingle, &inRaw, &prevEscape)
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

func tryMatchGenericFunc(lines []string, idx int, ext string) (string, bool) {
	switch ext {
	case ".py":
		return tryMatchPythonFunc(lines, idx)
	case ".ts", ".tsx", ".js", ".jsx":
		return tryMatchJSFunc(lines, idx)
	case ".java", ".kt":
		return tryMatchJavaKotlinFunc(lines, idx, ext)
	default:
		return "", false
	}
}

func tryMatchPythonFunc(lines []string, idx int) (string, bool) {
	line := lines[idx]
	m := pythonDefRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	openIdx := strings.Index(line, m[0]) + strings.LastIndex(m[0], "(")
	closeLine, closeCol, ok := findClosingParen(lines, idx, openIdx)
	if !ok || !pythonHeaderHasColon(lines, closeLine, closeCol) {
		return "", false
	}
	return m[1], true
}

func tryMatchJSFunc(lines []string, idx int) (string, bool) {
	line := lines[idx]
	type match struct {
		name    string
		openIdx int
		isArrow bool
	}
	var candidate *match
	try := func(re *regexp.Regexp, isArrow bool) {
		if candidate != nil {
			return
		}
		loc := re.FindStringSubmatchIndex(line)
		if loc == nil {
			return
		}
		openIdx := strings.LastIndex(line[:loc[1]], "(")
		if openIdx < 0 {
			return
		}
		candidate = &match{name: line[loc[2]:loc[3]], openIdx: openIdx, isArrow: isArrow}
	}
	try(jsNamedFuncRe, false)
	try(jsVarFuncRe, false)
	try(jsArrowAssignRe, true)
	if candidate == nil {
		return "", false
	}
	closeLine, closeCol, ok := findClosingParen(lines, idx, candidate.openIdx)
	if !ok {
		return "", false
	}
	if candidate.isArrow && !jsHeaderHasArrow(lines, closeLine, closeCol) {
		return "", false
	}
	return candidate.name, true
}

func tryMatchJavaKotlinFunc(lines []string, idx int, ext string) (string, bool) {
	line := lines[idx]
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "@") {
		return "", false
	}
	var m []string
	if ext == ".kt" {
		m = kotlinFunRe.FindStringSubmatch(line)
	}
	if m == nil {
		m = javaMethodRe.FindStringSubmatch(line)
	}
	if m == nil {
		return "", false
	}
	openIdx := strings.Index(line, m[0]) + strings.LastIndex(m[0], "(")
	if _, _, ok := findClosingParen(lines, idx, openIdx); !ok {
		return "", false
	}
	return m[1], true
}

func findClosingParen(lines []string, startLine, startCol int) (endLine, endCol int, ok bool) {
	if startLine < 0 || startLine >= len(lines) || startCol < 0 || startCol >= len(lines[startLine]) {
		return 0, 0, false
	}
	if lines[startLine][startCol] != '(' {
		return 0, 0, false
	}
	depth := 0
	for li := startLine; li < len(lines); li++ {
		colStart := 0
		if li == startLine {
			colStart = startCol
		}
		for col := colStart; col < len(lines[li]); col++ {
			switch lines[li][col] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return li, col, true
				}
			}
		}
	}
	return 0, 0, false
}

func pythonHeaderHasColon(lines []string, closeLine, closeCol int) bool {
	for li := closeLine; li < len(lines) && li <= closeLine+2; li++ {
		start := 0
		if li == closeLine {
			start = closeCol + 1
		}
		rest := strings.TrimSpace(stripHashComment(lines[li][start:]))
		if rest == "" {
			continue
		}
		beforeBrace := rest
		if idx := strings.Index(rest, "{"); idx >= 0 {
			beforeBrace = rest[:idx]
		}
		return strings.Contains(beforeBrace, ":")
	}
	return false
}

func jsHeaderHasArrow(lines []string, closeLine, closeCol int) bool {
	for li := closeLine; li < len(lines) && li <= closeLine+2; li++ {
		start := 0
		if li == closeLine {
			start = closeCol + 1
		}
		rest := stripLineComment(lines[li][start:], "//")
		if idx := strings.Index(rest, "{"); idx >= 0 {
			rest = rest[:idx]
		}
		if jsArrowTailRe.MatchString(rest) {
			return true
		}
		if strings.TrimSpace(rest) != "" {
			return false
		}
	}
	return false
}

func stripHashComment(s string) string {
	if idx := strings.Index(s, "#"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func stripLineComment(s, marker string) string {
	if idx := strings.Index(s, marker); idx >= 0 {
		return s[:idx]
	}
	return s
}

func lineIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// parseGoParams extracts normalized type names from a Go parameter list.
func parseGoParams(paramStr string) []string {
	paramStr = strings.TrimSpace(paramStr)
	if paramStr == "" {
		return nil
	}

	src := "package p\nfunc _ (" + paramStr + ") {}"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil
	}

	var params []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type == nil || fn.Type.Params == nil {
			continue
		}
		for _, field := range fn.Type.Params.List {
			typeStr := normalizeGoType(typeExprString(fset, field.Type))
			if typeStr == "" {
				continue
			}
			n := len(field.Names)
			if n == 0 {
				n = 1
			}
			for range n {
				params = append(params, typeStr)
			}
		}
		break
	}
	return params
}

func typeExprString(fset *token.FileSet, expr ast.Expr) string {
	var buf strings.Builder
	if err := format.Node(&buf, fset, expr); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func normalizeGoType(typeStr string) string {
	t := strings.TrimSpace(typeStr)
	for {
		switch {
		case strings.HasPrefix(t, "..."):
			t = strings.TrimPrefix(t, "...")
		case strings.HasPrefix(t, "*"):
			t = strings.TrimPrefix(t, "*")
		case strings.HasPrefix(t, "[]"):
			t = strings.TrimPrefix(t, "[]")
		default:
			return strings.TrimSpace(t)
		}
	}
}

// lineHasNLOCCode scans line for non-comment code, updating block/string state.
func lineHasNLOCCode(line string, inBlock bool, inDouble, inSingle, inRaw, prevEscape *bool) (hasCode bool, outBlock bool) {
	outBlock = inBlock
	runes := []rune(line)
	for i := 0; i < len(runes); {
		if outBlock {
			if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '/' {
				i += 2
				outBlock = false
				continue
			}
			i++
			continue
		}

		if *inRaw {
			if runes[i] == '`' {
				*inRaw = false
			}
			i++
			continue
		}
		if *inDouble {
			if *prevEscape {
				*prevEscape = false
				i++
				continue
			}
			if runes[i] == '\\' {
				*prevEscape = true
				i++
				continue
			}
			if runes[i] == '"' {
				*inDouble = false
			}
			i++
			continue
		}
		if *inSingle {
			if *prevEscape {
				*prevEscape = false
				i++
				continue
			}
			if runes[i] == '\\' {
				*prevEscape = true
				i++
				continue
			}
			if runes[i] == '\'' {
				*inSingle = false
			}
			i++
			continue
		}
		*prevEscape = false

		switch {
		case runes[i] == '/' && i+1 < len(runes) && runes[i+1] == '/':
			return hasCode, outBlock
		case runes[i] == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			outBlock = true
			for i < len(runes) && outBlock {
				if i+1 < len(runes) && runes[i] == '*' && runes[i+1] == '/' {
					i += 2
					outBlock = false
					break
				}
				i++
			}
		case runes[i] == '#':
			return hasCode, outBlock
		case runes[i] == '"':
			*inDouble = true
			i++
		case runes[i] == '\'':
			*inSingle = true
			i++
		case runes[i] == '`':
			*inRaw = true
			i++
		default:
			if !unicode.IsSpace(runes[i]) {
				hasCode = true
			}
			i++
		}
	}
	return hasCode, outBlock
}

// countNLOC counts non-blank, non-comment lines.
func countNLOC(lines []string) int {
	count := 0
	inBlock := false
	var inDouble, inSingle, inRaw, prevEscape bool
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		var hasCode bool
		hasCode, inBlock = lineHasNLOCCode(line, inBlock, &inDouble, &inSingle, &inRaw, &prevEscape)
		if hasCode {
			count++
		}
	}
	return count
}

// stripCodeLiterals removes double-quoted, single-quoted, and raw backtick
// literal contents so keyword scans do not match text inside strings.
func stripCodeLiterals(line string) string {
	var b strings.Builder
	inDouble, inSingle, inRaw, prevEscape := false, false, false, false
	for _, ch := range line {
		if inRaw {
			if ch == '`' {
				inRaw = false
			}
			continue
		}
		if inDouble {
			if prevEscape {
				prevEscape = false
				continue
			}
			if ch == '\\' {
				prevEscape = true
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		if inSingle {
			if prevEscape {
				prevEscape = false
				continue
			}
			if ch == '\\' {
				prevEscape = true
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		prevEscape = false
		switch ch {
		case '"':
			inDouble = true
		case '\'':
			inSingle = true
		case '`':
			inRaw = true
		default:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// branchRe matches branch/loop keywords that add cyclomatic complexity.
var branchRe = regexp.MustCompile(`\belse if\b|\bif\b|\bfor\b|\bwhile\b|\bcase\b|\bcatch\b|\bdefault\b|\?[^:]*:|\b&&\b|\|\|`)

// countCyclomatic counts additional branch points in function body lines.
func countCyclomatic(lines []string) int {
	count := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
			continue
		}
		count += len(branchRe.FindAllString(stripCodeLiterals(line), -1))
	}
	return count
}

// maxNestingDepth returns the maximum brace-based nesting depth in lines.
func maxNestingDepth(lines []string) int {
	depth, maxD := 0, 0
	var inDouble, inSingle, inRaw, prevEscape bool
	for _, line := range lines {
		depth = scanLineBraceDepth(line, depth, true, &maxD, &inDouble, &inSingle, &inRaw, &prevEscape)
	}
	return maxD
}

// branchLineRe detects branch-start lines for bumpy-road detection.
var branchLineRe = regexp.MustCompile(`^(\s*)(?:if\s|case\s|case\t|default:)`)

// hasBumpyRoad returns true if there are ≥bumpyRoadBranchMin sequential branch
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
	return maxSeq >= bumpyRoadBranchMin
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
				Message:   fmt.Sprintf("Bumpy road in '%s': ≥%d sequential branches at same level", fn.name, bumpyRoadBranchMin),
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
// contents supplies already-read file bodies keyed by path; missing entries fall back to disk.
func (me *MarkerEngine) checkDRYViolations(files []models.FileNode, contents map[string]string) []models.Marker {
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
		content, ok := contents[f.Path]
		if !ok {
			b, err := os.ReadFile(filepath.Join(me.repoPath, f.Path))
			if err != nil {
				continue
			}
			content = string(b)
		}

		tokens := tokenizeSource(content)
		if len(tokens) < dryHashWindow {
			continue
		}
		lines := strings.Split(content, "\n")

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
						deduction := 1.0
						if f.LastMod.After(cutoff) && prev.lastMod.After(cutoff) {
							deduction *= dryActiveDeductionMultiplier
						}
						deduction = min(1.5, deduction)
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
	for li, line := range lines {
		if strings.Contains(line, target) {
			return li + 1
		}
	}
	return 1
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
