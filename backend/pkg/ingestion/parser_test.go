package ingestion

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/models"
)

func TestLanguageRegistry(t *testing.T) {
	r := NewLanguageRegistry()

	tests := []struct {
		path     string
		expected string
	}{
		{"test.go", "go"},
		{"test.js", "javascript"},
		{"test.ts", "typescript"},
		{"test.java", "java"},
		{"test.py", "python"},
		{"test.unknown", ""},
	}

	for _, tt := range tests {
		lang, name := r.GetLanguageForPath(tt.path)
		if tt.expected == "" {
			assert.Nil(t, lang)
			assert.Equal(t, "", name)
		} else {
			assert.NotNil(t, lang, "Language for %s should not be nil", tt.path)
			assert.Equal(t, tt.expected, name)
		}
	}
}

func TestParser_Parse(t *testing.T) {
	p, err := NewTreeSitterParser()
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("Go Parsing", func(t *testing.T) {
		content := []byte("package main\nfunc main() { go func() { x = 1 }() }")
		tree, langName, err := p.Parse(ctx, content, "main.go")
		require.NoError(t, err)
		assert.Equal(t, "go", langName)

		biomarkers, err := p.ExecuteBiomarkers(tree, langName, content)
		require.NoError(t, err)
		assert.Contains(t, getMarkerNames(biomarkers), "unsecured_goroutine")
		assert.Contains(t, getMarkerNames(biomarkers), "goroutine_outer_mutation")

		// Safe version
		safeContent := []byte("package main\nfunc main() { go func(v int) { }(1) }")
		tree, _, _ = p.Parse(ctx, safeContent, "main.go")
		biomarkers, _ = p.ExecuteBiomarkers(tree, langName, safeContent)
		assert.NotContains(t, getMarkerNames(biomarkers), "goroutine_outer_mutation")
	})

	t.Run("Python AppSec", func(t *testing.T) {
		content := []byte("import hashlib\nhashlib.md5()\ncursor.execute('SELECT * FROM users WHERE id = ' + id)\nawait some_func()")
		tree, langName, err := p.Parse(ctx, content, "app.py")
		require.NoError(t, err)
		assert.Equal(t, "python", langName)

		biomarkers, err := p.ExecuteBiomarkers(tree, langName, content)
		require.NoError(t, err)
		names := getMarkerNames(biomarkers)
		assert.Contains(t, names, "broken_crypto")
		assert.Contains(t, names, "tainted_sql")
		assert.Contains(t, names, "logical_race")

		// Safe version
		safeContent := []byte("import hashlib\nhashlib.sha256()\ncursor.execute('SELECT * FROM users WHERE id = %s', (id,))\nsome_func()")
		tree, _, _ = p.Parse(ctx, safeContent, "app.py")
		biomarkers, _ = p.ExecuteBiomarkers(tree, langName, safeContent)
		names = getMarkerNames(biomarkers)
		assert.NotContains(t, names, "broken_crypto")
		assert.NotContains(t, names, "tainted_sql")
	})

	t.Run("Java AppSec", func(t *testing.T) {
		content := []byte("class T { void m() { MessageDigest.getInstance(\"MD5\"); stmt.executeQuery(\"SELECT * FROM \" + table); } }")
		tree, langName, err := p.Parse(ctx, content, "Test.java")
		require.NoError(t, err)
		assert.Equal(t, "java", langName)

		biomarkers, err := p.ExecuteBiomarkers(tree, langName, content)
		require.NoError(t, err)
		names := getMarkerNames(biomarkers)
		assert.Contains(t, names, "broken_crypto")
		assert.Contains(t, names, "tainted_sql")

		// Safe version
		safeContent := []byte("class T { void m() { MessageDigest.getInstance(\"SHA-256\"); stmt.executeQuery(\"SELECT * FROM users\"); } }")
		tree, _, _ = p.Parse(ctx, safeContent, "Test.java")
		biomarkers, _ = p.ExecuteBiomarkers(tree, langName, safeContent)
		names = getMarkerNames(biomarkers)
		assert.NotContains(t, names, "broken_crypto")
		assert.NotContains(t, names, "tainted_sql")
	})

	t.Run("JS AppSec", func(t *testing.T) {
		content := []byte("crypto.createHash('md5'); db.query('SELECT * FROM ' + table);")
		tree, langName, err := p.Parse(ctx, content, "app.js")
		require.NoError(t, err)
		assert.Equal(t, "javascript", langName)

		biomarkers, err := p.ExecuteBiomarkers(tree, langName, content)
		require.NoError(t, err)
		names := getMarkerNames(biomarkers)
		assert.Contains(t, names, "broken_crypto")
		assert.Contains(t, names, "tainted_sql")

		// Safe version
		safeContent := []byte("crypto.createHash('sha256'); db.query('SELECT * FROM users');")
		tree, _, _ = p.Parse(ctx, safeContent, "app.js")
		biomarkers, _ = p.ExecuteBiomarkers(tree, langName, safeContent)
		names = getMarkerNames(biomarkers)
		assert.NotContains(t, names, "broken_crypto")
		assert.NotContains(t, names, "tainted_sql")
	})
}

func getMarkerNames(symbols []models.Symbol) []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	return names
}
