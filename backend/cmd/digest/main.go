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
	shutdownTracing, _ := platformtracing.StartLangfuse(context.Background(), cfg, "second-brain-digest", logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownTracing(shutdownCtx)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	digest, err := service.GenerateDigest(ctx)
	if err != nil {
		logger.Error("generate digest", "error", err)
		os.Exit(1)
	}
	if digest == nil {
		logger.Error("generate digest returned no issue")
		os.Exit(1)
	}
	logger.Info("digest generated", "date", digest.DigestDate, "status", digest.Status, "subject", digest.Subject)
	for _, delivery := range digest.Deliveries {
		logger.Info(
			"digest delivery",
			"provider", delivery.Provider,
			"recipient", delivery.Recipient,
			"status", delivery.Status,
			"provider_message_id", delivery.ProviderMessageID,
			"error", delivery.Error,
		)
	}
}
