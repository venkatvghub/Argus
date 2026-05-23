// Package constants holds cross-package identifiers and version strings.
package constants

const (
	// APIVersion is the REST export and MCP server version.
	APIVersion = "1.0.0"
	// AllJobsWildcard matches every job in SSE and JobManager subscriptions.
	AllJobsWildcard = "*"
	// RepoIDLength is the number of hex chars used from sha256(absPath).
	RepoIDLength = 12
)
