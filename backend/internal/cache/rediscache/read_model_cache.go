package rediscache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	goredis "github.com/redis/go-redis/v9"
)

type Cache struct {
	client     *goredis.Client
	ttl        time.Duration
	refreshTTL time.Duration
}

func Open(ctx context.Context, cfg config.Config, logger *slog.Logger) (knowledge.ReadModelCache, func()) {
	if !cfg.RedisCacheEnabled || strings.TrimSpace(cfg.RedisURL) == "" {
		return nil, func() {}
	}
	cache, err := New(ctx, cfg.RedisURL, parseDuration(cfg.RedisCacheTTL, 30*24*time.Hour), parseDuration(cfg.RedisRefreshStatusTTL, 24*time.Hour))
	if err != nil {
		if logger != nil {
			logger.Warn("redis read-model cache disabled", "error", err)
		}
		return nil, func() {}
	}
	if logger != nil {
		logger.Info("redis read-model cache enabled")
	}
	return cache, func() {
		_ = cache.Close()
	}
}

func New(ctx context.Context, redisURL string, ttl time.Duration, refreshTTL time.Duration) (*Cache, error) {
	options, err := goredis.ParseURL(strings.TrimSpace(redisURL))
	if err != nil {
		return nil, err
	}
	client := goredis.NewClient(options)
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	if refreshTTL <= 0 {
		refreshTTL = 24 * time.Hour
	}
	return &Cache{client: client, ttl: ttl, refreshTTL: refreshTTL}, nil
}

func (c *Cache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *Cache) ReadAppState(ctx context.Context, ownerID string) (*knowledge.AppState, error) {
	manifest, err := c.readManifest(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	raw, err := c.client.Get(ctx, appStateKey(ownerID, manifest.RunID)).Bytes()
	if err != nil {
		return nil, redisError(err)
	}
	var state knowledge.AppState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	state.Manifest = manifest
	return &state, nil
}

func (c *Cache) ReadLatest(ctx context.Context, ownerID string) (*knowledge.Result, error) {
	manifest, err := c.readManifest(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	raw, err := c.client.Get(ctx, latestRunKey(ownerID, manifest.RunID)).Bytes()
	if err != nil {
		return nil, redisError(err)
	}
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var result knowledge.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	knowledge.NormalizeResultForReadModel(&result)
	return &result, nil
}

func (c *Cache) ReadDigests(ctx context.Context, ownerID string, limit int) ([]knowledge.DigestIssue, error) {
	manifest, err := c.readManifest(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	raw, err := c.client.Get(ctx, digestsKey(ownerID, manifest.RunID)).Bytes()
	if err != nil {
		return nil, redisError(err)
	}
	var digests []knowledge.DigestIssue
	if err := json.Unmarshal(raw, &digests); err != nil {
		return nil, err
	}
	if limit > 0 && len(digests) > limit {
		digests = digests[:limit]
	}
	if digests == nil {
		digests = []knowledge.DigestIssue{}
	}
	return digests, nil
}

func (c *Cache) ReadSourceMaterialStates(ctx context.Context, ownerID string, keys []knowledge.SourceMaterialKey) (map[string]knowledge.SourceMaterialState, error) {
	states := map[string]knowledge.SourceMaterialState{}
	if len(keys) == 0 {
		return states, nil
	}
	fields := make([]string, 0, len(keys))
	for _, key := range keys {
		fields = append(fields, key.String())
	}
	values, err := c.client.HMGet(ctx, sourceMaterialsKey(ownerID), fields...).Result()
	if err != nil {
		return states, err
	}
	for index, value := range values {
		if value == nil {
			continue
		}
		raw := ""
		switch typed := value.(type) {
		case string:
			raw = typed
		case []byte:
			raw = string(typed)
		default:
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var state knowledge.SourceMaterialState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			return states, err
		}
		states[fields[index]] = state
	}
	if len(states) == 0 {
		return states, knowledge.ErrReadModelCacheMiss
	}
	return states, nil
}

func (c *Cache) ReadRefreshStatus(ctx context.Context, ownerID string) (*knowledge.RefreshStatus, error) {
	raw, err := c.client.Get(ctx, refreshStatusKey(ownerID)).Bytes()
	if err != nil {
		return nil, redisError(err)
	}
	var status knowledge.RefreshStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Cache) WriteRefreshStatus(ctx context.Context, ownerID string, status knowledge.RefreshStatus) error {
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, refreshStatusKey(ownerID), raw, c.refreshTTL).Err()
}

func (c *Cache) PublishSourceMaterialStates(ctx context.Context, ownerID string, states []knowledge.SourceMaterialState) error {
	if len(states) == 0 {
		return nil
	}
	values := map[string]any{}
	for _, state := range states {
		if strings.TrimSpace(state.ExternalID) == "" || strings.TrimSpace(state.PromptVersion) == "" || strings.TrimSpace(state.Model) == "" {
			continue
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		values[state.Key().String()] = raw
	}
	if len(values) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	pipe.HSet(ctx, sourceMaterialsKey(ownerID), values)
	pipe.Expire(ctx, sourceMaterialsKey(ownerID), c.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *Cache) PublishAppState(ctx context.Context, ownerID string, state knowledge.AppState) error {
	runID := strings.TrimSpace(state.Manifest.RunID)
	if runID == "" {
		return fmt.Errorf("app state manifest run ID is required")
	}

	latestRaw, err := json.Marshal(state.Latest)
	if err != nil {
		return err
	}
	appStateRaw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	digestsRaw, err := json.Marshal(state.Digests)
	if err != nil {
		return err
	}
	graphRaw, err := json.Marshal(state.Graph)
	if err != nil {
		return err
	}
	askContextRaw, err := json.Marshal(state.AskContext)
	if err != nil {
		return err
	}
	insightsRaw, err := json.Marshal(state.Views.Insights)
	if err != nil {
		return err
	}
	newsletterRaw, err := json.Marshal(state.Views.DailyNewsletter)
	if err != nil {
		return err
	}
	xBookmarksRaw, err := json.Marshal(state.Views.OriginalXBookmarks)
	if err != nil {
		return err
	}
	youtubeRaw, err := json.Marshal(state.Views.OriginalYouTubePosts)
	if err != nil {
		return err
	}

	pipe := c.client.Pipeline()
	pipe.Set(ctx, latestRunKey(ownerID, runID), latestRaw, c.ttl)
	pipe.Set(ctx, appStateKey(ownerID, runID), appStateRaw, c.ttl)
	pipe.Set(ctx, digestsKey(ownerID, runID), digestsRaw, c.ttl)
	pipe.Set(ctx, graphKey(ownerID, runID), graphRaw, c.ttl)
	pipe.Set(ctx, askContextKey(ownerID, runID), askContextRaw, c.ttl)
	pipe.Set(ctx, viewKey(ownerID, runID, "insights"), insightsRaw, c.ttl)
	pipe.Set(ctx, viewKey(ownerID, runID, "daily-newsletter"), newsletterRaw, c.ttl)
	pipe.Set(ctx, viewKey(ownerID, runID, "original-x-bookmarks"), xBookmarksRaw, c.ttl)
	pipe.Set(ctx, viewKey(ownerID, runID, "original-youtube-posts"), youtubeRaw, c.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	if state.Latest != nil {
		if err := c.PublishSourceMaterialStates(ctx, ownerID, knowledge.SourceMaterialStatesFromResult(state.Latest)); err != nil {
			return err
		}
	}

	return c.client.HSet(ctx, manifestKey(ownerID), map[string]any{
		"schema_version": knowledge.AppStateSchemaVersion,
		"current_run_id": runID,
		"generated_at":   state.Manifest.GeneratedAt.UTC().Format(time.RFC3339Nano),
		"published_at":   state.Manifest.PublishedAt.UTC().Format(time.RFC3339Nano),
		"etag":           state.Manifest.ETag,
		"graph_status":   state.Manifest.GraphStatus,
		"digest_status":  state.Manifest.DigestStatus,
	}).Err()
}

func (c *Cache) readManifest(ctx context.Context, ownerID string) (knowledge.AppStateManifest, error) {
	fields, err := c.client.HGetAll(ctx, manifestKey(ownerID)).Result()
	if err != nil {
		return knowledge.AppStateManifest{}, err
	}
	if len(fields) == 0 || strings.TrimSpace(fields["current_run_id"]) == "" {
		return knowledge.AppStateManifest{}, knowledge.ErrReadModelCacheMiss
	}
	generatedAt, _ := time.Parse(time.RFC3339Nano, fields["generated_at"])
	publishedAt, _ := time.Parse(time.RFC3339Nano, fields["published_at"])
	return knowledge.AppStateManifest{
		SchemaVersion: fallback(fields["schema_version"], knowledge.AppStateSchemaVersion),
		RunID:         fields["current_run_id"],
		GeneratedAt:   generatedAt,
		PublishedAt:   publishedAt,
		ETag:          fields["etag"],
		GraphStatus:   fields["graph_status"],
		DigestStatus:  fields["digest_status"],
	}, nil
}

func redisError(err error) error {
	if err == goredis.Nil {
		return knowledge.ErrReadModelCacheMiss
	}
	return err
}

func parseDuration(value string, fallbackValue time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallbackValue
	}
	return parsed
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func manifestKey(ownerID string) string {
	return key(ownerID, "manifest")
}

func appStateKey(ownerID string, runID string) string {
	return key(ownerID, "app-state:"+runID)
}

func latestRunKey(ownerID string, runID string) string {
	return key(ownerID, "run:"+runID+":latest")
}

func viewKey(ownerID string, runID string, view string) string {
	return key(ownerID, "view:"+runID+":"+view)
}

func digestsKey(ownerID string, runID string) string {
	return key(ownerID, "digests:"+runID+":list")
}

func refreshStatusKey(ownerID string) string {
	return key(ownerID, "refresh:status")
}

func sourceMaterialsKey(ownerID string) string {
	return key(ownerID, "source-materials")
}

func graphKey(ownerID string, runID string) string {
	return key(ownerID, "graph:"+runID+":read-model")
}

func askContextKey(ownerID string, runID string) string {
	return key(ownerID, "ask:context:"+runID)
}

func key(ownerID string, suffix string) string {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		ownerID = "default"
	}
	return "sb:v1:" + ownerID + ":" + suffix
}
