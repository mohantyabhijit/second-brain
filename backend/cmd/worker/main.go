package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, closeStore := openStore(ctx, cfg, logger)
	defer closeStore()

	service := knowledge.NewService(cfg, store, httpclient.New())
	service.SetLogger(logger)

	refreshEvery := parseDuration(cfg.WorkerRefreshInterval, 2*time.Hour)
	digestEvery := parseDuration(cfg.WorkerDigestInterval, 2*time.Hour)
	logger.Info("self organizing worker started", "refresh_interval", refreshEvery.String(), "digest_interval", digestEvery.String(), "digest_time", cfg.DigestTime, "timezone", cfg.DigestTimezone)

	var lastDigest time.Time
	for {
		runOnce(ctx, cfg, service, logger)
		if digestEvery > 0 && time.Since(lastDigest) >= digestEvery {
			generateDigest(ctx, service, logger)
			lastDigest = time.Now()
		}
		if strings.EqualFold(os.Getenv("WORKER_ONCE"), "true") {
			return
		}
		timer := time.NewTimer(refreshEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Info("self organizing worker stopped")
			return
		case <-timer.C:
		}
	}
}

func runOnce(ctx context.Context, cfg config.Config, service *knowledge.Service, logger *slog.Logger) {
	runCtx, cancel := context.WithTimeout(ctx, parseDuration(cfg.RefreshTimeout, 90*time.Minute))
	defer cancel()
	result, err := service.Run(runCtx)
	if err != nil {
		logger.Error("worker refresh failed", "error", err)
		return
	}
	logger.Info("worker refresh completed", "x_bookmarks", len(result.XBookmarks), "youtube_items", len(result.YouTubeItems), "insights", len(result.Insights), "blockers", len(result.Blockers))
}

func generateDigest(ctx context.Context, service *knowledge.Service, logger *slog.Logger) {
	digestCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	digest, err := service.GenerateDigest(digestCtx)
	if err != nil {
		logger.Error("worker digest failed", "error", err)
		return
	}
	logger.Info("worker digest completed", "date", digest.DigestDate, "status", digest.Status, "subject", digest.Subject)
}

func openStore(ctx context.Context, cfg config.Config, logger *slog.Logger) (knowledge.Store, func()) {
	if cfg.SupabaseDatabaseURL == "" {
		logger.Warn("SUPABASE_DB_URL missing; using local knowledge run file", "path", cfg.KnowledgeRunPath)
		return localfile.New(cfg.KnowledgeRunPath), func() {}
	}
	postgresStore, err := postgres.New(ctx, cfg.SupabaseDatabaseURL)
	if err != nil {
		logger.Error("connect supabase postgres", "error", err)
		os.Exit(1)
	}
	return postgresStore, postgresStore.Close
}

func parseDuration(value string, fallbackValue time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallbackValue
	}
	return parsed
}
