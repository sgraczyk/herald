package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/sgraczyk/herald/internal/agent"
	"github.com/sgraczyk/herald/internal/config"
	"github.com/sgraczyk/herald/internal/health"
	"github.com/sgraczyk/herald/internal/hub"
	"github.com/sgraczyk/herald/internal/provider"
	"github.com/sgraczyk/herald/internal/store"
	"github.com/sgraczyk/herald/internal/telegram"
)

var version = "dev"

func main() {
	var configPath string

	root := &cobra.Command{
		Use:     "herald",
		Short:   "Lightweight AI assistant bot for Telegram",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(configPath)
		},
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVarP(&configPath, "config", "c", "config.json", "path to config file")
	root.AddCommand(newAskCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func serve(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Telegram.Token == "" {
		return fmt.Errorf("telegram token not set (env var: %s)", cfg.Telegram.TokenEnv)
	}

	// Open store.
	db, err := store.Open(cfg.Store.Path)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// Build providers.
	providers := buildProviders(cfg)
	if len(providers) == 0 {
		return fmt.Errorf("no providers configured")
	}
	chain := provider.NewFallback(providers)

	// Create hub.
	h := hub.New()

	// Create agent loop.
	loop := agent.NewLoop(h, chain, db, cfg.HistoryLimit)

	// Create Telegram adapter.
	tg, err := telegram.New(cfg.Telegram.Token, h, cfg.AllowedUserIDs)
	if err != nil {
		return fmt.Errorf("create telegram adapter: %w", err)
	}

	// Graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start health server.
	if cfg.HTTPPort > 0 {
		srv := health.NewServer(cfg.HTTPPort, version, loop.StartTime(), chain.Name())
		if err := srv.Start(ctx); err != nil {
			return fmt.Errorf("start health server: %w", err)
		}
		log.Printf("health endpoint on :%d", cfg.HTTPPort)
	}

	// Start agent loop.
	go loop.Run(ctx)

	log.Printf("herald %s starting (provider: %s)", version, chain.Name())

	// Start Telegram (blocks until ctx cancelled).
	tg.Start(ctx)

	log.Println("herald stopped")
	return nil
}
