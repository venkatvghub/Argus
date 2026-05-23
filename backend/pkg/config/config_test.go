package config

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Reset singleton so Load() runs fresh
	once = sync.Once{}
	cfg = nil
	initErr = nil

	t.Setenv("REPOWISE_APP_NAME", "test-argus")
	t.Setenv("REPOWISE_DATA_DIR", "/tmp/argus-data")
	t.Setenv("REPOWISE_PII_PATTERNS", "aadhaar,pan")

	c, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "test-argus", c.AppName)
	assert.Equal(t, "/tmp/argus-data", c.DataDir)
	assert.Contains(t, c.PIIPatterns, "aadhaar")
	assert.Contains(t, c.PIIPatterns, "pan")
}
