package providers

import (
	"net/http"

	"github.com/venkatvghub/argus/pkg/config"
)

func llmHTTPClient(cfg *config.Config) *http.Client {
	return &http.Client{Timeout: cfg.LLMHTTPTimeout()}
}
