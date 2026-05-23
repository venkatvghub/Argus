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
			Path:         "app.go",
			IsFile:       true,
			Churn:        15,
			LineCoverage: 10.0,
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

// TestCheckCoverageMarkers_SkipTestFiles verifies test files are skipped.
func TestCheckCoverageMarkers_SkipTestFiles(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	files := []models.FileNode{
		{
			Path:         "app_test.go",
			IsFile:       true,
			Churn:        15,
			LineCoverage: 10.0,
		},
	}

	coverage := map[string]float64{
		"app_test.go": 10.0,
	}

	markers := me.checkCoverageMarkers(files, coverage, nil, 0.9)

	for _, m := range markers {
		// No coverage_gap markers for test files
		assert.NotEqual(t, m.Type, "coverage_gap")
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
			Path:         "app.go",
			IsFile:       true,
			Churn:        15,
			LineCoverage: 85.0,
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
			Path:         "init.go",
			IsFile:       true,
			Churn:        2, // low churn
			LineCoverage: 10.0,
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
