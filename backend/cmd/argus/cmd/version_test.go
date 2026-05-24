package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/venkatvghub/argus/pkg/constants"
)

func TestVersionCmd_IsNotNil(t *testing.T) {
	assert.NotNil(t, versionCmd)
}

func TestVersionCmd_HasCorrectUse(t *testing.T) {
	assert.Equal(t, "version", versionCmd.Use)
}

func TestVersionCmd_HasCorrectShortDesc(t *testing.T) {
	assert.Equal(t, "Print Argus version information", versionCmd.Short)
}

func TestVersionCmd_HasRunE(t *testing.T) {
	assert.NotNil(t, versionCmd.RunE)
}

func TestVersionCmd_OverridesPersistentPreRunE(t *testing.T) {
	// version command should not initialize instance
	assert.NotNil(t, versionCmd.PersistentPreRunE)

	// Execute the override to ensure it returns nil (no-op)
	err := versionCmd.PersistentPreRunE(versionCmd, []string{})
	assert.NoError(t, err)
}

func TestVersionCmd_RunE(t *testing.T) {
	buf := new(bytes.Buffer)
	versionCmd.SetOut(buf)
	versionCmd.SetContext(context.Background())

	err := versionCmd.RunE(versionCmd, []string{})
	require.NoError(t, err)

	output := buf.Bytes()
	require.NotEmpty(t, output)

	var versionOutput map[string]string
	require.NoError(t, json.Unmarshal(output, &versionOutput))

	assert.NotEmpty(t, versionOutput["version"])
	assert.Equal(t, constants.APIVersion, versionOutput["version"])
	assert.Equal(t, "argus", versionOutput["app"])
	assert.Len(t, versionOutput, 2)
}
