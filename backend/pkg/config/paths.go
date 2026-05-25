package config

import "path/filepath"

// ResolveDocsPath joins path segments under DocsDir.
func (c *Config) ResolveDocsPath(parts ...string) string {
	dir := "docs"
	if c != nil && c.DocsDir != "" {
		dir = c.DocsDir
	}
	return filepath.Join(append([]string{dir}, parts...)...)
}
