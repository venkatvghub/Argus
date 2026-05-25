package argus

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/venkatvghub/argus/pkg/config"
)

func testDBURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("ARGUS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ARGUS_TEST_DATABASE_URL not set")
	}
	return dsn
}

func TestNew(t *testing.T) {
	cfg := &config.Config{
		AppName:     "test-argus",
		LogLevel:    "debug",
		DatabaseURL: testDBURL(t),
	}

	inst, err := New(context.Background(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, inst)
	defer inst.Close()

	assert.Equal(t, "test-argus", inst.Config().AppName)
}
