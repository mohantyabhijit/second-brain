package main

import (
	"context"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/cache/rediscache"
	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
)

func main() {
	cfg := config.Load()
	logger := logging.NewForConfig("precompute", cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
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

	state, err := service.PublishReadModels(ctx, "precompute_publish")
	if err != nil {
		logger.Error("precompute read models failed", "error", err)
		os.Exit(1)
	}
	logger.Info(
		"precompute read models completed",
		"run_id", state.Manifest.RunID,
		"etag", state.Manifest.ETag,
		"digests", len(state.Digests),
		"insights", graphInsightCount(state),
	)
}

func graphInsightCount(state *knowledge.AppState) int {
	if state == nil || state.Graph.InsightGraph == nil {
		return 0
	}
	return state.Graph.InsightGraph.Stats.ReturnedInsights
}
