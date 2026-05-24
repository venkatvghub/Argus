package providers_test

import (
	"testing"

	"github.com/venkatvghub/argus/pkg/config"
	"github.com/venkatvghub/argus/pkg/providers"
)

func TestDetectProviders_AllEmpty(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "",
		OpenAIKey:    "",
		GeminiKey:    "",
	}

	statuses := providers.DetectProviders(cfg)

	if len(statuses) != 3 {
		t.Fatalf("expected 3 provider statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Available {
			t.Errorf("provider %s should not be available when key is empty", s.Name)
		}
	}
}

func TestDetectProviders_OneSet(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "sk-ant-test-key",
		OpenAIKey:    "",
		GeminiKey:    "",
	}

	statuses := providers.DetectProviders(cfg)

	if len(statuses) != 3 {
		t.Fatalf("expected 3 provider statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.Name == "anthropic" {
			if !s.Available {
				t.Errorf("anthropic should be available when key is set")
			}
		} else {
			if s.Available {
				t.Errorf("%s should not be available when key is empty", s.Name)
			}
		}
	}
}

func TestDetectProviders_MultipleSet(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "sk-ant-test-key",
		OpenAIKey:    "sk-test-key",
		GeminiKey:    "",
	}

	statuses := providers.DetectProviders(cfg)

	availableCount := 0
	for _, s := range statuses {
		if s.Available {
			availableCount++
		}
	}

	if availableCount != 2 {
		t.Errorf("expected 2 available providers, got %d", availableCount)
	}
}

func TestDetectProviders_DefaultModels(t *testing.T) {
	tests := []struct {
		name            string
		anthropicKey    string
		openaiKey       string
		geminiKey       string
		wantProvider    string
		wantCheapModel  string
		wantMediumModel string
	}{
		{
			name:            "anthropic",
			anthropicKey:    "key",
			openaiKey:       "",
			geminiKey:       "",
			wantProvider:    "anthropic",
			wantCheapModel:  "claude-haiku-4-5-20251001",
			wantMediumModel: "claude-sonnet-4-6",
		},
		{
			name:            "openai",
			anthropicKey:    "",
			openaiKey:       "key",
			geminiKey:       "",
			wantProvider:    "openai",
			wantCheapModel:  "gpt-4o-mini",
			wantMediumModel: "gpt-4o",
		},
		{
			name:            "gemini",
			anthropicKey:    "",
			openaiKey:       "",
			geminiKey:       "key",
			wantProvider:    "gemini",
			wantCheapModel:  "gemini-2.0-flash",
			wantMediumModel: "gemini-1.5-pro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AnthropicKey: tt.anthropicKey,
				OpenAIKey:    tt.openaiKey,
				GeminiKey:    tt.geminiKey,
			}

			statuses := providers.DetectProviders(cfg)

			for _, s := range statuses {
				if s.Name == tt.wantProvider {
					if !s.Available {
						t.Errorf("expected %s to be available", tt.wantProvider)
					}
					if s.DefaultCheap != tt.wantCheapModel {
						t.Errorf("cheap model mismatch: got %s, want %s", s.DefaultCheap, tt.wantCheapModel)
					}
					if s.DefaultMedium != tt.wantMediumModel {
						t.Errorf("medium model mismatch: got %s, want %s", s.DefaultMedium, tt.wantMediumModel)
					}
					return
				}
			}
			t.Errorf("provider %s not found in statuses", tt.wantProvider)
		})
	}
}

func TestDetectProviders_AllSet(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "sk-ant-key",
		OpenAIKey:    "sk-key",
		GeminiKey:    "key",
	}

	statuses := providers.DetectProviders(cfg)

	if len(statuses) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(statuses))
	}

	for _, s := range statuses {
		if !s.Available {
			t.Errorf("provider %s should be available when all keys are set", s.Name)
		}
		if s.DefaultCheap == "" || s.DefaultMedium == "" || s.DefaultPremium == "" {
			t.Errorf("provider %s has missing default models", s.Name)
		}
	}
}

func TestDefaultTieredConfig_NoProviders(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "",
		OpenAIKey:    "",
		GeminiKey:    "",
	}

	_, err := providers.DefaultTieredConfig(cfg)

	if err == nil {
		t.Errorf("expected error when no providers are configured")
	}
}

func TestDefaultTieredConfig_AnthropicOnly(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "sk-ant-test-key",
		OpenAIKey:    "",
		GeminiKey:    "",
	}

	tc, err := providers.DefaultTieredConfig(cfg)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if tc.ProviderName != "anthropic" {
		t.Errorf("provider name mismatch: got %s, want anthropic", tc.ProviderName)
	}

	if tc.CheapModel == "" || tc.MediumModel == "" || tc.PremiumModel == "" {
		t.Errorf("config should have non-empty model names: cheap=%s medium=%s premium=%s",
			tc.CheapModel, tc.MediumModel, tc.PremiumModel)
	}

	if tc.CheapModel != "claude-haiku-4-5-20251001" {
		t.Errorf("cheap model mismatch: got %s", tc.CheapModel)
	}
}

func TestDefaultTieredConfig_OpenAIFirst(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "",
		OpenAIKey:    "sk-test-key",
		GeminiKey:    "gemini-key",
	}

	tc, err := providers.DefaultTieredConfig(cfg)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should pick the first available provider (openai in this case)
	if tc.ProviderName != "openai" {
		t.Errorf("provider name mismatch: got %s, want openai", tc.ProviderName)
	}
}

func TestDefaultTieredConfig_GeminiOnly(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "",
		OpenAIKey:    "",
		GeminiKey:    "gemini-key",
	}

	tc, err := providers.DefaultTieredConfig(cfg)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if tc.ProviderName != "gemini" {
		t.Errorf("provider name mismatch: got %s, want gemini", tc.ProviderName)
	}

	if tc.CheapModel != "gemini-2.0-flash" {
		t.Errorf("cheap model mismatch: got %s", tc.CheapModel)
	}
}

func TestDefaultTieredConfig_TieredConfigFields(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "sk-ant-test-key",
		OpenAIKey:    "",
		GeminiKey:    "",
	}

	tc, err := providers.DefaultTieredConfig(cfg)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify all fields are populated
	if tc.ProviderName == "" {
		t.Errorf("ProviderName is empty")
	}
	if tc.CheapModel == "" {
		t.Errorf("CheapModel is empty")
	}
	if tc.MediumModel == "" {
		t.Errorf("MediumModel is empty")
	}
	if tc.PremiumModel == "" {
		t.Errorf("PremiumModel is empty")
	}
}

func TestDetectProviders_StatusNames(t *testing.T) {
	cfg := &config.Config{
		AnthropicKey: "key",
		OpenAIKey:    "key",
		GeminiKey:    "key",
	}

	statuses := providers.DetectProviders(cfg)

	expectedNames := map[string]bool{"anthropic": true, "openai": true, "gemini": true}
	for _, s := range statuses {
		if !expectedNames[s.Name] {
			t.Errorf("unexpected provider name: %s", s.Name)
		}
		delete(expectedNames, s.Name)
	}

	if len(expectedNames) > 0 {
		t.Errorf("missing provider names: %v", expectedNames)
	}
}
