package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	err := Init(Config{
		AppName: "test-logger",
		Level:   "debug",
	})
	assert.NoError(t, err)

	l := Get()
	assert.NotNil(t, l)
}
