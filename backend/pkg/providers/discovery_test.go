package providers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/providers"
)

// --- classifyTier ---

func TestClassifyTier_Premium(t *testing.T) {
	cases := []string{
		"claude-opus-4",
		"openai/gpt-4-turbo",
		"openai/gpt-4-0125",
		"google/gemini-ultra",
		"meta/llama-large",
	}
	for _, m := range cases {
		if got := providers.ClassifyTier(m); got != "premium" {
			t.Errorf("ClassifyTier(%q) = %q, want premium", m, got)
		}
	}
}

func TestClassifyTier_Medium(t *testing.T) {
	cases := []string{
		"openai/gpt-4o",
		"claude-sonnet-4-6",
		"google/gemini-pro",
		"mistralai/mixtral-8x7b",
		"meta-llama/llama-3-70b",
	}
	for _, m := range cases {
		if got := providers.ClassifyTier(m); got != "medium" {
			t.Errorf("ClassifyTier(%q) = %q, want medium", m, got)
		}
	}
}

func TestClassifyTier_Cheap(t *testing.T) {
	cases := []string{
		// Note: gpt-4o-mini is "medium" because "gpt-4o" matches medium before "mini" matches cheap.
		// Pure cheap identifiers that don't overlap with medium keywords:
		"claude-3-5-haiku",
		"google/gemini-2.0-flash",
		"mistralai/mistral-7b-instruct",
		"meta-llama/llama-3-8b-instruct",
		"some-small-model",
		"some-lite-model",
		"gemini-nano",
	}
	for _, m := range cases {
		if got := providers.ClassifyTier(m); got != "cheap" {
			t.Errorf("ClassifyTier(%q) = %q, want cheap", m, got)
		}
	}
}

func TestClassifyTier_Unknown(t *testing.T) {
	cases := []string{
		"totally-unknown-model",
		"custom-v1",
		"",
	}
	for _, m := range cases {
		if got := providers.ClassifyTier(m); got != "" {
			t.Errorf("ClassifyTier(%q) = %q, want empty (unknown)", m, got)
		}
	}
}

func TestClassifyTier_PremiumPriority(t *testing.T) {
	// A model matching both "opus" (premium) and "pro" (medium) should be premium.
	m := "claude-opus-pro"
	if got := providers.ClassifyTier(m); got != "premium" {
		t.Errorf("ClassifyTier(%q) = %q, want premium (premium takes priority)", m, got)
	}
}

// --- BucketByTier ---

func TestBucketByTier_Groups(t *testing.T) {
	models := []string{
		"claude-3-5-haiku",  // cheap (no overlap with medium)
		"gemini-flash",      // cheap
		"gpt-4o",            // medium
		"claude-sonnet-4-6", // medium
		"claude-opus-4",     // premium
		"totally-unknown",   // excluded
	}
	buckets := providers.BucketByTier(models)

	if len(buckets["cheap"]) != 2 {
		t.Errorf("cheap bucket: got %d, want 2: %v", len(buckets["cheap"]), buckets["cheap"])
	}
	if len(buckets["medium"]) != 2 {
		t.Errorf("medium bucket: got %d, want 2: %v", len(buckets["medium"]), buckets["medium"])
	}
	if len(buckets["premium"]) != 1 {
		t.Errorf("premium bucket: got %d, want 1: %v", len(buckets["premium"]), buckets["premium"])
	}
}

func TestBucketByTier_AlwaysHasAllKeys(t *testing.T) {
	// Even with empty input, all three tier keys must be present.
	buckets := providers.BucketByTier(nil)
	for _, tier := range []string{"cheap", "medium", "premium"} {
		if _, ok := buckets[tier]; !ok {
			t.Errorf("BucketByTier(nil) missing key %q", tier)
		}
	}
}

func TestBucketByTier_Sorted(t *testing.T) {
	models := []string{"z-cheap-nano", "a-cheap-mini", "m-medium-pro"}
	buckets := providers.BucketByTier(models)
	cheap := buckets["cheap"]
	if len(cheap) == 2 && cheap[0] > cheap[1] {
		t.Errorf("cheap bucket not sorted: %v", cheap)
	}
}

func TestBucketByTier_ExcludesUnknown(t *testing.T) {
	buckets := providers.BucketByTier([]string{"mystery-model-x", "another-custom"})
	total := len(buckets["cheap"]) + len(buckets["medium"]) + len(buckets["premium"])
	if total != 0 {
		t.Errorf("unknown models should be excluded, got %d total", total)
	}
}

// --- DiscoverModels (OpenRouter) via mock HTTP server ---

func newOpenRouterTestServer(t *testing.T, responseBody interface{}, statusCode int) *httptest.Server {
	t.Helper()
	body, _ := json.Marshal(responseBody)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		w.Write(body) //nolint:errcheck
	}))
}

func TestDiscoverOpenRouterModels_ParsesPricing(t *testing.T) {
	srv := newOpenRouterTestServer(t, map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"id":   "openai/gpt-4o-mini",
				"name": "GPT-4o Mini",
				"pricing": map[string]string{
					"prompt":     "0.00000015", // $0.15 per 1M
					"completion": "0.00000060", // $0.60 per 1M
				},
			},
			{
				"id":   "anthropic/claude-opus-4",
				"name": "Claude Opus 4",
				"pricing": map[string]string{
					"prompt":     "0.000015", // $15 per 1M
					"completion": "0.000075", // $75 per 1M
				},
			},
			{
				"id":   "free/model",
				"name": "Free Model",
				"pricing": map[string]string{
					"prompt":     "0",
					"completion": "0",
				},
			},
		},
	}, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{
		OpenAIKey:     "test-key",
		OpenAIBaseURL: "https://openrouter.ai/api/v1",
	}

	models, pricing, err := providers.DiscoverOpenRouterModelsFromURL(context.Background(), cfg, srv.URL+"/models")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// free/model should be excluded
	if len(models) != 2 {
		t.Errorf("expected 2 models (free excluded), got %d: %v", len(models), models)
	}

	// Verify pricing conversion: per-token × 1M
	inp, out := pricing["openai/gpt-4o-mini"][0], pricing["openai/gpt-4o-mini"][1]
	if inp < 0.14 || inp > 0.16 {
		t.Errorf("gpt-4o-mini input pricing: got %.4f, want ~0.15", inp)
	}
	if out < 0.59 || out > 0.61 {
		t.Errorf("gpt-4o-mini output pricing: got %.4f, want ~0.60", out)
	}

	inp2, out2 := pricing["anthropic/claude-opus-4"][0], pricing["anthropic/claude-opus-4"][1]
	if inp2 < 14.9 || inp2 > 15.1 {
		t.Errorf("claude-opus-4 input pricing: got %.2f, want ~15.0", inp2)
	}
	if out2 < 74.9 || out2 > 75.1 {
		t.Errorf("claude-opus-4 output pricing: got %.2f, want ~75.0", out2)
	}
}

func TestDiscoverOpenRouterModels_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid key"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	cfg := &config.Config{
		OpenAIKey:     "bad-key",
		OpenAIBaseURL: "https://openrouter.ai/api/v1",
	}
	_, _, err := providers.DiscoverOpenRouterModelsFromURL(context.Background(), cfg, srv.URL+"/models")
	if err == nil {
		t.Error("expected error on non-200 status, got nil")
	}
}

func TestDiscoverOpenRouterModels_EmptyList(t *testing.T) {
	srv := newOpenRouterTestServer(t, map[string]interface{}{"data": []interface{}{}}, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{
		OpenAIKey:     "test-key",
		OpenAIBaseURL: "https://openrouter.ai/api/v1",
	}
	models, pricing, err := providers.DiscoverOpenRouterModelsFromURL(context.Background(), cfg, srv.URL+"/models")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
	if len(pricing) != 0 {
		t.Errorf("expected empty pricing map, got %d entries", len(pricing))
	}
}

func TestDiscoverOpenRouterModels_Sorted(t *testing.T) {
	srv := newOpenRouterTestServer(t, map[string]interface{}{
		"data": []map[string]interface{}{
			{"id": "z-model", "pricing": map[string]string{"prompt": "0.000001", "completion": "0.000001"}},
			{"id": "a-model", "pricing": map[string]string{"prompt": "0.000001", "completion": "0.000001"}},
			{"id": "m-model", "pricing": map[string]string{"prompt": "0.000001", "completion": "0.000001"}},
		},
	}, http.StatusOK)
	defer srv.Close()

	cfg := &config.Config{OpenAIKey: "test-key", OpenAIBaseURL: "https://openrouter.ai/api/v1"}
	models, _, err := providers.DiscoverOpenRouterModelsFromURL(context.Background(), cfg, srv.URL+"/models")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) < 2 {
		t.Skip("need at least 2 models to check sort")
	}
	for i := 1; i < len(models); i++ {
		if models[i] < models[i-1] {
			t.Errorf("models not sorted: %v", models)
			break
		}
	}
}
