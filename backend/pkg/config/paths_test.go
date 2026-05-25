package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveDocsPath(t *testing.T) {
	c := &Config{DocsDir: "docs"}
	assert.Equal(t, filepath.Join("docs", "exports", "cognee.json"), c.ResolveDocsPath("exports", "cognee.json"))

	var nilCfg *Config
	assert.Equal(t, filepath.Join("docs", "exports", "cognee.json"), nilCfg.ResolveDocsPath("exports", "cognee.json"))
}
