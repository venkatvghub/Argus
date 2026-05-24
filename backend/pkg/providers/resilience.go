package providers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sony/gobreaker"
)

// Sentinel errors for callers to distinguish failure modes.
var (
	ErrRateLimit   = errors.New("llm rate limit")
	ErrTransient   = errors.New("llm transient error")
	ErrPermanent   = errors.New("llm permanent error")
	ErrCircuitOpen = errors.New("llm circuit breaker open")
)

// RetryConfig holds exponential-backoff and circuit-breaker settings.
type RetryConfig struct {
	MaxRetries         uint          // max attempts (0 = no retry)
	InitialInterval    time.Duration // first backoff wait
	MaxInterval        time.Duration // cap on backoff wait
	Multiplier         float64       // back-off growth factor (e.g. 2.0)
	FailureThreshold   uint32        // CB trips after this many consecutive failures
	ResetTimeout       time.Duration // CB waits this long before half-open probe
}

// retryingProvider wraps a Provider with exponential back-off and a circuit breaker.
type retryingProvider struct {
	inner Provider
	cfg   RetryConfig
	cb    *gobreaker.CircuitBreaker
}

// newRetryingProvider creates a retryingProvider. The circuit breaker is shared
// across all calls on this instance, so per-tier isolation requires one instance
// per tier (which NewTieredRouter guarantees).
func newRetryingProvider(inner Provider, cfg RetryConfig) *retryingProvider {
	cbSettings := gobreaker.Settings{
		Name:        inner.Name(),
		MaxRequests: 1, // one probe in half-open state
		Interval:    0, // never reset counts in closed state automatically
		Timeout:     cfg.ResetTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.FailureThreshold
		},
	}
	return &retryingProvider{
		inner: inner,
		cfg:   cfg,
		cb:    gobreaker.NewCircuitBreaker(cbSettings),
	}
}

// Name delegates to the inner provider.
func (r *retryingProvider) Name() string { return r.inner.Name() }

// Chat calls inner.Chat with retry + circuit breaker.
func (r *retryingProvider) Chat(ctx context.Context, prompt string) (string, error) {
	var result string

	attempt := func() error {
		out, err := r.inner.Chat(ctx, prompt)
		if err == nil {
			result = out
			return nil
		}
		return classifyError(err)
	}

	err := r.callWithCB(ctx, attempt)
	return result, err
}

// ChatStream delegates directly — streaming is not retried (partial output).
func (r *retryingProvider) ChatStream(ctx context.Context, repoID, prompt string) (<-chan string, <-chan error, error) {
	return r.inner.ChatStream(ctx, repoID, prompt)
}

// callWithCB wraps fn in the circuit breaker then the retry loop.
// Permanent errors (bad API key, 400/401/403) are returned directly without
// incrementing the CB's failure counter — they indicate caller misconfiguration,
// not provider instability.
func (r *retryingProvider) callWithCB(ctx context.Context, fn func() error) error {
	var permErr error
	_, err := r.cb.Execute(func() (interface{}, error) {
		retryErr := r.callWithRetry(ctx, fn)
		if errors.Is(retryErr, ErrPermanent) {
			// Signal success to gobreaker so it doesn't count this as a CB failure,
			// then surface the error after Execute returns.
			permErr = retryErr
			return nil, nil
		}
		return nil, retryErr
	})
	if permErr != nil {
		return permErr
	}
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return ErrCircuitOpen
		}
		return err
	}
	return nil
}

// callWithRetry runs fn with exponential backoff, stopping on permanent errors or context cancel.
func (r *retryingProvider) callWithRetry(ctx context.Context, fn func() error) error {
	if r.cfg.MaxRetries == 0 {
		return fn()
	}

	eb := &backoff.ExponentialBackOff{
		InitialInterval:     r.cfg.InitialInterval,
		RandomizationFactor: 0.25,
		Multiplier:          r.cfg.Multiplier,
		MaxInterval:         r.cfg.MaxInterval,
		MaxElapsedTime:      0, // rely on MaxRetries + context
		Clock:               backoff.SystemClock,
	}
	eb.Reset()

	bo := backoff.WithContext(
		backoff.WithMaxRetries(eb, uint64(r.cfg.MaxRetries)),
		ctx,
	)

	var lastErr error
	operation := func() error {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		// Do not retry permanent errors.
		if errors.Is(err, ErrPermanent) {
			return backoff.Permanent(err)
		}
		return err
	}

	if err := backoff.Retry(operation, bo); err != nil {
		if lastErr != nil {
			return lastErr
		}
		return err
	}
	return nil
}

// classifyError maps a provider error string to a typed sentinel.
// Provider errors are string-wrapped: "openai chat: status 429: ..." or "openai chat: http: ...".
func classifyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	// Extract HTTP status if present: "... status NNN: ..."
	status := extractHTTPStatus(msg)
	switch {
	case status == 429:
		return fmt.Errorf("%w: %s", ErrRateLimit, msg)
	case status >= 500:
		return fmt.Errorf("%w: %s", ErrTransient, msg)
	case status == 401 || status == 403 || status == 400:
		return fmt.Errorf("%w: %s", ErrPermanent, msg)
	case status > 0:
		// Other 4xx → permanent (don't retry unknown client errors)
		return fmt.Errorf("%w: %s", ErrPermanent, msg)
	}

	// Network / connection errors (no HTTP status code)
	if strings.Contains(msg, ": http: ") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, ": EOF") ||
		strings.Contains(msg, "unexpected EOF") {
		return fmt.Errorf("%w: %s", ErrTransient, msg)
	}

	// Unknown — treat as transient to allow retry
	return fmt.Errorf("%w: %s", ErrTransient, msg)
}

// extractHTTPStatus parses the first "status NNN" found in an error string.
// Returns 0 if no status code is found.
func extractHTTPStatus(msg string) int {
	const marker = "status "
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return 0
	}
	rest := msg[idx+len(marker):]
	// Take digits up to the next non-digit character
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return n
}
