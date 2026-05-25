package providers

import (
	"github.com/pkoukk/tiktoken-go"
)

const defaultTokenEncoding = "cl100k_base"

// EstimateTokenCount counts tokens for text using a model-aware tokenizer.
// Falls back to len(text)/4 when tokenizer setup fails.
func EstimateTokenCount(model, text string) int {
	if text == "" {
		return 0
	}
	tkm, err := tiktoken.EncodingForModel(model)
	if err != nil {
		tkm, err = tiktoken.GetEncoding(defaultTokenEncoding)
	}
	if err != nil {
		return heuristicTokenCount(text)
	}
	return len(tkm.Encode(text, nil, nil))
}

func heuristicTokenCount(text string) int {
	return len(text) / 4
}
