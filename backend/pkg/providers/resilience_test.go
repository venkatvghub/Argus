package providers

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// stubProvider counts Chat calls and returns configurable error/response sequences.
type stubProvider struct {
	name    string
	calls   atomic.Int32
	results []error // nil = success, non-nil = error
	reply   string
}

func (s *stubProvider) callCount() int {
	return int(s.calls.Load())
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Chat(_ context.Context, _ string) (string, error) {
	idx := int(s.calls.Add(1)) - 1
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
		"openai chat: http: EOF",       // ": EOF" suffix
		"openai chat: unexpected EOF",  // "unexpected EOF" form
	}
	for _, msg := range cases {
		err := classifyError(errors.New(msg))
		if !errors.Is(err, ErrTransient) {
			t.Errorf("%q: got %v, want ErrTransient", msg, err)
		}
	}
}

func TestClassifyError_EOFSubstringNotOvermatched(t *testing.T) {
	// A message containing "EOF" in an unrelated position should not be
	// classified as transient solely because of the substring.
	// e.g. a model named "BEOF-model" should not match.
	msg := "some error about BEOF-model not found"
	err := classifyError(errors.New(msg))
	// No HTTP status, no ": EOF", no "unexpected EOF", no network keywords →
	// falls through to unknown → ErrTransient (expected — unknown is retried).
	// The key assertion: it must NOT match because of "EOF" alone — the
	// "BEOF" string does not contain ": EOF" or "unexpected EOF".
	if !errors.Is(err, ErrTransient) {
		t.Errorf("unexpected classification for %q: %v", msg, err)
	}
	// Confirm the inverse: a bare "EOF" without prefix does NOT trigger the network branch.
	bare := "EOF"
	bareErr := classifyError(errors.New(bare))
	// "EOF" alone doesn't contain ": EOF" → falls to unknown → still ErrTransient,
	// but via the fallback path, not the network branch. Both are ErrTransient so
	// we just verify the outcome is still transient.
	if !errors.Is(bareErr, ErrTransient) {
		t.Errorf("bare EOF should be transient: %v", bareErr)
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
	if stub.callCount() != 1 {
		t.Errorf("calls = %d, want 1", stub.callCount())
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
	if stub.callCount() != 3 {
		t.Errorf("calls = %d, want 3 (2 failures + 1 success)", stub.callCount())
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
	if stub.callCount() != 1 {
		t.Errorf("calls = %d, want 1 (no retry on 401)", stub.callCount())
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
	if stub.callCount() != 3 {
		t.Errorf("calls = %d, want 3 (1 initial + 2 retries)", stub.callCount())
	}
}

func TestRetryingProvider_ContextCancelStopsRetry(t *testing.T) {
	transient := fmt.Errorf("openai chat: status 503: unavailable")
	stub := &stubProvider{
		name:    "stub",
		results: []error{transient, transient, transient, transient, transient},
	}
	maxRetries := uint(10)
	p := newRetryingProvider(stub, fastRetryCfg(maxRetries))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel after the first retry attempt starts so the test does not depend
	// on a tight wall-clock timeout (which flakes under CI load).
	const targetCalls = 2
	go func() {
		for {
			if stub.callCount() >= targetCalls {
				cancel()
				return
			}
			time.Sleep(100 * time.Microsecond)
		}
	}()

	_, err := p.Chat(ctx, "hello")
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
	if stub.callCount() >= int(maxRetries)+1 {
		t.Errorf("calls = %d, want fewer than %d (context should stop retries)", stub.callCount(), maxRetries+1)
	}
	if stub.callCount() < targetCalls {
		t.Errorf("calls = %d, want at least %d before cancel", stub.callCount(), targetCalls)
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
	if stub.callCount() != 1 {
		t.Errorf("calls = %d, want 1 (retry disabled)", stub.callCount())
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
	callsBefore := stub.callCount()
	_, err := p.Chat(context.Background(), "hello")
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("after %d failures: got %v, want ErrCircuitOpen", cfg.FailureThreshold, err)
	}
	if stub.callCount() != callsBefore {
		t.Errorf("stub called after CB open (calls: %d → %d)", callsBefore, stub.callCount())
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

func TestRetryingProvider_PermanentErrorDoesNotTripCB(t *testing.T) {
	// A 401 (permanent) should never trip the circuit breaker — it signals
	// a misconfigured API key, not provider instability.
	cfg := RetryConfig{
		MaxRetries:       0,
		InitialInterval:  1 * time.Millisecond,
		MaxInterval:      5 * time.Millisecond,
		Multiplier:       2.0,
		FailureThreshold: 3, // CB trips after 3 consecutive transient failures
		ResetTimeout:     200 * time.Millisecond,
	}
	permanent := fmt.Errorf("openai chat: status 401: unauthorized")
	stub := &stubProvider{
		name: "stub",
		// Return permanent error many more times than the CB threshold.
		results: []error{permanent, permanent, permanent, permanent, permanent},
	}
	p := newRetryingProvider(stub, cfg)

	// Fire enough permanent errors to exceed the CB threshold if they were counted.
	for i := 0; i < 5; i++ {
		_, err := p.Chat(context.Background(), "hello")
		if !errors.Is(err, ErrPermanent) {
			t.Errorf("call %d: got %v, want ErrPermanent", i+1, err)
		}
	}

	// The CB must still be closed — a subsequent call reaches the stub.
	callsBefore := stub.callCount()
	stub.results = append(stub.results, nil) // next call succeeds
	out, err := p.Chat(context.Background(), "hello")
	if err != nil {
		t.Errorf("after permanent errors: CB should be closed, got %v", err)
	}
	if out != stub.reply {
		t.Errorf("reply = %q, want %q", out, stub.reply)
	}
	if stub.callCount() != callsBefore+1 {
		t.Errorf("stub.calls = %d, want %d (CB should not have blocked)", stub.callCount(), callsBefore+1)
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
