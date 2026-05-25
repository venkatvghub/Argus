package argus

import "errors"

// ErrRepoNotFound is returned when a repository ID is not indexed in memory.
var ErrRepoNotFound = errors.New("repo not found")

// ErrConversationNotFound is returned when a conversation ID does not exist.
var ErrConversationNotFound = errors.New("conversation not found")

// ErrJobNotFound is returned when a job ID does not exist.
var ErrJobNotFound = errors.New("job not found")
