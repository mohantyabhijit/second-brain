package main

import (
	"context"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
)

func main() {
	cfg := config.Load()
	logger := logging.NewForConfig("x-auth-check", cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	service := knowledge.NewService(cfg, localfile.New(cfg.KnowledgeRunPath), httpclient.New())
	service.SetLogger(logger)
	profile, err := service.CheckXAuth(ctx)
	if err != nil {
		logger.Error("x auth check failed", "error", err)
		os.Exit(1)
	}
	logger.Info("x auth check passed", "profile", "@"+profile.Username, "user_id", profile.ID)
}
