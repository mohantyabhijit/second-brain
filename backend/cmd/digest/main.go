package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var store knowledge.Store
	var closeStore func()
	if cfg.SupabaseDatabaseURL == "" {
		logger.Warn("SUPABASE_DB_URL missing; using local knowledge run file", "path", cfg.KnowledgeRunPath)
		store = localfile.New(cfg.KnowledgeRunPath)
		closeStore = func() {}
	} else {
		postgresStore, err := postgres.New(ctx, cfg.SupabaseDatabaseURL)
		if err != nil {
			logger.Error("connect supabase postgres", "error", err)
			os.Exit(1)
		}
		store = postgresStore
		closeStore = postgresStore.Close
	}
	defer closeStore()

	service := knowledge.NewService(cfg, store, httpclient.New())
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
}
