package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

func initLogging(levelStr string) {
	var level slog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	fi, err := os.Stderr.Stat()
	if err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

func serve(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	initLogging(cfg.LogLevel)

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
		tokenExpires := os.Getenv("CLAUDE_TOKEN_EXPIRES")
		if tokenExpires != "" {
			if _, err := time.Parse("2006-01-02", tokenExpires); err != nil {
				slog.Warn("invalid CLAUDE_TOKEN_EXPIRES, ignoring", slog.String("value", tokenExpires))
				tokenExpires = ""
			}
		}
		var claude health.ProviderStatus
		for _, p := range providers {
			if c, ok := p.(*provider.Claude); ok {
				claude = c
				break
			}
		}
		srv := health.NewServer(cfg.HTTPPort, version, loop.StartTime(), chain.Name(), claude, tokenExpires)
		if err := srv.Start(ctx); err != nil {
			return fmt.Errorf("start health server: %w", err)
		}
		slog.Info("health endpoint started", slog.Int("port", cfg.HTTPPort))
	}

	// Start agent loop.
	go loop.Run(ctx)

	slog.Info("herald starting", slog.String("version", version), slog.String("provider", chain.Name()))

	// Start Telegram (blocks until ctx cancelled).
	tg.Start(ctx)

	slog.Info("herald stopped")
	return nil
}
