package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateTokenCount_emptyText(t *testing.T) {
	assert.Equal(t, 0, EstimateTokenCount("gpt-4", ""))
}

func TestEstimateTokenCount_modelAware(t *testing.T) {
	assert.Equal(t, 2, EstimateTokenCount("gpt-4", "hello world"))
}

func TestEstimateTokenCount_heuristicFallback(t *testing.T) {
	text := "1234567890123456" // 16 bytes → heuristic 4
	assert.Equal(t, 4, heuristicTokenCount(text))
}
