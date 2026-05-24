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

	t.Setenv("ARGUS_APP_NAME", "test-argus")
	t.Setenv("ARGUS_DATA_DIR", "/tmp/argus-data")
	t.Setenv("ARGUS_PII_PATTERNS", "AADHAAR,PAN")

	c, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "test-argus", c.AppName)
	assert.Equal(t, "/tmp/argus-data", c.DataDir)
	assert.Contains(t, c.PIIPatterns, "AADHAAR")
	assert.Contains(t, c.PIIPatterns, "PAN")
}

func TestLoad_CoverageValidation(t *testing.T) {
	tests := []struct {
		name     string
		coverage string
		setEnv   bool
		wantErr  bool
	}{
		{name: "default", setEnv: false, wantErr: false},
		{name: "min boundary", coverage: "0.10", setEnv: true, wantErr: false},
		{name: "max boundary", coverage: "1.0", setEnv: true, wantErr: false},
		{name: "below min", coverage: "0.05", setEnv: true, wantErr: true},
		{name: "above max", coverage: "1.5", setEnv: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			once = sync.Once{}
			cfg = nil
			initErr = nil

			if tt.setEnv {
				t.Setenv("ARGUS_COVERAGE", tt.coverage)
			}

			c, err := Load()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "COVERAGE must be between 0.10 and 1.0")
				assert.Nil(t, c)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, c)
			if !tt.setEnv {
				assert.Equal(t, 0.20, c.Coverage)
			}
		})
	}
}

func TestLoad_LLMResilienceValidation(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "default",
			env:  nil,
		},
		{
			name:    "multiplier too low",
			env:     map[string]string{"ARGUS_LLM_RETRY_MULTIPLIER": "1.0"},
			wantErr: "LLM_RETRY_MULTIPLIER must be > 1.0",
		},
		{
			name:    "multiplier not finite",
			env:     map[string]string{"ARGUS_LLM_RETRY_MULTIPLIER": "NaN"},
			wantErr: "LLM_RETRY_MULTIPLIER must be finite",
		},
		{
			name:    "negative initial delay",
			env:     map[string]string{"ARGUS_LLM_RETRY_INITIAL_DELAY_MS": "-1"},
			wantErr: "LLM_RETRY_INITIAL_DELAY_MS must be >= 0",
		},
		{
			name: "max delay below initial",
			env: map[string]string{
				"ARGUS_LLM_RETRY_INITIAL_DELAY_MS": "5000",
				"ARGUS_LLM_RETRY_MAX_DELAY_MS":     "1000",
			},
			wantErr: "LLM_RETRY_MAX_DELAY_MS must be >=",
		},
		{
			name:    "negative circuit reset timeout",
			env:     map[string]string{"ARGUS_LLM_CIRCUIT_RESET_TIMEOUT_S": "-1"},
			wantErr: "LLM_CIRCUIT_RESET_TIMEOUT_S must be >= 0",
		},
		{
			name: "circuit failure threshold zero allowed",
			env:  map[string]string{"ARGUS_LLM_CIRCUIT_FAILURE_THRESHOLD": "0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			once = sync.Once{}
			cfg = nil
			initErr = nil

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			c, err := Load()
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, c)
				return
			}
			assert.NoError(t, err)
			assert.NotNil(t, c)
		})
	}
}
