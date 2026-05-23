package config

import "path/filepath"

// ResolveDBPath returns an absolute path for the SQLite database file.
// Relative DBPath values are resolved under DataDir.
func (c *Config) ResolveDBPath() string {
	if c == nil {
		return "data/argus.db"
	}
	if filepath.IsAbs(c.DBPath) {
		return c.DBPath
	}
	return filepath.Join(c.DataDir, c.DBPath)
}

// ResolveDocsPath joins path segments under DocsDir.
func (c *Config) ResolveDocsPath(parts ...string) string {
	if c == nil {
		return filepath.Join(append([]string{"docs"}, parts...)...)
	}
	p := c.DocsDir
	for _, part := range parts {
		p = filepath.Join(p, part)
	}
	return p
}
