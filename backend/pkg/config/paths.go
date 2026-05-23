package config

import "path/filepath"

// ResolveDBPath returns an absolute path for the SQLite database file.
// Relative DBPath values are resolved under DataDir.
func (c *Config) ResolveDBPath() string {
	dataDir := "data"
	dbPath := "argus.db"
	if c != nil {
		if c.DataDir != "" {
			dataDir = c.DataDir
		}
		if c.DBPath != "" {
			dbPath = c.DBPath
		}
	}
	if filepath.IsAbs(dbPath) {
		return dbPath
	}
	return filepath.Join(dataDir, dbPath)
}

// ResolveDocsPath joins path segments under DocsDir.
func (c *Config) ResolveDocsPath(parts ...string) string {
	dir := "docs"
	if c != nil && c.DocsDir != "" {
		dir = c.DocsDir
	}
	return filepath.Join(append([]string{dir}, parts...)...)
}
