package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type cloudflarePurgeResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (s *Service) purgeEdgeCacheBestEffort(ctx context.Context, reason string) {
	if !s.shouldPurgeEdgeCache(reason) {
		return
	}
	files := edgeCachePurgeURLs(s.cfg.PublicBaseURL)
	if len(files) == 0 {
		return
	}
	purgeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.purgeCloudflareFiles(purgeCtx, files); err != nil {
		s.log(ctx).Warn("edge cache purge failed", "reason", reason, "error", err)
		return
	}
	s.log(ctx).Info("edge cache purge completed", "reason", reason, "files", len(files))
}

func (s *Service) shouldPurgeEdgeCache(reason string) bool {
	if !s.cfg.CloudflareCachePurgeEnabled {
		return false
	}
	if strings.TrimSpace(s.cfg.CloudflareAPIToken) == "" || strings.TrimSpace(s.cfg.CloudflareZoneID) == "" {
		return false
	}
	switch reason {
	case "refresh_publish", "refresh_noop_publish", "digest_publish", "precompute_publish", "post_refresh_precompute", "daily_precompute":
		return true
	default:
		return false
	}
}

func (s *Service) purgeCloudflareFiles(ctx context.Context, files []string) error {
	apiBase := strings.TrimRight(strings.TrimSpace(s.cfg.CloudflareAPIBaseURL), "/")
	if apiBase == "" {
		apiBase = "https://api.cloudflare.com/client/v4"
	}
	endpoint := apiBase + "/zones/" + url.PathEscape(strings.TrimSpace(s.cfg.CloudflareZoneID)) + "/purge_cache"
	raw, err := json.Marshal(map[string]any{"files": files})
	if err != nil {
		return err
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+strings.TrimSpace(s.cfg.CloudflareAPIToken))
	headers.Set("Content-Type", "application/json")
	var response cloudflarePurgeResponse
	if err := s.requestJSON(ctx, http.MethodPost, endpoint, headers, bytes.NewReader(raw), &response); err != nil {
		return err
	}
	if !response.Success {
		messages := make([]string, 0, len(response.Errors))
		for _, item := range response.Errors {
			if strings.TrimSpace(item.Message) != "" {
				messages = append(messages, item.Message)
			}
		}
		if len(messages) == 0 {
			messages = append(messages, "unknown Cloudflare purge failure")
		}
		return errors.New(strings.Join(messages, "; "))
	}
	return nil
}

func edgeCachePurgeURLs(baseURL string) []string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil
	}
	paths := []string{
		"/",
		"/insights/",
		"/daily-newsletter/",
		"/original-x-bookmarks/",
		"/original-x-posts/",
		"/original-youtube-videos/",
		"/original-youtube-posts/",
		"/knowledge-graph/",
		"/api/app-state",
		"/api/app-state?view=insights&limit=20",
		"/api/app-state?view=daily-newsletter&limit=10",
		"/api/app-state?view=original-x-posts&limit=15",
		"/api/app-state?view=original-youtube-posts&limit=15",
		"/api/app-state?view=knowledge-graph&limit=180",
		"/api/digests",
		"/api/knowledge-graph/insights?limit=180",
	}
	urls := make([]string, 0, len(paths))
	for _, path := range paths {
		urls = append(urls, base+path)
	}
	return urls
}
