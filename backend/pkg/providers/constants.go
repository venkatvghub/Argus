package providers

import (
	"time"

	"github.com/venkatvghub/argus/pkg/config"
)

const defaultMockStreamTokenDelay = 50 * time.Millisecond

func mockStreamDelay(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.MockStreamTokenDelayMS > 0 {
		return time.Duration(cfg.MockStreamTokenDelayMS) * time.Millisecond
	}
	return defaultMockStreamTokenDelay
}
