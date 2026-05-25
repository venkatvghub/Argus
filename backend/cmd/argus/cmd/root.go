package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/venkatvghub/argus/pkg/argus"
	"github.com/venkatvghub/argus/pkg/config"
)

var (
	instance *argus.Instance
	rootRepoID string

	// persistent flag values
	flagLogLevel string
)

var rootCmd = &cobra.Command{
	Use:   "argus",
	Short: "Argus — deep codebase intelligence",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		if flagLogLevel != "" {
			cfg.LogLevel = flagLogLevel
		}
		inst, err := argus.New(cmd.Context(), cfg)
		if err != nil {
			return fmt.Errorf("init: %w", err)
		}
		instance = inst
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if instance != nil {
			return instance.Close()
		}
		return nil
	},
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "{\"error\": %q}\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagLogLevel, "log-level", "", "override log level")
	rootCmd.PersistentFlags().StringVar(&rootRepoID, "repo-id", "", "repository ID for query commands")

	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(reposCmd)
	rootCmd.AddCommand(symbolsCmd)
	rootCmd.AddCommand(markersCmd)
	rootCmd.AddCommand(scoreCmd)
	rootCmd.AddCommand(communityCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(jobsCmd)
	rootCmd.AddCommand(wikiCmd)
}
