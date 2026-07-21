package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/cache/rediscache"
	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	platformtracing "github.com/abhijitmohanty/second-brain/backend/internal/platform/tracing"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
	rcron "github.com/robfig/cron/v3"
)

func main() {
	cfg := config.Load()
	logger := logging.NewForConfig("worker", cfg)
	shutdownTracing, _ := platformtracing.StartLangfuse(context.Background(), cfg, "second-brain-worker", logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownTracing(shutdownCtx)
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, closeStore := openStore(ctx, cfg, logger)
	defer closeStore()

	service := knowledge.NewService(cfg, store, httpclient.New())
	service.SetLogger(logger)
	readModelCache, closeCache := rediscache.Open(ctx, cfg, logger)
	defer closeCache()
	service.SetReadModelCache(readModelCache)

	refreshEvery := parseDuration(cfg.WorkerRefreshInterval, 2*time.Hour)
	cronRunner := rcron.New(rcron.WithChain(rcron.Recover(rcron.DefaultLogger)))
	var runLock sync.Mutex
	runCycle := func(reason string) {
		runLock.Lock()
		defer runLock.Unlock()
		logger.Info("worker cycle started", "reason", reason)
		outcome := runOnce(ctx, cfg, service, logger)
		if outcome.ok && outcome.newContent {
			runGraphSync(ctx, cfg, logger)
		} else if outcome.ok {
			logger.Info("worker graph sync skipped", "reason", outcome.skippedReason)
		}
		if shouldPublishAfterRefresh(outcome) {
			publishReadModels(ctx, service, logger, "post_refresh_precompute")
		} else {
			logger.Warn("worker precompute skipped", "reason", outcome.skippedReason)
		}
	}
	runDigest := func(reason string) {
		runLock.Lock()
		defer runLock.Unlock()
		if generateDigest(ctx, service, logger, reason) {
			publishReadModels(ctx, service, logger, "daily_precompute")
		}
	}

	runCycle("startup")
	if strings.EqualFold(os.Getenv("WORKER_ONCE"), "true") {
		return
	}

	refreshSpec := "@every " + refreshEvery.String()
	digestSpec := digestCronSpec(cfg)
	if _, err := cronRunner.AddFunc(refreshSpec, func() { runCycle("scheduled_refresh") }); err != nil {
		logger.Error("schedule worker refresh", "spec", refreshSpec, "error", err)
		os.Exit(1)
	}
	if _, err := cronRunner.AddFunc(digestSpec, func() { runDigest("scheduled_time") }); err != nil {
		logger.Error("schedule worker digest", "spec", digestSpec, "error", err)
		os.Exit(1)
	}
	logger.Info("self organizing worker started", "refresh_spec", refreshSpec, "daily_digest_spec", digestSpec)
	cronRunner.Start()
	<-ctx.Done()
	stopCtx := cronRunner.Stop()
	<-stopCtx.Done()
	logger.Info("self organizing worker stopped")
}

type refreshOutcome struct {
	ok            bool
	newContent    bool
	skippedReason string
}

func runOnce(ctx context.Context, cfg config.Config, service *knowledge.Service, logger *logging.Logger) refreshOutcome {
	runCtx, cancel := context.WithTimeout(ctx, parseDuration(cfg.RefreshTimeout, 90*time.Minute))
	defer cancel()
	outcome, err := service.RunCycle(runCtx)
	if err != nil {
		logger.Error("worker refresh failed", "error", err)
		return refreshOutcome{ok: false, skippedReason: "refresh_failed"}
	}
	result := outcome.Result
	logger.Info("worker refresh completed", "x_bookmarks", len(result.XBookmarks), "youtube_items", len(result.YouTubeItems), "insights", len(result.Insights), "blockers", len(result.Blockers))
	return refreshOutcome{ok: true, newContent: outcome.NewContent, skippedReason: fallbackReason(outcome.SkippedReason, "no_new_source_materials")}
}

func shouldPublishAfterRefresh(outcome refreshOutcome) bool {
	return outcome.ok
}

func generateDigest(ctx context.Context, service *knowledge.Service, logger *logging.Logger, reason string) bool {
	digestCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	digest, err := service.GenerateDigest(digestCtx)
	if err != nil {
		if errors.Is(err, knowledge.ErrNoNewDigestSources) {
			logger.Info("worker digest skipped", "reason", reason, "error", err)
			return false
		}
		logger.Error("worker digest failed", "reason", reason, "error", err)
		return false
	}
	logger.Info("worker digest completed", "reason", reason, "date", digest.DigestDate, "status", digest.Status, "subject", digest.Subject)
	return true
}

func publishReadModels(ctx context.Context, service *knowledge.Service, logger *logging.Logger, reason string) {
	precomputeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	state, err := service.PublishReadModels(precomputeCtx, reason)
	if err != nil {
		logger.Warn("worker precompute failed", "reason", reason, "error", err)
		return
	}
	logger.Info("worker precompute completed", "reason", reason, "run_id", state.Manifest.RunID, "etag", state.Manifest.ETag)
}

func runGraphSync(ctx context.Context, cfg config.Config, logger *logging.Logger) {
	if strings.TrimSpace(cfg.Neo4jURI) == "" || strings.TrimSpace(cfg.Neo4jUsername) == "" || strings.TrimSpace(cfg.Neo4jPassword) == "" {
		logger.Info("worker graph sync skipped", "reason", "NEO4J_URI, NEO4J_USERNAME, and NEO4J_PASSWORD are required for graph sync")
		return
	}
	executable, err := os.Executable()
	if err != nil {
		logger.Warn("worker graph sync skipped", "error", err)
		return
	}
	graphSyncPath := filepath.Join(filepath.Dir(executable), "second-brain-graph-sync")
	if _, err := os.Stat(graphSyncPath); err != nil {
		logger.Warn("worker graph sync skipped", "path", graphSyncPath, "error", err)
		return
	}
	graphCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(graphCtx, graphSyncPath)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = os.Environ()
	if err := command.Run(); err != nil {
		logger.Warn("worker graph sync failed", "error", err)
		return
	}
	logger.Info("worker graph sync completed")
}

func openStore(ctx context.Context, cfg config.Config, logger *logging.Logger) (knowledge.Store, func()) {
	if cfg.DatabaseURL == "" {
		logger.Warn("DATABASE_URL missing; using local knowledge run file", "path", cfg.KnowledgeRunPath)
		return localfile.New(cfg.KnowledgeRunPath), func() {}
	}
	postgresStore, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect postgres", "error", err)
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

func fallbackReason(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func digestCronSpec(cfg config.Config) string {
	timezone := strings.TrimSpace(cfg.DigestTimezone)
	if timezone == "" {
		timezone = "Asia/Singapore"
	}
	_, err := time.LoadLocation(timezone)
	if err != nil {
		timezone = "Asia/Singapore"
	}
	hour, minute := parseClock(strings.TrimSpace(cfg.DigestTime), 18, 0)
	return fmt.Sprintf("CRON_TZ=%s %d %d * * *", timezone, minute, hour)
}

func parseClock(value string, fallbackHour int, fallbackMinute int) (int, int) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return fallbackHour, fallbackMinute
	}
	hour, okHour := parseClockPart(parts[0])
	minute, okMinute := parseClockPart(parts[1])
	if !okHour || !okMinute || hour > 23 || minute > 59 {
		return fallbackHour, fallbackMinute
	}
	return hour, minute
}

func parseClockPart(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		parsed = parsed*10 + int(r-'0')
	}
	return parsed, true
}
