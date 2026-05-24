package providers

const (
	openAIDefaultBaseURL = "https://api.openai.com/v1"
	anthropicAPIURL      = "https://api.anthropic.com/v1/messages"
	anthropicModelsURL   = "https://api.anthropic.com/v1/models"
	anthropicVersion     = "2023-06-01"
	defaultMaxTokens     = 4096

	openRouterReferer = "https://github.com/venkatvghub/argus"
	openRouterTitle   = "Argus"
)

// Tier names used for generation routing.
const (
	TierCheap   = "cheap"
	TierMedium  = "medium"
	TierPremium = "premium"
)
