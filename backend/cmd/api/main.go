package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/cache/rediscache"
	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/httpapi"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	platformtracing "github.com/abhijitmohanty/second-brain/backend/internal/platform/tracing"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
)

func main() {
	cfg := config.Load()
	logger := logging.NewForConfig("api", cfg)
	shutdownTracing, _ := platformtracing.StartLangfuse(context.Background(), cfg, "second-brain-api", logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownTracing(shutdownCtx)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	readModelCache, closeCache := rediscache.Open(ctx, cfg, logger)
	defer closeCache()
	service.SetReadModelCache(readModelCache)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(cfg, service, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("api shutdown failed", "error", err)
	}
}
