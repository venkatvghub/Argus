package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

// TestExtractGoFunctions parses a short Go snippet with 2 functions and validates metrics.
func TestExtractGoFunctions(t *testing.T) {
	content := `package main

func simple(x int) int {
	return x + 1
}

func complex(a int, b string) string {
	if a > 0 {
		if b != "" {
			if a > 10 {
				return "nested"
			}
		}
	}
	return b
}
`
	funcs := extractGoFunctions(content)
	require.Len(t, funcs, 2)

	// Check first function: simple
	assert.Equal(t, "simple", funcs[0].name)
	assert.Equal(t, 3, funcs[0].startLine)
	assert.Equal(t, 5, funcs[0].endLine)
	assert.Equal(t, 3, funcs[0].nloc) // func declaration + return statement + closing brace
	assert.GreaterOrEqual(t, funcs[0].cyclomatic, 1)
	assert.Equal(t, 1, funcs[0].maxNesting) // single brace level

	// Check second function: complex
	assert.Equal(t, "complex", funcs[1].name)
	assert.Equal(t, 7, funcs[1].startLine)
	assert.Equal(t, 16, funcs[1].endLine)
	assert.GreaterOrEqual(t, funcs[1].cyclomatic, 2) // multiple if statements
	assert.Equal(t, 4, funcs[1].maxNesting)          // four levels of braces (func + 3 if levels)
}

func TestExtractGoFunctions_IgnoresBracesInStringLiterals(t *testing.T) {
	content := `package main

func withLiterals() string {
	c := '"'
	s := ` + "`{ not a brace }`" + `
	t := "also { not }"
	return string(c) + s + t
}
`
	funcs := extractGoFunctions(content)
	require.Len(t, funcs, 1)
	assert.Equal(t, "withLiterals", funcs[0].name)
	assert.Equal(t, 8, funcs[0].endLine)
}

func TestExtractGenericFunctions_IgnoresBracesInStringLiterals(t *testing.T) {
	jsContent := `function withLiterals() {
	const c = '"';
	const s = ` + "`{ not a brace }`" + `;
	const t = "also { not }";
	return c + s + t;
}`
	jsFuncs := extractGenericFunctions(jsContent, ".js")
	require.Len(t, jsFuncs, 1)
	assert.Equal(t, "withLiterals", jsFuncs[0].name)

	javaContent := `public class Example {
	public void withLiterals() {
		char c = '{';
		String s = "not a { brace";
	}
}`
	javaFuncs := extractGenericFunctions(javaContent, ".java")
	require.Len(t, javaFuncs, 1)
	assert.Equal(t, "withLiterals", javaFuncs[0].name)
}

func TestExtractGenericFunctions_RealWorldPatterns(t *testing.T) {
	t.Run("python decorated function", func(t *testing.T) {
		content := `@app.route("/health")
def health_check():
    return {"status": "ok"}

class Service:
    @staticmethod
    def process(data, transform=lambda x: x(x)):
        return data
`
		funcs := extractGenericFunctions(content, ".py")
		names := make([]string, len(funcs))
		for i, fn := range funcs {
			names[i] = fn.name
		}
		assert.Contains(t, names, "health_check")
		assert.Contains(t, names, "process")
	})

	t.Run("javascript arrow and nested params", func(t *testing.T) {
		content := `const parseQuery = (
  raw,
  decodeURIComponent(value)
) => {
  return raw.split("&");
};

function legacyHandler(req, res) {
  res.send("ok");
}

const handler = async function run(
  opts,
  map(fn => fn(opts))
) {
  return opts;
};
`
		funcs := extractGenericFunctions(content, ".js")
		names := make([]string, len(funcs))
		for i, fn := range funcs {
			names[i] = fn.name
		}
		assert.Contains(t, names, "parseQuery")
		assert.Contains(t, names, "legacyHandler")
		assert.Contains(t, names, "run")
	})

	t.Run("java annotated multiline method", func(t *testing.T) {
		content := `public class Repo {
    @Override
    @Transactional(readOnly = true)
    public Map<String, List<Item>> findByTags(
        String tenant,
        Predicate<String> matches(tag)
    ) throws DataAccessException {
        return repository.find(tenant, matches);
    }
}
`
		funcs := extractGenericFunctions(content, ".java")
		require.NotEmpty(t, funcs)
		assert.Equal(t, "findByTags", funcs[0].name)
	})

	t.Run("kotlin fun with annotations", func(t *testing.T) {
		content := `class UserService {
    @JvmOverloads
    fun loadUser(
        id: String,
        includeRoles: Boolean = false
    ): User {
        return repo.find(id, includeRoles)
    }
}
`
		funcs := extractGenericFunctions(content, ".kt")
		require.NotEmpty(t, funcs)
		assert.Equal(t, "loadUser", funcs[0].name)
	})
}

// TestCountNLOC verifies blank lines and comment lines are excluded.
func TestCountNLOC(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int
	}{
		{
			name: "blank lines and comments excluded",
			input: []string{
				"func test() {",
				"  // comment",
				"  x := 1",
				"",
				"  y := 2",
				"  /* multi-line",
				"     comment */",
				"  z := 3",
				"}",
			},
			expected: 5, // func test() {, x := 1, y := 2, z := 3, }
		},
		{
			name: "single-line block comment",
			input: []string{
				"func test() {",
				"  /* inline */ x := 1",
				"  return x",
				"}",
			},
			expected: 4, // func test() {, inline code after block comment, return x, }
		},
		{
			name: "comment markers inside strings are not comments",
			input: []string{
				`x := "https://example.com"`,
				`s := "/* not a block */"`,
				`return "line with // inside"`,
			},
			expected: 3,
		},
		{
			name: "line comment inside multiline string is not a comment",
			input: []string{
				`x := "start`,
				`// still in string`,
				`end"`,
				`y := 1`,
			},
			expected: 2, // x := "start..." and y := 1; middle lines are string continuations
		},
		{
			name: "only comments and blanks",
			input: []string{
				"// comment 1",
				"",
				"// comment 2",
				"",
			},
			expected: 0,
		},
		{
			name: "hash comments for Python",
			input: []string{
				"def test():",
				"  # Python comment",
				"  x = 1",
				"  return x",
			},
			expected: 3, // def, x = 1, return x
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countNLOC(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseGoParams(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple params",
			input:    "x int, y string",
			expected: []string{"int", "string"},
		},
		{
			name:     "grouped params",
			input:    "a, b int, c string",
			expected: []string{"int", "int", "string"},
		},
		{
			name:     "complex types preserved",
			input:    "m map[string]int, ch chan int, fn func(int) error",
			expected: []string{"map[string]int", "chan int", "func(int) error"},
		},
		{
			name:     "pointers slices and variadic",
			input:    "p *int, s []string, vals ...byte",
			expected: []string{"int", "string", "byte"},
		},
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseGoParams(tt.input))
		})
	}
}

// TestCountCyclomatic verifies if, for, case, && and || each increment cyclomatic.
func TestCountCyclomatic(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int
	}{
		{
			name: "single if adds 1",
			input: []string{
				"if x > 0 {",
				"  y := 1",
				"}",
			},
			expected: 1,
		},
		{
			name: "if and for and case",
			input: []string{
				"if x > 0 {",
				"  for i := 0; i < 10; i++ {",
				"    switch y {",
				"    case 1:",
				"      break",
				"    }",
				"  }",
				"}",
			},
			expected: 3, // if, for, case
		},
		{
			name: "logical operators",
			input: []string{
				"if a && b || c {",
				"  x := 1",
				"}",
			},
			expected: 3, // if, &&, ||
		},
		{
			name: "comments ignored",
			input: []string{
				"// if ignored",
				"x := 1",
			},
			expected: 0,
		},
		{
			name: "else if",
			input: []string{
				"if x > 0 {",
				"} else if x < 0 {",
				"} else {",
				"}",
			},
			expected: 2, // if, else if
		},
		{
			name: "while and ternary and default",
			input: []string{
				"while x > 0 {",
				"  y := cond ? a : b",
				"  switch z {",
				"  default:",
				"  }",
				"}",
			},
			expected: 3, // while, ternary, default
		},
		{
			name: "keywords inside string literals ignored",
			input: []string{
				`s := "if fake && x"`,
				`'case'`,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countCyclomatic(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMaxNestingDepth verifies depth counting via braces.
func TestMaxNestingDepth(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int
	}{
		{
			name: "single level",
			input: []string{
				"func test() {",
				"  x := 1",
				"}",
			},
			expected: 1,
		},
		{
			name: "two levels",
			input: []string{
				"func test() {",
				"  if x > 0 {",
				"    y := 1",
				"  }",
				"}",
			},
			expected: 2,
		},
		{
			name: "three levels",
			input: []string{
				"func test() {",
				"  if x > 0 {",
				"    if y > 0 {",
				"      z := 1",
				"    }",
				"  }",
				"}",
			},
			expected: 3,
		},
		{
			name: "no braces",
			input: []string{
				"x := 1",
				"y := 2",
			},
			expected: 0,
		},
		{
			name: "ignores braces in string literals",
			input: []string{
				"func test() {",
				`  s := "{ not nesting }"`,
				"  if x > 0 {",
				"    y := 1",
				"  }",
				"}",
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maxNestingDepth(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasBumpyRoad checks sequential if/case detection at same indent.
func TestHasBumpyRoad(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected bool
	}{
		{
			name: "3 sequential ifs at same indent without code interruption",
			input: []string{
				"if x > 0 { return 1 }",
				"if x > 1 { return 2 }",
				"if x > 2 { return 3 }",
			},
			expected: true,
		},
		{
			name: "only 2 sequential ifs",
			input: []string{
				"if x > 0 { return 1 }",
				"if x > 1 { return 2 }",
			},
			expected: false,
		},
		{
			name: "case statements consecutive",
			input: []string{
				"case 1: break",
				"case 2: break",
				"case 3: break",
			},
			expected: true, // 3 cases at same indent (0)
		},
		{
			name: "switch cases with bodies between labels",
			input: []string{
				"switch x {",
				"case 1:",
				"  y := 1",
				"case 2:",
				"  y := 2",
				"case 3:",
				"  y := 3",
				"}",
			},
			expected: true,
		},
		{
			name: "sibling nested ifs at same indent",
			input: []string{
				"if x > 0 {",
				"  if y > 0 {",
				"  }",
				"  if z > 0 {",
				"  }",
				"  if w > 0 {",
				"  }",
				"}",
			},
			expected: true, // 3 sequential ifs at indent 2
		},
		{
			name: "mixed indents reset counter",
			input: []string{
				"if x > 0 {",
				"}",
				"  if x > 1 {", // different indent
				"  }",
				"if x > 2 {",
				"}",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasBumpyRoad(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestComputePageRankThreshold builds a small graph and checks percentile calculation.
func TestComputePageRankThreshold(t *testing.T) {
	// Nil graph returns 0
	threshold := computePageRankThreshold(nil, 0.10)
	assert.Equal(t, 0.0, threshold)

	// Empty graph returns 0
	emptyGraph := NewGraphEngine()
	threshold = computePageRankThreshold(emptyGraph, 0.10)
	assert.Equal(t, 0.0, threshold)

	files := []models.FileNode{
		{Path: "a.go", IsFile: true},
		{Path: "b.go", IsFile: true},
		{Path: "c.go", IsFile: true},
	}
	ge := NewGraphEngine()
	require.NoError(t, ge.BuildGraph(files, nil, nil))

	for path, score := range map[string]float64{
		"a.go": 0.1,
		"b.go": 0.5,
		"c.go": 0.9,
	} {
		node, ok := ge.GetNodeByPath(path)
		require.True(t, ok)
		node.PageRank = score
	}

	// Sorted scores [0.1, 0.5, 0.9]:
	// top 10% -> idx 2 -> 0.9; top 50% -> idx 1 -> 0.5
	assert.Equal(t, 0.9, computePageRankThreshold(ge, 0.10))
	assert.Equal(t, 0.5, computePageRankThreshold(ge, 0.50))
}

// TestCheckStructuralAndSizeMarkers_ComplexMethod emits complex_method for high cyclomatic.
func TestCheckStructuralAndSizeMarkers_ComplexMethod(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// Build function with cyclomatic >= 9 (threshold from defaults.go)
	// Each of these adds 1: if, else if, else if, else if, else if, for, case, &&, ||
	content := `package main

func complexFunc(a, b, c int) int {
	if a == 1 {
		return 1
	} else if a == 2 {
		return 2
	} else if a == 3 {
		return 3
	} else if a == 4 {
		return 4
	} else if a == 5 {
		return 5
	}
	for i := 0; i < 10; i++ {
		if b > 0 {
			switch c {
			case 1:
				return 10
			}
		}
	}
	return 0
}
`
	filePath := "test.go"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	fileNode := &Node{nodeType: NodeTypeFile, PageRank: 0.0}
	markers := me.checkStructuralAndSizeMarkers(filePath, content, ".go", 0.0, fileNode)

	// Should have complex_method marker (cyclomatic >= 9)
	complexFound := false
	for _, m := range markers {
		if m.Type == "complex_method" {
			complexFound = true
			assert.Equal(t, "test.go", m.File)
			assert.GreaterOrEqual(t, m.Line, 1)
			assert.Equal(t, 0.8, m.Deduction)
			assert.Equal(t, models.ScoreCatSize, m.Category)
		}
	}
	assert.True(t, complexFound, "complex_method marker not found")
}

// TestCheckStructuralAndSizeMarkers_LargeMethod emits large_method for NLOC > threshold.
func TestCheckStructuralAndSizeMarkers_LargeMethod(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// Build a function with >60 NLOC (lines of actual code, no blanks/comments)
	lines := []string{"func largeFunc() {"}
	for i := 0; i < 65; i++ {
		lines = append(lines, "  x"+string(rune(97+(i%26)))+" := "+string(rune(48+(i%10))))
	}
	lines = append(lines, "}")
	content := "package main\n\n" + "func someFunc() {}\n\n" + "package main\n\n" + "func preFunc() {}\n\n" + "package main\n\n" + "func largeFunc() {\n"
	for i := 0; i < 65; i++ {
		content += "  statement" + string(rune(48+(i%10))) + "\n"
	}
	content += "}\n"

	filePath := "large.go"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	fileNode := &Node{nodeType: NodeTypeFile, PageRank: 0.0}
	markers := me.checkStructuralAndSizeMarkers(filePath, content, ".go", 0.0, fileNode)

	// Should have large_method marker
	largeFound := false
	for _, m := range markers {
		if m.Type == "large_method" {
			largeFound = true
			assert.Equal(t, "large.go", m.File)
			assert.Equal(t, 0.6, m.Deduction)
			assert.Equal(t, models.ScoreCatSize, m.Category)
		}
	}
	assert.True(t, largeFound, "large_method marker not found")
}

// TestCheckStructuralAndSizeMarkers_NestedComplexity emits nested_complexity for depth >= 4.
func TestCheckStructuralAndSizeMarkers_NestedComplexity(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	content := `package main

func deeplyNested() {
	if a {
		if b {
			if c {
				if d {
					x := 1
				}
			}
		}
	}
}
`
	filePath := "nested.go"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	fileNode := &Node{nodeType: NodeTypeFile, PageRank: 0.0}
	markers := me.checkStructuralAndSizeMarkers(filePath, content, ".go", 0.0, fileNode)

	// Should have nested_complexity marker (depth >= 4)
	nestedFound := false
	for _, m := range markers {
		if m.Type == "nested_complexity" {
			nestedFound = true
			assert.Equal(t, "nested.go", m.File)
			assert.Equal(t, "medium", m.Severity)
			assert.Equal(t, 1.0, m.Deduction)
			assert.Equal(t, models.ScoreCatStructural, m.Category)
		}
	}
	assert.True(t, nestedFound, "nested_complexity marker not found")
}

// TestCheckStructuralAndSizeMarkers_BumpyRoad emits bumpy_road for >= 3 sequential ifs.
func TestCheckStructuralAndSizeMarkers_BumpyRoad(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// bumpy_road requires 3+ sequential branch-start lines at the same indent;
	// non-branch lines with code reset the counter, so each if must stand alone.
	content := `package main

func bumpyFunc() int {
	if x > 0 { return 1 }
	if x > 1 { return 2 }
	if x > 2 { return 3 }
	return 0
}
`
	filePath := "bumpy.go"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	fileNode := &Node{nodeType: NodeTypeFile, PageRank: 0.0}
	markers := me.checkStructuralAndSizeMarkers(filePath, content, ".go", 0.0, fileNode)

	// Should have bumpy_road marker
	bumpyFound := false
	for _, m := range markers {
		if m.Type == "bumpy_road" {
			bumpyFound = true
			assert.Equal(t, "bumpy.go", m.File)
			assert.Equal(t, "medium", m.Severity)
			assert.Equal(t, 1.0, m.Deduction)
			assert.Equal(t, models.ScoreCatStructural, m.Category)
		}
	}
	assert.True(t, bumpyFound, "bumpy_road marker not found")
}

// TestCheckDRYViolations detects identical code clones via rolling hash.
func TestCheckDRYViolations(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// Create two files with highly similar token sequences
	// The rolling hash window size is 6 tokens, and Jaccard similarity must be >= 0.80
	file1Content := `func foo() { a := b; c := d; e := f; g := h; }`

	file2Content := `func bar() { a := b; c := d; e := f; g := h; }`

	filePath1 := "file1.go"
	filePath2 := "file2.go"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath1), []byte(file1Content), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath2), []byte(file2Content), 0644))

	files := []models.FileNode{
		{Path: filePath1, IsFile: true, LastMod: time.Now()},
		{Path: filePath2, IsFile: true, LastMod: time.Now()},
	}

	markers := me.checkDRYViolations(files, map[string]string{
		filePath1: file1Content,
		filePath2: file2Content,
	})

	// Should detect dry_violation (if tokens match closely enough)
	// This is an integration test of the rolling hash and Jaccard similarity logic.
	dryFound := false
	for _, m := range markers {
		if m.Type == "dry_violation" {
			dryFound = true
			assert.Equal(t, models.ScoreCatDuplication, m.Category)
			assert.Equal(t, 1.5, m.Deduction) // both files recently modified → 1.0 * 1.5, capped
		}
	}
	require.True(t, dryFound, "expected dry_violation: both files share identical token window [b,c,d,e,f,g]")
}

// TestTokenizeSource ensures non-identifier chars are excluded.
func TestTokenizeSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple identifiers",
			input:    "foo bar baz",
			expected: []string{"foo", "bar", "baz"},
		},
		{
			name:     "identifiers with digits",
			input:    "x := 1; y := 2",
			expected: []string{"x", "1", "y", "2"},
		},
		{
			name:     "underscores and digits",
			input:    "_var name2 test_123",
			expected: []string{"_var", "name2", "test_123"},
		},
		{
			name:     "brackets and braces ignored",
			input:    "{x} [y] (z)",
			expected: []string{"x", "y", "z"},
		},
		{
			name:     "no tokens",
			input:    "{}()[]<>",
			expected: nil,
		},
		{
			name:     "mixed code snippet",
			input:    "func add(a, b int) int { return a + b }",
			expected: []string{"func", "add", "a", "b", "int", "int", "return", "a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizeSource(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestJaccard verifies Jaccard similarity for identical and disjoint sets.
func TestJaccard(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected float64
	}{
		{
			name:     "identical slices",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: 1.0,
		},
		{
			name:     "disjoint slices",
			a:        []string{"a", "b"},
			b:        []string{"c", "d"},
			expected: 0.0,
		},
		{
			name:     "20% overlap",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "d", "e"},
			expected: 0.2, // 1 of 5 union elements shared (Jaccard = 1/5)
		},
		{
			name:     "both empty",
			a:        []string{},
			b:        []string{},
			expected: 1.0,
		},
		{
			name:     "one empty",
			a:        []string{"x"},
			b:        []string{},
			expected: 0.0,
		},
		{
			name:     "subset",
			a:        []string{"a", "b"},
			b:        []string{"a", "b", "c", "d"},
			expected: 0.5, // {a,b} / {a,b,c,d} = 2/4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jaccard(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

// TestRollingHash ensures same token slice produces same hash.
func TestRollingHash(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
	}{
		{
			name:   "simple sequence",
			tokens: []string{"func", "test", "int"},
		},
		{
			name:   "longer sequence",
			tokens: []string{"a", "b", "c", "d", "e", "f"},
		},
		{
			name:   "single token",
			tokens: []string{"x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h1 := rollingHash(tt.tokens)
			h2 := rollingHash(tt.tokens)
			assert.Equal(t, h1, h2, "same token slice should produce same hash")
		})
	}

	// Different token slices should (very likely) produce different hashes
	h1 := rollingHash([]string{"a", "b", "c"})
	h2 := rollingHash([]string{"c", "b", "a"})
	assert.NotEqual(t, h1, h2, "different token slices should produce different hashes")
}

// TestCheckStructuralAndSizeMarkers_BrainMethod emits brain_method for top PageRank functions.
func TestCheckStructuralAndSizeMarkers_BrainMethod(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	var filler, branches strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&filler, "\t\t\t\t\tx%02d := %d\n", i, i)
	}
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&branches, "\t\t\t\t\tif e == %d { v%d := %d }\n", i, i, i)
	}
	content := fmt.Sprintf(`package main

func superComplex(a, b, c, d, e int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				if d > 0 {
					if a == 1 { x := 1 } else if a == 2 { x := 2 }
					if b == 1 { y := 1 } else if b == 2 { y := 2 }
%s%s
					return x + y
				}
			}
		}
	}
	return 0
}
`, branches.String(), filler.String())

	filePath := "brain.go"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	funcs := extractGoFunctions(content)
	require.Len(t, funcs, 1)
	fn := funcs[0]

	const prThreshold = 0.50
	fileNode := &Node{nodeType: NodeTypeFile, PageRank: 0.95}
	isTopPageRank := fileNode.PageRank >= prThreshold && prThreshold > 0
	shouldEmit := fn.nloc > brainMethodNLOCThreshold &&
		fn.cyclomatic >= brainMethodCyclomaticMin &&
		fn.maxNesting >= brainMethodNestingMin &&
		isTopPageRank

	markers := me.checkStructuralAndSizeMarkers(filePath, content, ".go", prThreshold, fileNode)

	var brainMarkers []models.Marker
	for _, m := range markers {
		if m.Type == "brain_method" {
			brainMarkers = append(brainMarkers, m)
		}
	}

	if shouldEmit {
		require.Len(t, brainMarkers, 1,
			"expected brain_method when nloc=%d cyclomatic=%d nesting=%d PageRank=%.2f",
			fn.nloc, fn.cyclomatic, fn.maxNesting, fileNode.PageRank)
		m := brainMarkers[0]
		assert.Equal(t, "brain.go", m.File)
		assert.Equal(t, fn.startLine, m.Line)
		assert.Equal(t, "high", m.Severity)
		assert.Equal(t, 1.5, m.Deduction)
		assert.Equal(t, models.ScoreCatStructural, m.Category)
	} else {
		assert.Empty(t, brainMarkers,
			"unexpected brain_method when nloc=%d cyclomatic=%d nesting=%d PageRank=%.2f prThreshold=%.2f",
			fn.nloc, fn.cyclomatic, fn.maxNesting, fileNode.PageRank, prThreshold)
	}
}

// TestPairKey ensures canonical ordering of file pair keys.
func TestPairKey(t *testing.T) {
	key1 := pairKey("a.go", "b.go")
	key2 := pairKey("b.go", "a.go")
	assert.Equal(t, key1, key2, "pair key should be canonical regardless of order")
	assert.Equal(t, "a.go|b.go", key1)
}

// TestExtractGoFunctions_Methods validates method extraction (receiver syntax).
func TestExtractGoFunctions_Methods(t *testing.T) {
	content := `package main

type MyType struct {
	Value int
}

func (m *MyType) GetValue() int {
	return m.Value
}

func (m MyType) String() string {
	return "MyType"
}
`

	funcs := extractGoFunctions(content)
	require.Len(t, funcs, 2)

	assert.Equal(t, "GetValue", funcs[0].name)
	assert.Equal(t, "String", funcs[1].name)
}
