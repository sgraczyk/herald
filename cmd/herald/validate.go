package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sgraczyk/herald"
	"github.com/sgraczyk/herald/internal/config"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-config",
		Short: "Validate config file and report warnings",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.LoadWithDefaults(configPath, herald.DefaultConfig)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			result := cfg.Validate()

			for _, w := range result.Warnings {
				fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
			}
			for _, d := range result.Defaults {
				fmt.Fprintf(os.Stderr, "INFO: %s\n", d)
			}

			if len(result.Warnings) == 0 && len(result.Defaults) == 0 {
				fmt.Fprintln(os.Stderr, "config OK — no warnings, no defaults applied")
			}

			return nil
		},
	}
}
