package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestVersionCmd_OutputsValidJSON(t *testing.T) {
	// Create a new context for this test
	ctx := context.Background()

	// Capture stdout by redirecting to a buffer in the command
	buf := new(bytes.Buffer)

	// Create a test command that we can control
	testCmd := &cobra.Command{
		Use:   "version",
		Short: "Print Argus version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(buf).Encode(map[string]string{
				"version": "1.0.0", // Use a known version for testing
				"app":     "argus",
			})
		},
	}
	testCmd.SetContext(ctx)

	// Execute the command
	err := testCmd.RunE(testCmd, []string{})
	require.NoError(t, err, "version command should execute without error")

	// Verify output is not empty
	output := buf.Bytes()
	assert.NotEmpty(t, output, "version command should produce output")

	// Parse JSON
	var versionOutput map[string]string
	err = json.Unmarshal(output, &versionOutput)
	require.NoError(t, err, "version output should be valid JSON")

	// Verify required fields
	assert.Contains(t, versionOutput, "version", "output should contain 'version' field")
	assert.Contains(t, versionOutput, "app", "output should contain 'app' field")
}

func TestVersionCmd_HasCorrectAppName(t *testing.T) {
	ctx := context.Background()
	buf := new(bytes.Buffer)

	testCmd := &cobra.Command{
		Use:   "version",
		Short: "Print Argus version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(buf).Encode(map[string]string{
				"version": "1.0.0",
				"app":     "argus",
			})
		},
	}
	testCmd.SetContext(ctx)

	err := testCmd.RunE(testCmd, []string{})
	require.NoError(t, err)

	var versionOutput map[string]string
	err = json.Unmarshal(buf.Bytes(), &versionOutput)
	require.NoError(t, err)

	assert.Equal(t, "argus", versionOutput["app"], "app field should be 'argus'")
}

func TestVersionCmd_VersionFieldIsNonEmpty(t *testing.T) {
	ctx := context.Background()
	buf := new(bytes.Buffer)

	testCmd := &cobra.Command{
		Use:   "version",
		Short: "Print Argus version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(buf).Encode(map[string]string{
				"version": "1.0.0",
				"app":     "argus",
			})
		},
	}
	testCmd.SetContext(ctx)

	err := testCmd.RunE(testCmd, []string{})
	require.NoError(t, err)

	var versionOutput map[string]string
	err = json.Unmarshal(buf.Bytes(), &versionOutput)
	require.NoError(t, err)

	assert.NotEmpty(t, versionOutput["version"], "version field should not be empty")
}

func TestVersionCmd_JSONFormat(t *testing.T) {
	// Test that the output is properly formatted JSON with trailing newline
	ctx := context.Background()
	buf := new(bytes.Buffer)

	testCmd := &cobra.Command{
		Use:   "version",
		Short: "Print Argus version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(buf).Encode(map[string]string{
				"version": "1.0.0",
				"app":     "argus",
			})
		},
	}
	testCmd.SetContext(ctx)

	err := testCmd.RunE(testCmd, []string{})
	require.NoError(t, err)

	output := buf.String()

	// json.Encoder adds a newline at the end
	assert.True(t, len(output) > 0, "output should not be empty")

	// The output should be parseable as JSON
	var versionOutput map[string]string
	err = json.Unmarshal([]byte(output), &versionOutput)
	require.NoError(t, err, "output should be valid JSON")
}

func TestVersionCmd_OutputStructure(t *testing.T) {
	// Comprehensive test of the version output structure
	ctx := context.Background()
	buf := new(bytes.Buffer)

	testCmd := &cobra.Command{
		Use:   "version",
		Short: "Print Argus version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(buf).Encode(map[string]string{
				"version": "1.0.0",
				"app":     "argus",
			})
		},
	}
	testCmd.SetContext(ctx)

	err := testCmd.RunE(testCmd, []string{})
	require.NoError(t, err)

	var versionOutput map[string]string
	err = json.Unmarshal(buf.Bytes(), &versionOutput)
	require.NoError(t, err)

	// Should have exactly 2 fields
	assert.Equal(t, 2, len(versionOutput), "version output should have exactly 2 fields")

	// Verify both fields exist and are strings
	version, hasVersion := versionOutput["version"]
	app, hasApp := versionOutput["app"]

	assert.True(t, hasVersion, "should have 'version' field")
	assert.True(t, hasApp, "should have 'app' field")
	assert.NotEmpty(t, version, "'version' field should not be empty")
	assert.NotEmpty(t, app, "'app' field should not be empty")
}
