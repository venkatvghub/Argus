package providers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// stubProvider counts Chat calls and returns configurable error/response sequences.
type stubProvider struct {
	name    string
	calls   int
	results []error // nil = success, non-nil = error
	reply   string
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Chat(_ context.Context, _ string) (string, error) {
	idx := s.calls
	s.calls++
	if idx < len(s.results) {
		return s.reply, s.results[idx]
	}
	return s.reply, nil
}
func (s *stubProvider) ChatStream(_ context.Context, _ string, _ string) (<-chan string, <-chan error, error) {
	return nil, nil, errors.New("not implemented")
}

func fastRetryCfg(maxRetries uint) RetryConfig {
	return RetryConfig{
		MaxRetries:       maxRetries,
		InitialInterval:  1 * time.Millisecond,
		MaxInterval:      5 * time.Millisecond,
		Multiplier:       2.0,
		FailureThreshold: 5,
		ResetTimeout:     100 * time.Millisecond,
	}
}

// --- classifyError ---

func TestClassifyError_Nil(t *testing.T) {
	if err := classifyError(nil); err != nil {
		t.Errorf("classifyError(nil) = %v, want nil", err)
	}
}

func TestClassifyError_RateLimit(t *testing.T) {
	err := classifyError(fmt.Errorf("openai chat: status 429: rate limited"))
	if !errors.Is(err, ErrRateLimit) {
		t.Errorf("status 429: got %v, want ErrRateLimit", err)
	}
}

func TestClassifyError_Transient5xx(t *testing.T) {
	for _, code := range []int{500, 502, 503} {
		err := classifyError(fmt.Errorf("openai chat: status %d: server error", code))
		if !errors.Is(err, ErrTransient) {
			t.Errorf("status %d: got %v, want ErrTransient", code, err)
		}
	}
}

func TestClassifyError_Permanent4xx(t *testing.T) {
	for _, code := range []int{400, 401, 403} {
		err := classifyError(fmt.Errorf("openai chat: status %d: bad", code))
		if !errors.Is(err, ErrPermanent) {
			t.Errorf("status %d: got %v, want ErrPermanent", code, err)
		}
	}
}

func TestClassifyError_NetworkTransient(t *testing.T) {
	cases := []string{
		"openai chat: http: dial tcp: connection refused",
		"openai chat: http: read: i/o timeout",
		"openai chat: http: EOF",
	}
	for _, msg := range cases {
		err := classifyError(errors.New(msg))
		if !errors.Is(err, ErrTransient) {
			t.Errorf("%q: got %v, want ErrTransient", msg, err)
		}
	}
}

func TestClassifyError_UnknownTreatedAsTransient(t *testing.T) {
	err := classifyError(errors.New("some unknown provider error"))
	if !errors.Is(err, ErrTransient) {
		t.Errorf("unknown error: got %v, want ErrTransient", err)
	}
}

// --- extractHTTPStatus ---

func TestExtractHTTPStatus(t *testing.T) {
	cases := []struct {
		msg  string
		want int
	}{
		{"openai chat: status 429: too many requests", 429},
		{"anthropic chat: status 500: internal error", 500},
		{"no status here", 0},
		{"status ", 0}, // no digits
		{"prefix status 200 suffix", 200},
	}
	for _, tc := range cases {
		got := extractHTTPStatus(tc.msg)
		if got != tc.want {
			t.Errorf("extractHTTPStatus(%q) = %d, want %d", tc.msg, got, tc.want)
		}
	}
}

// --- retryingProvider.Chat ---

func TestRetryingProvider_SuccessOnFirstAttempt(t *testing.T) {
	stub := &stubProvider{name: "stub", reply: "ok"}
	p := newRetryingProvider(stub, fastRetryCfg(3))

	out, err := p.Chat(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Errorf("reply = %q, want ok", out)
	}
	if stub.calls != 1 {
		t.Errorf("calls = %d, want 1", stub.calls)
	}
}

func TestRetryingProvider_RetriesTransientThenSucceeds(t *testing.T) {
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		reply:   "ok",
		results: []error{transient, transient, nil},
	}
	p := newRetryingProvider(stub, fastRetryCfg(3))

	out, err := p.Chat(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}
	if out != "ok" {
		t.Errorf("reply = %q, want ok", out)
	}
	if stub.calls != 3 {
		t.Errorf("calls = %d, want 3 (2 failures + 1 success)", stub.calls)
	}
}

func TestRetryingProvider_NoRetryOnPermanentError(t *testing.T) {
	permanent := fmt.Errorf("openai chat: status 401: unauthorized")
	stub := &stubProvider{
		name:    "stub",
		results: []error{permanent},
	}
	p := newRetryingProvider(stub, fastRetryCfg(5))

	_, err := p.Chat(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if stub.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 401)", stub.calls)
	}
}

func TestRetryingProvider_ExhaustsMaxRetries(t *testing.T) {
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		results: []error{transient, transient, transient, transient, transient},
	}
	p := newRetryingProvider(stub, fastRetryCfg(2)) // max 2 retries = 3 total attempts

	_, err := p.Chat(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if stub.calls != 3 {
		t.Errorf("calls = %d, want 3 (1 initial + 2 retries)", stub.calls)
	}
}

func TestRetryingProvider_ContextCancelStopsRetry(t *testing.T) {
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		results: []error{transient, transient, transient, transient, transient},
	}
	p := newRetryingProvider(stub, fastRetryCfg(10))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Chat(ctx, "hello")
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
	// Fewer attempts than MaxRetries because context fired first
	if stub.calls > 10 {
		t.Errorf("too many calls (%d) before context cancel", stub.calls)
	}
}

func TestRetryingProvider_ZeroRetriesNoRetry(t *testing.T) {
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		results: []error{transient},
	}
	p := newRetryingProvider(stub, fastRetryCfg(0)) // disabled

	_, err := p.Chat(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if stub.calls != 1 {
		t.Errorf("calls = %d, want 1 (retry disabled)", stub.calls)
	}
}

// --- circuit breaker ---

func TestRetryingProvider_CircuitBreakerTrips(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:       0, // disable retry so CB counts raw failures
		InitialInterval:  1 * time.Millisecond,
		MaxInterval:      5 * time.Millisecond,
		Multiplier:       2.0,
		FailureThreshold: 3,
		ResetTimeout:     200 * time.Millisecond,
	}
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		results: []error{transient, transient, transient, transient, transient},
	}
	p := newRetryingProvider(stub, cfg)

	// Trip the breaker (3 consecutive failures)
	for i := 0; i < 3; i++ {
		p.Chat(context.Background(), "hello") //nolint:errcheck
	}

	// Next call should return ErrCircuitOpen without hitting the stub
	callsBefore := stub.calls
	_, err := p.Chat(context.Background(), "hello")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("after %d failures: got %v, want ErrCircuitOpen", cfg.FailureThreshold, err)
	}
	if stub.calls != callsBefore {
		t.Errorf("stub called after CB open (calls: %d → %d)", callsBefore, stub.calls)
	}
}

func TestRetryingProvider_CircuitBreakerResets(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:       0,
		InitialInterval:  1 * time.Millisecond,
		MaxInterval:      5 * time.Millisecond,
		Multiplier:       2.0,
		FailureThreshold: 2,
		ResetTimeout:     50 * time.Millisecond,
	}
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		reply:   "recovered",
		results: []error{transient, transient}, // 2 failures to trip CB
	}
	p := newRetryingProvider(stub, cfg)

	// Trip the breaker
	p.Chat(context.Background(), "hello") //nolint:errcheck
	p.Chat(context.Background(), "hello") //nolint:errcheck

	// Wait for reset timeout → half-open → success should close the CB
	time.Sleep(cfg.ResetTimeout + 10*time.Millisecond)

	out, err := p.Chat(context.Background(), "hello")
	if err != nil {
		t.Errorf("after CB reset: unexpected error: %v", err)
	}
	if out != "recovered" {
		t.Errorf("reply = %q, want recovered", out)
	}
}

// --- Name passthrough ---

func TestRetryingProvider_Name(t *testing.T) {
	stub := &stubProvider{name: "my-provider"}
	p := newRetryingProvider(stub, fastRetryCfg(1))
	if p.Name() != "my-provider" {
		t.Errorf("Name() = %q, want my-provider", p.Name())
	}
}
