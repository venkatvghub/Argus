package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB(t *testing.T) {
	dataDir := t.TempDir()
	// New() now takes a full DB path, not just a directory.
	dbPath := dataDir + "/argus.db"
	db, err := New(dbPath)
	require.NoError(t, err)
	defer db.Close()

	// Verify table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='repositories'").Scan(&name)
	assert.NoError(t, err)
	assert.Equal(t, "repositories", name)
}
