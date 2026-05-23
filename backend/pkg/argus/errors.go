package argus

import "errors"

// ErrRepoNotFound is returned when a repository ID is not indexed in memory.
var ErrRepoNotFound = errors.New("repo not found")
