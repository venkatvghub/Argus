// Package config handles application configuration using environment variables.
// It leverages kelseyhightower/envconfig for structured mapping and supports
// tiered configuration for LLM providers and compliance patterns.
package config

import (
	"fmt"
	"sync"

	"github.com/kelseyhightower/envconfig"
)

// Config represents the application's global configuration.
type Config struct {
	// AppName is the identifier for the service.
	AppName string `envconfig:"APP_NAME" default:"argus"`
	// Environment (development, production, etc.)
	Env string `envconfig:"ENV" default:"development"`
	// LogLevel for structured logging.
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// Persistence configuration
	DBPath  string `envconfig:"DB_PATH" default:"data/argus.db"`
	DataDir string `envconfig:"DATA_DIR" default:"data"`
	DocsDir string `envconfig:"DOCS_DIR" default:"docs"`

	// Compliance patterns (comma-separated regex or identifiers)
	PIIPatterns []string `envconfig:"PII_PATTERNS" default:"AADHAAR,PAN,UPI_ID"`

	// LLM Configuration
	LLMProvider  string `envconfig:"LLM_PROVIDER" default:"openai"`
	OpenAIKey    string `envconfig:"OPENAI_API_KEY"`
	AnthropicKey string `envconfig:"ANTHROPIC_API_KEY"`
	GeminiKey    string `envconfig:"GEMINI_API_KEY"`

	// Model Names
	OpenAIModel    string `envconfig:"OPENAI_MODEL" default:"gpt-4-turbo"`
	AnthropicModel string `envconfig:"ANTHROPIC_MODEL" default:"claude-3-opus-20240229"`
	GeminiModel    string `envconfig:"GEMINI_MODEL" default:"gemini-1.5-flash"`
}

var (
	once sync.Once
	cfg  *Config
)

// Load returns the singleton configuration object. It initializes the config
// on the first call by reading environment variables.
func Load() (*Config, error) {
	var err error
	once.Do(func() {
		cfg = &Config{}
		err = envconfig.Process("REPOWISE", cfg)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to process config: %w", err)
	}
	return cfg, nil
}
