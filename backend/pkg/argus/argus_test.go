package argus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/venkatvghub/argus/pkg/config"
)

func TestNew(t *testing.T) {
	dataDir := t.TempDir()
	dbPath := dataDir + "/argus.db"
	cfg := &config.Config{
		AppName:  "test-argus",
		LogLevel: "debug",
		DBPath:   dbPath,
	}

	inst, err := New(context.Background(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, inst)
	defer inst.Close()

	assert.Equal(t, "test-argus", inst.Config().AppName)
}
