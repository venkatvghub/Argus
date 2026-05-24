package cmd

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/constants"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Argus version information",
	// Override parent's PersistentPreRunE so instance init is skipped.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	RunE: func(cmd *cobra.Command, args []string) error {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"version": constants.APIVersion,
			"app":     "argus",
		})
	},
}
