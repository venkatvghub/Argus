package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	os.Setenv("REPOWISE_APP_NAME", "test-argus")
	os.Setenv("REPOWISE_DATA_DIR", "/tmp/argus-data")
	os.Setenv("REPOWISE_PII_PATTERNS", "aadhaar,pan")

	cfg, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "test-argus", cfg.AppName)
	assert.Equal(t, "/tmp/argus-data", cfg.DataDir)
	assert.Contains(t, cfg.PIIPatterns, "aadhaar")
	assert.Contains(t, cfg.PIIPatterns, "pan")
}
