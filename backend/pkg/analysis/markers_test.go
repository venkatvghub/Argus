package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestMarkerEngine_PIIDetection(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir)

	content := "Aadhaar: 123456789012, PAN: ABCDE1234F, UPI: test@okaxis"
	filePath := "test.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	types := make(map[string]bool)
	for _, m := range markers {
		types[m.Type] = true
	}

	assert.True(t, types["dpdp_pii_exposure"])
}

func TestMarkerEngine_TokenBloat(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir)

	// Create a file with very long lines to trigger token bloat
	content := ""
	for i := 0; i < 100; i++ {
		content += "token1 token2 token3 token4 token5 token6 token7 token8 token9 token10 token11 token12 token13 token14 token15 token16 token17 token18 token19 token20 token21 token22 token23 token24 token25 token26 token27 token28 token29 token30 token31 token32 token33 token34 token35 token36 token37 token38 token39 token40 token41 token42 token43 token44 token45 token46 token47 token48 token49 token50 token51\n"
	}
	filePath := "bloat.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	found := false
	for _, m := range markers {
		if m.Type == "token_bloat" {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestMarkerEngine_ZombieExports(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir)
	ge := NewGraphEngine()

	symbols := []models.Symbol{
		{Name: "ZombieFunc", FilePath: "file1.go", Type: models.SymbolFunction, Line: 10},
		{Name: "ActiveFunc", FilePath: "file1.go", Type: models.SymbolFunction, Line: 20},
		{Name: "CallerFunc", FilePath: "file2.go", Type: models.SymbolFunction, Line: 5},
	}
	// Build graph with a call from CallerFunc to ActiveFunc
	require.NoError(t, ge.BuildGraph(nil, symbols, nil))
	require.NoError(t, ge.AddCallEdge("file2.go", "CallerFunc", "file1.go", "ActiveFunc"))

	markers := me.Run(nil, symbols, ge)

	zombies := 0
	for _, m := range markers {
		if m.Type == "zombie_exports" {
			if strings.Contains(m.Message, "ZombieFunc") {
				zombies++
			}
		}
	}
	assert.Equal(t, 1, zombies, "Should have found exactly one zombie export (ZombieFunc)")
}

// ============ detectPhantomCoupling Tests ============

// TestDetectPhantomCoupling_HighChurnLowOwnershipNoEdges verifies that a file
// with high churn (>= 5), low ownership (< 0.5), and zero structural edges
// produces a phantom_coupling marker.
func TestDetectPhantomCoupling_HighChurnLowOwnershipNoEdges(t *testing.T) {
	ge := NewGraphEngine()

	// Create a file node with phantom_coupling attributes
	file := models.FileNode{
		Path:      "src/service/handler.go",
		IsFile:    true,
		Churn:     10,        // >= 5
		Ownership: 0.3,       // < 0.5
	}

	// Build graph with no file-to-file edges
	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	require.Len(t, markers, 1, "Should produce exactly one phantom_coupling marker")
	m := markers[0]
	assert.Equal(t, "phantom_coupling", m.Type)
	assert.Equal(t, "medium", m.Severity)
	assert.Equal(t, file.Path, m.File)
	assert.Contains(t, m.Message, file.Path, "Message should contain file path")
	assert.Contains(t, m.Message, "10", "Message should contain churn value")
	assert.Contains(t, m.Message, "30%", "Message should contain ownership percentage")
}

// TestDetectPhantomCoupling_LowChurnNoMarker verifies that a file with low churn
// (< 5) does not produce a marker, even with low ownership and no edges.
func TestDetectPhantomCoupling_LowChurnNoMarker(t *testing.T) {
	ge := NewGraphEngine()

	file := models.FileNode{
		Path:      "src/util/helpers.go",
		IsFile:    true,
		Churn:     4,         // < 5
		Ownership: 0.2,       // < 0.5
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	assert.Len(t, markers, 0, "Low churn should not produce marker")
}

// TestDetectPhantomCoupling_HighOwnershipNoMarker verifies that a file with
// high ownership (>= 0.5) does not produce a marker, even with high churn and no edges.
func TestDetectPhantomCoupling_HighOwnershipNoMarker(t *testing.T) {
	ge := NewGraphEngine()

	file := models.FileNode{
		Path:      "src/core/engine.go",
		IsFile:    true,
		Churn:     15,        // >= 5
		Ownership: 0.8,       // >= 0.5
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	assert.Len(t, markers, 0, "High ownership should not produce marker")
}

// TestDetectPhantomCoupling_HasStructuralEdgesNoMarker verifies that a file with
// structural edges to other files does not produce a marker, even with high churn
// and low ownership.
func TestDetectPhantomCoupling_HasStructuralEdgesNoMarker(t *testing.T) {
	ge := NewGraphEngine()

	file1 := models.FileNode{
		Path:      "src/api/auth.go",
		IsFile:    true,
		Churn:     8,         // >= 5
		Ownership: 0.3,       // < 0.5
	}
	file2 := models.FileNode{
		Path:      "src/db/models.go",
		IsFile:    true,
		Churn:     5,
		Ownership: 0.4,
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file1, file2}, nil, nil))

	// Add a file-to-file edge (structural relationship)
	require.NoError(t, ge.AddCoChangeEdge(file1.Path, file2.Path))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file1}, ge)

	assert.Len(t, markers, 0, "File with structural edges should not produce marker")
}

// TestDetectPhantomCoupling_DirectoryNodeSkipped verifies that directory nodes
// (IsFile=false) are skipped and do not produce markers.
func TestDetectPhantomCoupling_DirectoryNodeSkipped(t *testing.T) {
	ge := NewGraphEngine()

	dir := models.FileNode{
		Path:      "src/util",
		IsFile:    false,      // Directory, not a file
		Churn:     10,         // >= 5
		Ownership: 0.2,        // < 0.5
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{dir}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{dir}, ge)

	assert.Len(t, markers, 0, "Directory nodes should be skipped")
}

// TestDetectPhantomCoupling_MessageFormat verifies that the marker message
// contains the expected format with file path, churn, and ownership percentage.
func TestDetectPhantomCoupling_MessageFormat(t *testing.T) {
	ge := NewGraphEngine()

	file := models.FileNode{
		Path:      "pkg/complex/logic.go",
		IsFile:    true,
		Churn:     12,
		Ownership: 0.35,
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	require.Len(t, markers, 1)
	m := markers[0]

	// Verify message format contains expected components
	assert.Contains(t, m.Message, "pkg/complex/logic.go", "Message should contain file path")
	assert.Contains(t, m.Message, "12", "Message should contain churn value")
	assert.Contains(t, m.Message, "35%", "Message should contain ownership as percentage")
}

// TestDetectPhantomCoupling_MultipleFiles verifies that when processing multiple
// files, only those matching the criteria produce markers.
func TestDetectPhantomCoupling_MultipleFiles(t *testing.T) {
	ge := NewGraphEngine()

	files := []models.FileNode{
		{
			// Matches criteria: Churn >= 5, Ownership < 0.5, no edges
			Path:      "src/handler.go",
			IsFile:    true,
			Churn:     7,
			Ownership: 0.4,
		},
		{
			// Does not match: Low churn
			Path:      "src/utils.go",
			IsFile:    true,
			Churn:     2,
			Ownership: 0.3,
		},
		{
			// Does not match: High ownership
			Path:      "src/core.go",
			IsFile:    true,
			Churn:     10,
			Ownership: 0.6,
		},
	}

	require.NoError(t, ge.BuildGraph(files, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling(files, ge)

	// Only the first file should produce a marker
	require.Len(t, markers, 1, "Only one file meets phantom_coupling criteria")
	assert.Equal(t, "src/handler.go", markers[0].File)
}

// TestDetectPhantomCoupling_EmptyFiles verifies that an empty file slice
// returns no markers (no panic).
func TestDetectPhantomCoupling_EmptyFiles(t *testing.T) {
	ge := NewGraphEngine()

	require.NoError(t, ge.BuildGraph(nil, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{}, ge)

	// A nil slice and an empty slice are both len==0, and that's correct behavior
	assert.Equal(t, 0, len(markers), "Empty files should produce no markers")
}

// TestDetectPhantomCoupling_BoundaryChurn5 verifies that churn = 5 (exactly at boundary)
// is included in the phantom_coupling detection.
func TestDetectPhantomCoupling_BoundaryChurn5(t *testing.T) {
	ge := NewGraphEngine()

	file := models.FileNode{
		Path:      "src/boundary.go",
		IsFile:    true,
		Churn:     5,         // Exactly at boundary
		Ownership: 0.4,       // < 0.5
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	require.Len(t, markers, 1, "Churn=5 should be included (>= 5)")
}

// TestDetectPhantomCoupling_BoundaryOwnership0_5 verifies that ownership = 0.5 (exactly at boundary)
// does NOT produce a marker (< 0.5, not <=).
func TestDetectPhantomCoupling_BoundaryOwnership0_5(t *testing.T) {
	ge := NewGraphEngine()

	file := models.FileNode{
		Path:      "src/boundary_own.go",
		IsFile:    true,
		Churn:     8,
		Ownership: 0.5,       // Exactly at boundary (should NOT match: < 0.5)
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	assert.Len(t, markers, 0, "Ownership=0.5 should not match (criteria is < 0.5)")
}

// TestDetectPhantomCoupling_AllCriteriaRequired tests a comprehensive table-driven
// scenario to verify all three criteria are independently necessary.
func TestDetectPhantomCoupling_AllCriteriaRequired(t *testing.T) {
	tests := []struct {
		name      string
		file      models.FileNode
		addEdge   bool
		expectMap bool
	}{
		{
			name:      "all criteria met",
			file:      models.FileNode{Path: "a.go", IsFile: true, Churn: 10, Ownership: 0.3},
			addEdge:   false,
			expectMap: true,
		},
		{
			name:      "churn too low",
			file:      models.FileNode{Path: "b.go", IsFile: true, Churn: 4, Ownership: 0.3},
			addEdge:   false,
			expectMap: false,
		},
		{
			name:      "ownership too high",
			file:      models.FileNode{Path: "c.go", IsFile: true, Churn: 10, Ownership: 0.6},
			addEdge:   false,
			expectMap: false,
		},
		{
			name:      "has structural edge",
			file:      models.FileNode{Path: "d.go", IsFile: true, Churn: 10, Ownership: 0.3},
			addEdge:   true,
			expectMap: false,
		},
		{
			name:      "is directory",
			file:      models.FileNode{Path: "e/", IsFile: false, Churn: 10, Ownership: 0.3},
			addEdge:   false,
			expectMap: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ge := NewGraphEngine()
			files := []models.FileNode{tt.file}

			if tt.addEdge {
				// Add a second file to create an edge
				other := models.FileNode{Path: "other.go", IsFile: true}
				files = append(files, other)
				require.NoError(t, ge.BuildGraph(files, nil, nil))
				require.NoError(t, ge.AddCoChangeEdge(tt.file.Path, other.Path))
			} else {
				require.NoError(t, ge.BuildGraph(files, nil, nil))
			}

			me := NewMarkerEngine("")
			markers := me.detectPhantomCoupling([]models.FileNode{tt.file}, ge)

			if tt.expectMap {
				assert.Len(t, markers, 1, "Expected marker for: "+tt.name)
				if len(markers) > 0 {
					assert.Equal(t, "phantom_coupling", markers[0].Type)
					assert.Equal(t, "medium", markers[0].Severity)
				}
			} else {
				assert.Len(t, markers, 0, "Expected no marker for: "+tt.name)
			}
		})
	}
}

// TestDetectPhantomCoupling_CallEdgesDoNotPreventMarker verifies that symbol-level
// edges (e.g., "calls") do not prevent a phantom_coupling marker. Only file-to-file
// structural edges (e.g., "co-change") should count.
func TestDetectPhantomCoupling_CallEdgesDoNotPreventMarker(t *testing.T) {
	ge := NewGraphEngine()

	file1 := models.FileNode{
		Path:      "src/a.go",
		IsFile:    true,
		Churn:     8,
		Ownership: 0.4,
	}
	file2 := models.FileNode{
		Path:      "src/b.go",
		IsFile:    true,
		Churn:     5,
		Ownership: 0.5,
	}

	symbols := []models.Symbol{
		{Name: "FuncA", FilePath: "src/a.go", Type: models.SymbolFunction},
		{Name: "FuncB", FilePath: "src/b.go", Type: models.SymbolFunction},
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file1, file2}, symbols, nil))

	// Add a symbol-level "calls" edge (not a file-to-file edge)
	require.NoError(t, ge.AddCallEdge("src/a.go", "FuncA", "src/b.go", "FuncB"))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file1}, ge)

	// Should still produce marker because symbol-level edge doesn't count as
	// file-to-file structural edge (no co-change or import edge)
	assert.Len(t, markers, 1, "Symbol-level edges should not prevent phantom_coupling marker")
}

// TestDetectPhantomCoupling_OwnershipPercentageFormatting verifies that ownership
// is displayed as a percentage (e.g., "25%" not "0.25").
func TestDetectPhantomCoupling_OwnershipPercentageFormatting(t *testing.T) {
	ge := NewGraphEngine()

	file := models.FileNode{
		Path:      "src/format_test.go",
		IsFile:    true,
		Churn:     6,
		Ownership: 0.25,
	}

	require.NoError(t, ge.BuildGraph([]models.FileNode{file}, nil, nil))

	me := NewMarkerEngine("")
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	require.Len(t, markers, 1)
	// Message should contain "25%" not "0.25"
	assert.Contains(t, markers[0].Message, "25%")
	assert.NotContains(t, markers[0].Message, "0.25")
}
