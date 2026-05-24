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
	DBPath  string `envconfig:"DB_PATH" default:"argus.db"`
	DataDir string `envconfig:"DATA_DIR" default:"data"`
	DocsDir string `envconfig:"DOCS_DIR" default:"docs"`

	// Compliance patterns (comma-separated identifiers: AADHAAR, PAN, UPI_ID, MOBILE, EMAIL)
	PIIPatterns []string `envconfig:"PII_PATTERNS" default:"AADHAAR,PAN,UPI_ID,MOBILE,EMAIL"`
	// TokenBloatThreshold is the max tokens-per-line before flagging token_bloat markers.
	TokenBloatThreshold float64 `envconfig:"TOKEN_BLOAT_THRESHOLD" default:"50"`

	// JobManager worker pool sizing
	WorkerCount       int `envconfig:"WORKER_COUNT" default:"3"`
	WorkQueueSize     int `envconfig:"WORK_QUEUE_SIZE" default:"32"`
	JobListenerBuffer int `envconfig:"JOB_LISTENER_BUFFER" default:"10"`

	// LLM Configuration
	LLMProvider  string `envconfig:"LLM_PROVIDER" default:"openai"`
	OpenAIKey    string `envconfig:"OPENAI_API_KEY"`
	AnthropicKey string `envconfig:"ANTHROPIC_API_KEY"`
	GeminiKey    string `envconfig:"GEMINI_API_KEY"`

	// Model Names
	OpenAIModel    string `envconfig:"OPENAI_MODEL" default:"gpt-4o-mini"`
	AnthropicModel string `envconfig:"ANTHROPIC_MODEL" default:"claude-3-5-haiku-20241022"`
	GeminiModel    string `envconfig:"GEMINI_MODEL" default:"gemini-2.0-flash"`

	// MockStreamTokenDelayMS is the per-token delay for stub LLM streaming implementations.
	MockStreamTokenDelayMS int `envconfig:"MOCK_STREAM_TOKEN_DELAY_MS" default:"50"`

	// CORSAllowedOrigins lists origins permitted for browser SSE access (comma-separated).
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS"`

	// CoverageFile is the path to the coverage report file (lcov.info, coverage.xml, or clover.xml).
	// If empty, Argus auto-discovers coverage files in the repository root.
	CoverageFile string `envconfig:"COVERAGE_FILE"`
}

var (
	once    sync.Once
	cfg     *Config
	initErr error // persists across calls; once.Do local err would be lost after first call
)

// Load returns the singleton configuration object. It initializes the config
// on the first call by reading environment variables. Any initialization error
// is preserved and returned on all subsequent calls.
func Load() (*Config, error) {
	once.Do(func() {
		cfg = &Config{}
		initErr = envconfig.Process("ARGUS", cfg)
	})
	if initErr != nil {
		return nil, fmt.Errorf("failed to process config: %w", initErr)
	}
	return cfg, nil
}
