package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	if strings.TrimSpace(cfg.SupabaseDatabaseURL) == "" {
		logger.Error("SUPABASE_DB_URL is required to import X OAuth tokens into the shared store")
		os.Exit(1)
	}
	refreshToken := strings.TrimSpace(os.Getenv("X_REFRESH_TOKEN"))
	if refreshToken == "" {
		logger.Error("X_REFRESH_TOKEN is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store, err := postgres.New(ctx, cfg.SupabaseDatabaseURL)
	if err != nil {
		logger.Error("connect supabase postgres", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	service := knowledge.NewService(cfg, store, httpclient.New())
	service.SetLogger(logger)
	result, err := service.ImportXRefreshToken(ctx, refreshToken)
	if err != nil {
		logger.Error("import X OAuth token store", "error", err)
		os.Exit(1)
	}
	logger.Info("x oauth tokens imported to shared store", "profile", "@"+result.Profile.Username, "access_expires_at", result.AccessExpiresAt.Format(time.RFC3339))
}
