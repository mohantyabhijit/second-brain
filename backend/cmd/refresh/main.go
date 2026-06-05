package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/cache/rediscache"
	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	platformtracing "github.com/abhijitmohanty/second-brain/backend/internal/platform/tracing"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	shutdownTracing, _ := platformtracing.StartLangfuse(context.Background(), cfg, "second-brain-refresh", logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownTracing(shutdownCtx)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), parseDuration(cfg.RefreshTimeout, 90*time.Minute))
	defer cancel()

	var store knowledge.Store
	var closeStore func()
	if cfg.DatabaseURL == "" {
		logger.Warn("DATABASE_URL missing; using local knowledge run file", "path", cfg.KnowledgeRunPath)
		store = localfile.New(cfg.KnowledgeRunPath)
		closeStore = func() {}
	} else {
		postgresStore, err := postgres.New(ctx, cfg.DatabaseURL)
		if err != nil {
			logger.Error("connect postgres", "error", err)
			os.Exit(1)
		}
		store = postgresStore
		closeStore = postgresStore.Close
	}
	defer closeStore()

	service := knowledge.NewService(cfg, store, httpclient.New())
	service.SetLogger(logger)
	readModelCache, closeCache := rediscache.Open(ctx, cfg, logger)
	defer closeCache()
	service.SetReadModelCache(readModelCache)
	outcome, err := service.RunCycle(ctx)
	if err != nil {
		logger.Error("knowledge refresh failed", "error", err)
		os.Exit(1)
	}
	result := outcome.Result
	logger.Info(
		"knowledge refresh completed",
		"new_content", outcome.NewContent,
		"skipped_reason", outcome.SkippedReason,
		"x_bookmarks", len(result.XBookmarks),
		"youtube_items", len(result.YouTubeItems),
		"summaries", len(result.Summaries),
		"insights", len(result.Insights),
		"actions", len(result.ActionItems),
	)
}

func parseDuration(value string, fallbackValue time.Duration) time.Duration {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallbackValue
	}
	return parsed
}
