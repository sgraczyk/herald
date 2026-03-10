package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/sgraczyk/herald"
	"github.com/sgraczyk/herald/internal/config"
	"github.com/sgraczyk/herald/internal/provider"
)

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask a question directly (bypass Telegram)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")

			cfg, err := config.LoadWithDefaults(configPath, herald.DefaultConfig)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			providers := buildProviders(cfg)
			if len(providers) == 0 {
				return fmt.Errorf("no providers configured")
			}

			chain := provider.NewFallback(providers, *cfg.MaxRetries, nil)
			question := strings.Join(args, " ")

			messages := []provider.Message{
				{Role: "user", Content: question},
			}

			response, err := chain.Chat(context.Background(), messages)
			if err != nil {
				return err
			}

			fmt.Fprintln(os.Stdout, response)
			return nil
		},
	}
}

func buildProviders(cfg *config.Config) []provider.LLMProvider {
	var providers []provider.LLMProvider
	for _, pc := range cfg.Providers {
		switch pc.Type {
		case "claude-cli":
			providers = append(providers, provider.NewClaude())
		case "openai":
			if pc.APIKey != "" {
				providers = append(providers, provider.NewOpenAI(pc.Name, pc.BaseURL, pc.Model, pc.APIKey))
			}
		}
	}
	return providers
}
