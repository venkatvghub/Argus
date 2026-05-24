// Package config handles application configuration using environment variables.
// It leverages kelseyhightower/envconfig for structured mapping and supports
// tiered configuration for LLM providers and compliance patterns.
package config

import (
	"fmt"
	"math"
	"sync"
	"time"

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

	// OpenAIBaseURL overrides the default OpenAI endpoint — use for OpenRouter or Azure.
	OpenAIBaseURL string `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`

	// Model Names — empty by default; set via env or selected interactively by argus init.
	OpenAIModel    string `envconfig:"OPENAI_MODEL"`
	AnthropicModel string `envconfig:"ANTHROPIC_MODEL"`
	GeminiModel    string `envconfig:"GEMINI_MODEL"`

	// Wiki Generation Tier Models — override interactive discovery for that tier.
	// If all three are set, argus init skips model discovery entirely.
	CheapModel   string `envconfig:"CHEAP_MODEL"`
	MediumModel  string `envconfig:"MEDIUM_MODEL"`
	PremiumModel string `envconfig:"PREMIUM_MODEL"`

	// MockStreamTokenDelayMS is the per-token delay for stub LLM streaming implementations.
	MockStreamTokenDelayMS int `envconfig:"MOCK_STREAM_TOKEN_DELAY_MS" default:"50"`

	// CORSAllowedOrigins lists origins permitted for browser SSE access (comma-separated).
	CORSAllowedOrigins []string `envconfig:"CORS_ALLOWED_ORIGINS"`

	// CoverageFile is the path to the coverage report file (lcov.info, coverage.xml, or clover.xml).
	// If empty, Argus auto-discovers coverage files in the repository root.
	CoverageFile string `envconfig:"COVERAGE_FILE"`

	// Coverage is the fraction of the repo to generate wiki pages for (0.10–1.0).
	// Used by argus init. Default 0.20 (20%).
	Coverage float64 `envconfig:"COVERAGE" default:"0.20"`

	// RecentAuthorCutoffDays is the lookback window for counting distinct recent authors per file.
	RecentAuthorCutoffDays int `envconfig:"RECENT_AUTHOR_CUTOFF_DAYS" default:"90"`

	// LLM Retry & Circuit Breaker — applied per-tier in the TieredRouter.
	// MaxRetries=0 disables retry (pass-through). CircuitFailureThreshold=0 disables CB.
	LLMHTTPTimeoutS             int     `envconfig:"LLM_HTTP_TIMEOUT_S" default:"120"`
	LLMMaxRetries                 uint    `envconfig:"LLM_MAX_RETRIES" default:"3"`
	LLMRetryInitialDelayMS    int     `envconfig:"LLM_RETRY_INITIAL_DELAY_MS" default:"500"`
	LLMRetryMaxDelayMS        int     `envconfig:"LLM_RETRY_MAX_DELAY_MS" default:"30000"`
	LLMRetryMultiplier        float64 `envconfig:"LLM_RETRY_MULTIPLIER" default:"2.0"`
	LLMCircuitFailureThreshold uint32 `envconfig:"LLM_CIRCUIT_FAILURE_THRESHOLD" default:"5"`
	LLMCircuitResetTimeoutS   int     `envconfig:"LLM_CIRCUIT_RESET_TIMEOUT_S" default:"60"`
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
		if initErr != nil {
			return
		}
		initErr = validateConfig(cfg)
	})
	if initErr != nil {
		return nil, fmt.Errorf("failed to process config: %w", initErr)
	}
	return cfg, nil
}

const defaultLLMHTTPTimeoutS = 120

// LLMHTTPTimeout returns the HTTP client timeout for LLM provider requests.
func (c *Config) LLMHTTPTimeout() time.Duration {
	s := c.LLMHTTPTimeoutS
	if s <= 0 {
		s = defaultLLMHTTPTimeoutS
	}
	return time.Duration(s) * time.Second
}

func validateConfig(cfg *Config) error {
	if cfg.Coverage < 0.10 || cfg.Coverage > 1.0 {
		return fmt.Errorf("COVERAGE must be between 0.10 and 1.0, got %v", cfg.Coverage)
	}
	if cfg.LLMRetryInitialDelayMS < 0 {
		return fmt.Errorf("LLM_RETRY_INITIAL_DELAY_MS must be >= 0, got %d", cfg.LLMRetryInitialDelayMS)
	}
	if cfg.LLMRetryMaxDelayMS < cfg.LLMRetryInitialDelayMS {
		return fmt.Errorf("LLM_RETRY_MAX_DELAY_MS must be >= LLM_RETRY_INITIAL_DELAY_MS (%d), got %d",
			cfg.LLMRetryInitialDelayMS, cfg.LLMRetryMaxDelayMS)
	}
	if math.IsNaN(cfg.LLMRetryMultiplier) || math.IsInf(cfg.LLMRetryMultiplier, 0) {
		return fmt.Errorf("LLM_RETRY_MULTIPLIER must be finite, got %v", cfg.LLMRetryMultiplier)
	}
	if cfg.LLMRetryMultiplier <= 1.0 {
		return fmt.Errorf("LLM_RETRY_MULTIPLIER must be > 1.0, got %v", cfg.LLMRetryMultiplier)
	}
	if cfg.LLMCircuitResetTimeoutS < 0 {
		return fmt.Errorf("LLM_CIRCUIT_RESET_TIMEOUT_S must be >= 0, got %d", cfg.LLMCircuitResetTimeoutS)
	}
	if cfg.LLMHTTPTimeoutS < 0 {
		return fmt.Errorf("LLM_HTTP_TIMEOUT_S must be >= 0, got %d", cfg.LLMHTTPTimeoutS)
	}
	if cfg.RecentAuthorCutoffDays <= 0 {
		return fmt.Errorf("ARGUS_RECENT_AUTHOR_CUTOFF_DAYS must be > 0, got %d", cfg.RecentAuthorCutoffDays)
	}
	return nil
}
