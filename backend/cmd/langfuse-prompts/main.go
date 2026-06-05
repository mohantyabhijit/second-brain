package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	langfuseclient "github.com/abhijitmohanty/second-brain/backend/internal/platform/langfuse"
	"github.com/abhijitmohanty/second-brain/backend/prompts"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := langfuseclient.NewClient(cfg, httpclient.New())
	if !client.Enabled() {
		logger.Error("LANGFUSE_BASE_URL is required to sync prompts")
		os.Exit(1)
	}
	labels := []string{fallback(cfg.LangfusePromptLabel, "production")}
	promptName := fallback(cfg.LangfuseDigestPromptName, "second-brain/digest-newsletter")
	promptBody := strings.Join(
		prompts.AppendInputJSON(
			prompts.DigestNewsletterLines("{{digest_date}}"),
			"{{input_json}}",
		),
		"\n",
	)
	prompt, created, err := client.EnsureTextPrompt(ctx, promptName, promptBody, labels, map[string]any{
		"source":         "second-brain",
		"prompt_version": prompts.DigestPromptVersion,
		"managed_by":     "backend/cmd/langfuse-prompts",
	})
	if err != nil {
		logger.Error("sync langfuse prompt", "name", promptName, "error", err)
		os.Exit(1)
	}
	logger.Info("langfuse prompt ready", "name", prompt.Name, "version", prompt.Version, "created", created, "labels", strings.Join(labels, ","))
}

func fallback(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
