package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestMarkerEngine_PIIDetection(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

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

func TestMarkerEngine_PIIPatternsFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{PIIPatterns: []string{"AADHAAR"}}
	me := NewMarkerEngine(dir, cfg)

	content := "Aadhaar: 123456789012, PAN: ABCDE1234F, UPI: test@okaxis"
	filePath := "test.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	markers := me.Run([]models.FileNode{{Path: filePath, IsFile: true}}, nil, nil)

	var aadhaarFound, panFound, upiFound bool
	for _, m := range markers {
		switch {
		case strings.Contains(m.Message, "Aadhaar"):
			aadhaarFound = true
		case strings.Contains(m.Message, "PAN"):
			panFound = true
		case strings.Contains(m.Message, "UPI"):
			upiFound = true
		}
	}

	assert.True(t, aadhaarFound, "PIIPatterns AADHAAR should flag Aadhaar exposure")
	assert.False(t, panFound, "PIIPatterns without PAN should not flag PAN")
	assert.False(t, upiFound, "PIIPatterns without UPI_ID should not flag UPI")
}

func TestMarkerEngine_TokenBloat(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

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

func TestMarkerEngine_TokenBloatThresholdFromConfig(t *testing.T) {
	dir := t.TempDir()
	// Single line (~52 tokens) — above default TokenBloatThreshold (50), below raised threshold (60).
	content := strings.TrimSpace(strings.Repeat("word ", 51))
	filePath := "bloat.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))
	files := []models.FileNode{{Path: filePath, IsFile: true}}

	meDefault := NewMarkerEngine(dir, nil)
	defaultMarkers := meDefault.Run(files, nil, nil)
	defaultFound := false
	for _, m := range defaultMarkers {
		if m.Type == "token_bloat" {
			defaultFound = true
			break
		}
	}
	assert.True(t, defaultFound, "default TokenBloatThreshold should detect bloat")

	meCustom := NewMarkerEngine(dir, &config.Config{TokenBloatThreshold: 60})
	customMarkers := meCustom.Run(files, nil, nil)
	for _, m := range customMarkers {
		assert.NotEqual(t, "token_bloat", m.Type, "raised TokenBloatThreshold should suppress detection")
	}
}

func TestMarkerEngine_ZombieExports(t *testing.T) {
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

			me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
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

	me := NewMarkerEngine("", nil)
	markers := me.detectPhantomCoupling([]models.FileNode{file}, ge)

	require.Len(t, markers, 1)
	// Message should contain "25%" not "0.25"
	assert.Contains(t, markers[0].Message, "25%")
	assert.NotContains(t, markers[0].Message, "0.25")
}

// ============ Dart/Flutter Regex Marker Tests ============

func TestCheckDartFlutter_SetStateAfterAwait(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "setState after await triggers marker",
			content: `
async void loadData() {
  final data = await fetchData();
  setState(() {
    items = data;
  });
}`,
			expect: true,
		},
		{
			name: "setState before await is safe",
			content: `
void updateState() {
  setState(() {
    items = [];
  });
  fetchData();
}`,
			expect: false,
		},
		{
			name: "no setState no marker",
			content: `
async void loadData() {
  final data = await fetchData();
  print(data);
}`,
			expect: false,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "test.dart"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "dart_setstate_after_await" {
					found = true
					break
				}
			}

			if tt.expect {
				assert.True(t, found, "Expected dart_setstate_after_await marker")
			} else {
				assert.False(t, found, "Did not expect dart_setstate_after_await marker")
			}
		})
	}
}

func TestCheckDartFlutter_ContextAfterAwait(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "context used after await triggers marker",
			content: `
void navigateAfterLoad() async {
  await loadData();
  Navigator.of(context).push(...);
}`,
			expect: true,
		},
		{
			name: "context before await is safe",
			content: `
void navigate() {
  Navigator.of(context).push(...);
  loadData();
}`,
			expect: false,
		},
		{
			name: "no context usage no marker",
			content: `
async void load() {
  final data = await fetchData();
  print(data);
}`,
			expect: false,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "screen.dart"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "dart_context_after_await" {
					found = true
					break
				}
			}

			if tt.expect {
				assert.True(t, found, "Expected dart_context_after_await marker")
			} else {
				assert.False(t, found, "Did not expect dart_context_after_await marker")
			}
		})
	}
}

func TestCheckDartFlutter_BrokenCrypto(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "weak crypto algorithm triggers marker",
			content: `
import 'package:crypto/crypto.dart';

String hashPassword(String pwd) {
  return md5.convert(pwd.codeUnits).toString();
}`,
			expect: true,
		},
		{
			name: "strong crypto is safe",
			content: `
import 'package:crypto/crypto.dart';

String hashPassword(String pwd) {
  return sha256.convert(pwd.codeUnits).toString();
}`,
			expect: false,
		},
		{
			name: "no crypto usage no marker",
			content: `
void printData(String data) {
  print(data);
}`,
			expect: false,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "crypto_util.dart"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "dart_broken_crypto" {
					found = true
					break
				}
			}

			if tt.expect {
				assert.True(t, found, "Expected dart_broken_crypto marker")
			} else {
				assert.False(t, found, "Did not expect dart_broken_crypto marker")
			}
		})
	}
}

func TestCheckDartFlutter_NoMarkerSafeCode(t *testing.T) {
	content := `
void safeFunction(String input) {
  final result = input.toUpperCase();
  print(result);
}

class SafeWidget extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container();
  }
}
`
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	filePath := "safe.dart"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	for _, m := range markers {
		if m.Type == "dart_setstate_after_await" || m.Type == "dart_context_after_await" || m.Type == "dart_broken_crypto" {
			t.Fatalf("Unexpected Dart marker found: %s", m.Type)
		}
	}
}

func TestMarkerEngineRun_DartFileGated(t *testing.T) {
	content := `
async void load() {
  await fetchData();
  setState(() {});
}
`
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// Write to non-Dart file
	filePath := "wrong.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	for _, m := range markers {
		if strings.HasPrefix(m.Type, "dart_") {
			t.Fatalf("Dart marker should not trigger for non-.dart file")
		}
	}

	// Now write to actual .dart file
	dartPath := "screen.dart"
	require.NoError(t, os.WriteFile(filepath.Join(dir, dartPath), []byte(content), 0644))

	files = []models.FileNode{{Path: dartPath, IsFile: true}}
	markers = me.Run(files, nil, nil)

	// Should find setState marker in Dart file
	found := false
	for _, m := range markers {
		if m.Type == "dart_setstate_after_await" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should detect Dart marker in .dart file")
}

// ============ SQL Regex Marker Tests ============

func TestCheckSQL_InjectionRisk(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "SQL concatenation triggers marker",
			content: `
SELECT * FROM users WHERE id = ' + userId + '
SELECT * FROM orders WHERE status = " || status || "
`,
			expect: true,
		},
		{
			name: "parameterized query is safe",
			content: `
SELECT * FROM users WHERE id = ?
SELECT * FROM orders WHERE status = $1
SELECT * FROM products WHERE name = :name
`,
			expect: false,
		},
		{
			name: "literal strings only are safe",
			content: `
SELECT * FROM users WHERE status = 'active'
SELECT * FROM accounts WHERE type = 'premium'
`,
			expect: false,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "query.sql"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "sql_injection_risk" {
					found = true
					break
				}
			}

			if tt.expect {
				assert.True(t, found, "Expected sql_injection_risk marker")
			} else {
				assert.False(t, found, "Did not expect sql_injection_risk marker")
			}
		})
	}
}

func TestCheckSQL_SelectStar(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "SELECT * triggers marker",
			content: `
SELECT * FROM users
SELECT col1, col2 FROM orders
SELECT * FROM products WHERE active = true
`,
			expect: true,
		},
		{
			name: "explicit columns are safe",
			content: `
SELECT id, name, email FROM users
SELECT order_id, total, date FROM orders
SELECT product_id, name, price FROM products
`,
			expect: false,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "schema.sql"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "sql_select_star" {
					found = true
					break
				}
			}

			if tt.expect {
				assert.True(t, found, "Expected sql_select_star marker")
			} else {
				assert.False(t, found, "Did not expect sql_select_star marker")
			}
		})
	}
}

func TestCheckSQL_HardcodedCredential(t *testing.T) {
	tests := []struct {
		name    string
		content string
		expect  bool
	}{
		{
			name: "hardcoded credentials trigger marker",
			content: `
CREATE USER 'admin'@'localhost' IDENTIFIED BY 'MySecurePass123';
ALTER USER 'user1' PASSWORD 'VerySecretPassword456';
`,
			expect: true,
		},
		{
			name: "generic strings are safe",
			content: `
SELECT * FROM users WHERE username = 'testuser'
INSERT INTO audit_log VALUES ('action_performed', 'timestamp')
`,
			expect: false,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "migration.sql"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "sql_hardcoded_credential" {
					found = true
					break
				}
			}

			if tt.expect {
				assert.True(t, found, "Expected sql_hardcoded_credential marker")
			} else {
				assert.False(t, found, "Did not expect sql_hardcoded_credential marker")
			}
		})
	}
}

func TestCheckSQL_NoMarkerParameterized(t *testing.T) {
	content := `
-- Parameterized queries are safe
SELECT * FROM users WHERE id = $1 AND status = $2
SELECT email FROM accounts WHERE username = ?
INSERT INTO transactions (user_id, amount) VALUES (?, ?)
DELETE FROM logs WHERE timestamp < :cutoff_date
UPDATE settings SET value = @newValue WHERE key = @key
`
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	filePath := "safe_queries.sql"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	for _, m := range markers {
		if m.Type == "sql_injection_risk" || m.Type == "sql_hardcoded_credential" {
			t.Fatalf("Unexpected SQL security marker for parameterized query: %s", m.Type)
		}
	}
}

func TestMarkerEngineRun_SQLFileGated(t *testing.T) {
	content := `
SELECT * FROM users WHERE id = ' + userId
`
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	// Write to non-SQL file
	filePath := "readme.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	for _, m := range markers {
		if strings.HasPrefix(m.Type, "sql_") {
			t.Fatalf("SQL marker should not trigger for non-.sql file")
		}
	}

	// Now write to actual .sql file
	sqlPath := "query.sql"
	require.NoError(t, os.WriteFile(filepath.Join(dir, sqlPath), []byte(content), 0644))

	files = []models.FileNode{{Path: sqlPath, IsFile: true}}
	markers = me.Run(files, nil, nil)

	// Should find injection risk marker in SQL file
	found := false
	for _, m := range markers {
		if m.Type == "sql_injection_risk" {
			found = true
			break
		}
	}
	assert.True(t, found, "Should detect SQL marker in .sql file")
}

// ============ PII Mobile & Email Tests ============

// TestCheckPII_IndianMobile tests Indian mobile number detection (DPDP-regulated)
// with various formats: bare 10-digit, +91 prefix, 91 prefix, 0091 prefix with single separator.
func TestCheckPII_IndianMobile(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPII bool // Should marker be present?
	}{
		{
			name:      "bare 10-digit Indian mobile",
			content:   "User phone: 9876543210",
			expectPII: true,
		},
		{
			name:      "Indian mobile with +91 prefix no separator",
			content:   "Contact: +919876543210",
			expectPII: true,
		},
		{
			name:      "Indian mobile with 91 prefix no separator",
			content:   "Mobile: 919876543210",
			expectPII: true,
		},
		{
			name:      "Indian mobile with +91 and dash separator",
			content:   "Phone: +91-9876543210",
			expectPII: true,
		},
		{
			name:      "non-Indian mobile starting with 1",
			content:   "Phone: 1234567890",
			expectPII: false,
		},
		{
			name:      "too short, not valid mobile",
			content:   "Short: 98765",
			expectPII: false,
		},
		{
			name:      "starts with 5, not valid Indian mobile",
			content:   "Invalid: 5876543210",
			expectPII: false,
		},
		{
			name:      "multiple Indian mobiles",
			content:   "Phones: 9876543210, +919876543210, and 919876543210",
			expectPII: true,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "test.txt"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "dpdp_mobile_exposure" {
					found = true
					break
				}
			}

			if tt.expectPII {
				assert.True(t, found, "Expected dpdp_mobile_exposure marker for: "+tt.name)
			} else {
				assert.False(t, found, "Did not expect dpdp_mobile_exposure marker for: "+tt.name)
			}
		})
	}
}

// TestCheckPII_InternationalMobile tests E.164 international mobile detection
// excluding Indian numbers which are caught separately.
func TestCheckPII_InternationalMobile(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPII bool
	}{
		{
			name:      "US phone number E.164",
			content:   "Contact: +14155552671.",
			expectPII: true,
		},
		{
			name:      "UK phone number E.164",
			content:   "Phone: +447700900123.",
			expectPII: true,
		},
		{
			name:      "Indian +91 not counted as international",
			content:   "India: +919876543210",
			expectPII: false,
		},
		{
			name:      "E.164 prefix only, too short",
			content:   "Invalid: +1",
			expectPII: false,
		},
		{
			name:      "Germany phone",
			content:   "Contact: +491234567890.",
			expectPII: true,
		},
		{
			name:      "Australia phone",
			content:   "Aus: +61412345678.",
			expectPII: true,
		},
		{
			name:      "multiple international numbers",
			content:   "Contacts: +14155552671. +447700900123. +491234567890.",
			expectPII: true,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "test.txt"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "pii_mobile_exposure" {
					found = true
					break
				}
			}

			if tt.expectPII {
				assert.True(t, found, "Expected pii_mobile_exposure marker for: "+tt.name)
			} else {
				assert.False(t, found, "Did not expect pii_mobile_exposure marker for: "+tt.name)
			}
		})
	}
}

// TestCheckPII_Email tests email address detection with filtering of test domains.
func TestCheckPII_Email(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPII bool
	}{
		{
			name:      "real company email",
			content:   "Contact: user@company.com",
			expectPII: true,
		},
		{
			name:      "real bank email",
			content:   "Email: john.doe@bank.co.in",
			expectPII: true,
		},
		{
			name:      "test domain example.com filtered",
			content:   "Test: test@example.com",
			expectPII: false,
		},
		{
			name:      "test domain noreply filtered",
			content:   "Email: noreply@example.org",
			expectPII: false,
		},
		{
			name:      "test domain foo.com filtered",
			content:   "Email: admin@foo.com",
			expectPII: false,
		},
		{
			name:      "real startup domain",
			content:   "Contact: user@real-company.io",
			expectPII: true,
		},
		{
			name:      "bar.com test domain filtered",
			content:   "Email: test@bar.com",
			expectPII: false,
		},
		{
			name:      "multiple real emails",
			content:   "Emails: alice@company.com, bob@startup.io, charlie@bank.co.in",
			expectPII: true,
		},
		{
			name:      "mixed real and test emails, should detect real",
			content:   "Emails: admin@foo.com, real@company.com",
			expectPII: true,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "test.txt"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "pii_email_exposure" {
					found = true
					break
				}
			}

			if tt.expectPII {
				assert.True(t, found, "Expected pii_email_exposure marker for: "+tt.name)
			} else {
				assert.False(t, found, "Did not expect pii_email_exposure marker for: "+tt.name)
			}
		})
	}
}

// TestCheckPII_EmailCount verifies that the marker message includes occurrence count.
func TestCheckPII_EmailCount(t *testing.T) {
	content := "Emails: alice@company.com, bob@startup.io, charlie@bank.co.in"
	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	filePath := "test.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	var emailMarker *models.Marker
	for i, m := range markers {
		if m.Type == "pii_email_exposure" {
			emailMarker = &markers[i]
			break
		}
	}

	require.NotNil(t, emailMarker, "Should find email marker")
	assert.Contains(t, emailMarker.Message, "3", "Message should contain count of 3")
	assert.Contains(t, emailMarker.Message, "occurrence", "Message should contain 'occurrence'")
}

// TestCheckPII_NoFalsePositives tests that standard code patterns don't trigger false positives.
func TestCheckPII_NoFalsePositives(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "port number 8080 not flagged as mobile",
			content: "server_port: 8080",
		},
		{
			name:    "port number 3000 not flagged as mobile",
			content: "listen_port: 3000",
		},
		{
			name:    "version string 1.2.3.4 not flagged as mobile",
			content: "version: 1.2.3.4",
		},
		{
			name:    "IPv4 address not flagged as mobile",
			content: "server_ip: 192.168.1.1",
		},
		{
			name:    "IPv4 address not flagged as mobile variant",
			content: "gateway: 10.0.0.1",
		},
		{
			name:    "code example with phone comment",
			content: "// Config for: 8080, 3000, or any port",
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "test.txt"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			for _, m := range markers {
				if m.Type == "dpdp_mobile_exposure" || m.Type == "pii_mobile_exposure" {
					t.Fatalf("Unexpected mobile marker for: %s", tt.name)
				}
			}
		})
	}
}

// TestFilterTestEmails_KnownTestDomains tests the filterTestEmails helper function
// directly to verify test domain filtering behavior.
func TestFilterTestEmails_KnownTestDomains(t *testing.T) {
	tests := []struct {
		name     string
		emails   []string
		expected int // Expected number of emails after filtering
	}{
		{
			name: "example.com filtered",
			emails: []string{
				"test@example.com",
				"admin@example.org",
				"user@example.net",
			},
			expected: 0,
		},
		{
			name: "test.com and derivatives filtered",
			emails: []string{
				"test@test.com",
				"admin@test.org",
			},
			expected: 0,
		},
		{
			name: "foo.com and bar.com filtered",
			emails: []string{
				"user@foo.com",
				"admin@bar.com",
			},
			expected: 0,
		},
		{
			name: "real domains kept",
			emails: []string{
				"user@company.com",
				"alice@startup.io",
				"bob@bank.co.in",
			},
			expected: 3,
		},
		{
			name: "mixed real and test",
			emails: []string{
				"admin@foo.com",
				"real@company.com",
				"test@example.com",
				"alice@startup.io",
			},
			expected: 2,
		},
		{
			name: "case insensitive filtering",
			emails: []string{
				"user@Example.COM",
				"test@TEST.COM",
				"admin@Foo.Com",
				"real@Company.Com",
			},
			expected: 1,
		},
		{
			name: "placeholder.com and noreply.com filtered",
			emails: []string{
				"noreply@placeholder.com",
				"no-reply@noreply.com",
			},
			expected: 0,
		},
		{
			name: "empty list",
			emails: []string{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterTestEmails(tt.emails)
			assert.Equal(t, tt.expected, len(result),
				"Expected %d real emails after filtering, got %d: %v",
				tt.expected, len(result), result)
		})
	}
}

// TestCheckPII_MobileEmailCombined tests that mobile and email markers
// can both be present in the same content without interference.
func TestCheckPII_MobileEmailCombined(t *testing.T) {
	content := `
	Customer Phone: 9876543210.
	Email: john@company.com
	International: +14155552671.
	Landline: 04423456789
	Contact: alice@startup.io
	`

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	filePath := "customer_data.txt"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(content), 0644))

	files := []models.FileNode{{Path: filePath, IsFile: true}}
	markers := me.Run(files, nil, nil)

	typeMap := make(map[string]int)
	for _, m := range markers {
		typeMap[m.Type]++
	}

	assert.Greater(t, typeMap["dpdp_mobile_exposure"], 0, "Should detect Indian mobile")
	assert.Greater(t, typeMap["pii_mobile_exposure"], 0, "Should detect international mobile")
	assert.Greater(t, typeMap["pii_email_exposure"], 0, "Should detect real emails")
}

// TestCheckPII_IndianMobileVariations tests various valid formatting of Indian mobiles.
func TestCheckPII_IndianMobileVariations(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectPII bool
	}{
		{
			name:      "with single space before number",
			content:   "Phone: +91 9876543210",
			expectPII: true,
		},
		{
			name:      "with single dash and +91",
			content:   "Contact: +91-9876543210",
			expectPII: true,
		},
		{
			name:      "with dot separator and 91 prefix",
			content:   "Mobile: 91.9876543210",
			expectPII: true,
		},
		{
			name:      "starts with 6, valid Indian mobile",
			content:   "Phone: 6876543210",
			expectPII: true,
		},
		{
			name:      "starts with 7, valid Indian mobile",
			content:   "Phone: 7876543210",
			expectPII: true,
		},
		{
			name:      "starts with 8, valid Indian mobile",
			content:   "Phone: 8876543210",
			expectPII: true,
		},
		{
			name:      "starts with 9, valid Indian mobile",
			content:   "Phone: 9876543210",
			expectPII: true,
		},
	}

	dir := t.TempDir()
	me := NewMarkerEngine(dir, nil)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := "test.txt"
			require.NoError(t, os.WriteFile(filepath.Join(dir, filePath), []byte(tt.content), 0644))

			files := []models.FileNode{{Path: filePath, IsFile: true}}
			markers := me.Run(files, nil, nil)

			found := false
			for _, m := range markers {
				if m.Type == "dpdp_mobile_exposure" {
					found = true
					break
				}
			}

			if tt.expectPII {
				assert.True(t, found, "Expected dpdp_mobile_exposure marker for: "+tt.name)
			} else {
				assert.False(t, found, "Did not expect dpdp_mobile_exposure marker for: "+tt.name)
			}
		})
	}
}
