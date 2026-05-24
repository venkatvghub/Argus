package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

// TestParseLCOV parses a minimal lcov.info with 2 files and validates coverage calculation.
func TestParseLCOV(t *testing.T) {
	lCOVData := `SF:test.go
FN:10,TestFunc
FNDA:10,TestFunc
FNF:1
FNH:1
DA:10,5
DA:11,5
DA:12,0
LH:2
LF:3
end_of_record
SF:other.go
FN:20,OtherFunc
FNDA:0,OtherFunc
FNF:1
FNH:0
DA:20,0
DA:21,10
LH:1
LF:2
end_of_record
`

	coverage := parseLCOV([]byte(lCOVData))

	require.NotNil(t, coverage)
	assert.Len(t, coverage, 2)

	// test.go: 2 hit / 3 total = 66.67%
	assert.InDelta(t, 66.67, coverage["test.go"], 0.1)

	// other.go: 1 hit / 2 total = 50%
	assert.InDelta(t, 50.0, coverage["other.go"], 0.1)
}

// TestParseCobertura parses a minimal Cobertura coverage.xml.
func TestParseCobertura(t *testing.T) {
	coberturaXML := `<?xml version="1.0" ?>
<coverage line-rate="0.65" version="5.5">
  <packages>
    <package name="main" line-rate="0.65">
      <classes>
        <class name="TestClass" filename="main.go" line-rate="0.75" />
        <class name="OtherClass" filename="util.go" line-rate="0.5" />
      </classes>
    </package>
  </packages>
</coverage>
`

	coverage := parseCobertura([]byte(coberturaXML))

	require.NotNil(t, coverage)
	assert.Len(t, coverage, 2)

	// Cobertura stores 0.0-1.0, we convert to 0-100
	assert.InDelta(t, 75.0, coverage["main.go"], 0.1)
	assert.InDelta(t, 50.0, coverage["util.go"], 0.1)
}

// TestParseClover parses a minimal Clover coverage XML.
func TestParseClover(t *testing.T) {
	cloverXML := `<?xml version="1.0"?>
<clover generated="1234567890">
  <project>
    <file name="handler.go">
      <metrics coveredstatements="8" statements="10" />
    </file>
    <file name="service.go">
      <metrics coveredstatements="10" statements="20" />
    </file>
  </project>
</clover>
`

	coverage := parseClover([]byte(cloverXML))

	require.NotNil(t, coverage)
	assert.Len(t, coverage, 2)

	// handler.go: 8/10 = 80%
	assert.InDelta(t, 80.0, coverage["handler.go"], 0.1)

	// service.go: 10/20 = 50%
	assert.InDelta(t, 50.0, coverage["service.go"], 0.1)
}

// TestCheckCoverageMarkers_CoverageGap creates a file with low coverage and verifies coverage_gap marker.
func TestCheckCoverageMarkers_CoverageGap(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// File with coverage 10% < 60% threshold
	files := []models.FileNode{
		{
			Path:   "app.go",
			IsFile: true,
			Churn:  15,
		},
	}

	coverage := map[string]float64{
		"app.go": 10.0,
	}

	markers := me.checkCoverageMarkers(files, coverage, nil, 0.9)

	require.NotEmpty(t, markers)
	found := false
	for _, m := range markers {
		if m.Type == "coverage_gap" && m.File == "app.go" {
			found = true
			assert.Equal(t, "medium", m.Severity)
			assert.Equal(t, models.ScoreCatTestCoverage, m.Category)
			assert.True(t, m.Deduction > 0)
			break
		}
	}
	assert.True(t, found, "coverage_gap marker not found")
}

// TestCheckCoverageMarkers_SkipNilCoverage tests that nil coverage map returns no markers.
func TestCheckCoverageMarkers_SkipNilCoverage(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{
			Path:   "app.go",
			IsFile: true,
			Churn:  15,
		},
	}

	markers := me.checkCoverageMarkers(files, nil, nil, 0.9)

	assert.Nil(t, markers)
}

func TestIsTestFilename(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		isTest bool
	}{
		{name: "go", base: "app_test.go", isTest: true},
		{name: "go source", base: "app.go", isTest: false},
		{name: "java", base: "UserServiceTest.java", isTest: true},
		{name: "java tests plural", base: "UserServiceTests.java", isTest: true},
		{name: "python prefix", base: "test_utils.py", isTest: true},
		{name: "python suffix", base: "utils_test.py", isTest: true},
		{name: "js test", base: "config.test.js", isTest: true},
		{name: "js spec", base: "config.spec.js", isTest: true},
		{name: "ts test", base: "config.test.ts", isTest: true},
		{name: "tsx spec", base: "Button.spec.tsx", isTest: true},
		{name: "kotlin test", base: "UserRepositoryTest.kt", isTest: true},
		{name: "ruby rspec", base: "user_spec.rb", isTest: true},
		{name: "ruby spec", base: "user.spec.rb", isTest: true},
		{name: "ruby test", base: "user_test.rb", isTest: true},
		{name: "ruby source", base: "app.rb", isTest: false},
		{name: "dart test", base: "widget_test.dart", isTest: true},
		{name: "terraform test tf", base: "main_test.tf", isTest: true},
		{name: "terraform test hcl", base: "network.tftest.hcl", isTest: true},
		{name: "terraform source", base: "main.tf", isTest: false},
		{name: "non test", base: "handler.go", isTest: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isTest, isTestFilename(tt.base))
		})
	}
}

// TestCheckCoverageMarkers_SkipTestFiles verifies test files are skipped.
func TestCheckCoverageMarkers_SkipTestFiles(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{
			Path:   "app_test.go",
			IsFile: true,
			Churn:  15,
		},
	}

	coverage := map[string]float64{
		"app_test.go": 10.0,
	}

	markers := me.checkCoverageMarkers(files, coverage, nil, 0.9)

	for _, m := range markers {
		assert.NotEqual(t, "coverage_gap", m.Type)
		assert.NotEqual(t, "untested_hotspot", m.Type)
	}
}

func TestCheckCoverageMarkers_SkipUntestedHotspotForTestFiles(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{Path: "app_test.go", IsFile: true, Churn: coverageUntestedChurnThreshold},
	}
	ge := NewGraphEngine()
	require.NoError(t, ge.BuildGraph(files, nil, nil))
	node, ok := ge.GetNodeByPath("app_test.go")
	require.True(t, ok)
	node.PageRank = 1.0

	coverage := map[string]float64{"app_test.go": 5.0}
	markers := me.checkCoverageMarkers(files, coverage, ge, 0.5)

	for _, m := range markers {
		assert.NotEqual(t, "untested_hotspot", m.Type)
	}
}

func TestCheckCoverageMarkers_UntestedHotspotOnSourceFile(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{Path: "app.go", IsFile: true, Churn: coverageUntestedChurnThreshold},
	}
	ge := NewGraphEngine()
	require.NoError(t, ge.BuildGraph(files, nil, nil))
	node, ok := ge.GetNodeByPath("app.go")
	require.True(t, ok)
	node.PageRank = 1.0

	coverage := map[string]float64{"app.go": 5.0}
	markers := me.checkCoverageMarkers(files, coverage, ge, 0.5)

	found := false
	for _, m := range markers {
		if m.Type == "untested_hotspot" && m.File == "app.go" {
			found = true
			break
		}
	}
	assert.True(t, found, "source file with high churn and low coverage should get untested_hotspot")
}

func TestCheckCoverageMarkers_SkipNonGoTestFiles(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	testFiles := []string{
		"UserServiceTest.java",
		"test_utils.py",
		"config.test.ts",
		"Button.spec.tsx",
		"widget_test.dart",
		"main_test.tf",
		"network.tftest.hcl",
	}

	for _, path := range testFiles {
		t.Run(path, func(t *testing.T) {
			files := []models.FileNode{{Path: path, IsFile: true, Churn: coverageUntestedChurnThreshold}}
			coverage := map[string]float64{path: 10.0}
			ge := NewGraphEngine()
			require.NoError(t, ge.BuildGraph(files, nil, nil))
			node, ok := ge.GetNodeByPath(path)
			require.True(t, ok)
			node.PageRank = 1.0
			markers := me.checkCoverageMarkers(files, coverage, ge, 0.9)
			for _, m := range markers {
				assert.NotEqual(t, "coverage_gap", m.Type, "test file %q should not get coverage_gap", path)
				assert.NotEqual(t, "untested_hotspot", m.Type, "test file %q should not get untested_hotspot", path)
			}
		})
	}
}

// TestCheckCoverageMarkers_SkipDirectoryNodes verifies directory nodes are skipped.
func TestCheckCoverageMarkers_SkipDirectoryNodes(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{
			Path:   "cmd/",
			IsFile: false, // directory
			Churn:  15,
		},
	}

	coverage := map[string]float64{
		"cmd/": 10.0,
	}

	markers := me.checkCoverageMarkers(files, coverage, nil, 0.9)

	for _, m := range markers {
		assert.NotEqual(t, m.File, "cmd/")
	}
}

// TestCheckCoverageMarkers_AboveThreshold verifies no marker for good coverage.
func TestCheckCoverageMarkers_AboveThreshold(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{
			Path:   "app.go",
			IsFile: true,
			Churn:  15,
		},
	}

	coverage := map[string]float64{
		"app.go": 85.0,
	}

	markers := me.checkCoverageMarkers(files, coverage, nil, 0.9)

	for _, m := range markers {
		assert.NotEqual(t, m.Type, "coverage_gap", "no coverage_gap for 85% coverage")
	}
}

// TestCheckCoverageMarkers_LowChurnNoMarker verifies low-churn files with poor coverage skip untested_hotspot.
func TestCheckCoverageMarkers_LowChurnNoMarker(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{
			Path:   "init.go",
			IsFile: true,
			Churn:  2, // low churn
		},
	}

	coverage := map[string]float64{
		"init.go": 10.0,
	}

	// nil graph means untested_hotspot won't fire (needs PageRank)
	// but coverage_gap should fire
	markers := me.checkCoverageMarkers(files, coverage, nil, 0.9)

	found := false
	for _, m := range markers {
		if m.Type == "coverage_gap" {
			found = true
		}
	}
	assert.True(t, found, "coverage_gap should still fire for low coverage")
}

func TestLookupCoverage(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		cov, ok := lookupCoverage(map[string]float64{"app.go": 75.0}, "app.go")
		assert.True(t, ok)
		assert.InDelta(t, 75.0, cov, 0.01)
	})

	t.Run("longest suffix match", func(t *testing.T) {
		coverage := map[string]float64{
			"pkg/app.go":                 20.0,
			"/repo/backend/pkg/app.go":   30.0,
		}
		cov, ok := lookupCoverage(coverage, "backend/pkg/app.go")
		assert.True(t, ok)
		assert.InDelta(t, 30.0, cov, 0.01)
	})

	t.Run("no match", func(t *testing.T) {
		cov, ok := lookupCoverage(map[string]float64{"other.go": 50.0}, "missing.go")
		assert.False(t, ok)
		assert.Zero(t, cov)
	})

	t.Run("basename-only key does not suffix-match nested path", func(t *testing.T) {
		cov, ok := lookupCoverage(map[string]float64{"app.go": 75.0}, "pkg/app.go")
		assert.False(t, ok)
		assert.Zero(t, cov)
	})

	t.Run("equal-length suffix ties pick lexicographically smaller key", func(t *testing.T) {
		coverage := map[string]float64{
			"b/pkg/app.go": 30.0,
			"a/pkg/app.go": 40.0,
		}
		cov, ok := lookupCoverage(coverage, "pkg/app.go")
		assert.True(t, ok)
		assert.InDelta(t, 40.0, cov, 0.01)
	})

	t.Run("normalizes path separators", func(t *testing.T) {
		cov, ok := lookupCoverage(map[string]float64{`repo\pkg\app.go`: 55.0}, "pkg/app.go")
		assert.True(t, ok)
		assert.InDelta(t, 55.0, cov, 0.01)
	})
}
