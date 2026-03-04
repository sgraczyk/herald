package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/sgraczyk/herald/internal/config"
	"github.com/sgraczyk/herald/internal/provider"
)

func newAskCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "ask [question]",
		Short: "Ask a question directly (bypass Telegram)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			providers := buildProviders(cfg)
			if len(providers) == 0 {
				return fmt.Errorf("no providers configured")
			}

			chain := provider.NewFallback(providers)
			question := strings.Join(args, " ")

			messages := []provider.Message{
				{Role: "user", Content: question},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			response, err := chain.Chat(ctx, messages)
			if err != nil {
				return err
			}

			fmt.Fprintln(os.Stdout, response)
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "config.json", "path to config file")

	return cmd
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
