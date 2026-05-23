package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveDBPath(t *testing.T) {
	defaultPath := filepath.Join("data", "argus.db")

	var nilCfg *Config
	assert.Equal(t, defaultPath, nilCfg.ResolveDBPath())
	assert.Equal(t, defaultPath, (&Config{}).ResolveDBPath())

	c := &Config{DataDir: "data", DBPath: "argus.db"}
	assert.Equal(t, defaultPath, c.ResolveDBPath())

	abs := filepath.Join(t.TempDir(), "custom.db")
	c2 := &Config{DataDir: "data", DBPath: abs}
	assert.Equal(t, abs, c2.ResolveDBPath())
}

func TestResolveDocsPath(t *testing.T) {
	c := &Config{DocsDir: "docs"}
	assert.Equal(t, filepath.Join("docs", "exports", "cognee.json"), c.ResolveDocsPath("exports", "cognee.json"))
}
