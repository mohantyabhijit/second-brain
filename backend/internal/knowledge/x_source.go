package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type xUserResponse struct {
	Data *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"data"`
}

type xBookmarkResponse struct {
	Data []struct {
		ID            string         `json:"id"`
		Text          string         `json:"text"`
		AuthorID      string         `json:"author_id"`
		CreatedAt     string         `json:"created_at"`
		PublicMetrics map[string]int `json:"public_metrics"`
		Article       *struct {
			Title       string `json:"title"`
			PlainText   string `json:"plain_text"`
			PreviewText string `json:"preview_text"`
		} `json:"article"`
		Entities *struct {
			URLs []struct {
				ExpandedURL string `json:"expanded_url"`
				UnwoundURL  string `json:"unwound_url"`
			} `json:"urls"`
		} `json:"entities"`
	} `json:"data"`
	Includes *struct {
		Users []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"users"`
	} `json:"includes"`
}

type xTokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDetail  string `json:"error_description"`
}

func (s *Service) fetchXBookmarks(ctx context.Context, limit int) ([]XBookmark, error) {
	accessToken, err := s.refreshXAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	if accessToken == "" && os.Getenv("X_USER_ACCESS_TOKEN") == "" && !s.cfg.OneCLIGateway {
		return nil, fmt.Errorf(credentialHint("X_USER_ACCESS_TOKEN"))
	}

	headers := xAccessHeaders(accessToken)
	var me xUserResponse
	if err := s.requestJSON(ctx, http.MethodGet, "https://api.x.com/2/users/me?user.fields=username,name", headers, nil, &me); err != nil {
		return nil, fmt.Errorf("X /2/users/me failed: %w", err)
	}
	if me.Data == nil || me.Data.ID == "" {
		return nil, fmt.Errorf("X /2/users/me did not return an authenticated user id")
	}

	requestURL := "https://api.x.com/2/users/" + me.Data.ID + "/bookmarks"
	requestURL = appendQueryValue(requestURL, "max_results", strconv.Itoa(limit))
	requestURL = appendQueryValue(requestURL, "tweet.fields", "article,created_at,public_metrics,author_id,entities")
	requestURL = appendQueryValue(requestURL, "expansions", "author_id,article.cover_media")
	requestURL = appendQueryValue(requestURL, "media.fields", "url,preview_image_url,type,alt_text")
	requestURL = appendQueryValue(requestURL, "user.fields", "username,name")

	var payload xBookmarkResponse
	if err := s.requestJSON(ctx, http.MethodGet, requestURL, headers, nil, &payload); err != nil {
		return nil, fmt.Errorf("X bookmarks failed: %w", err)
	}

	users := map[string]struct {
		Name     string
		Username string
	}{}
	if payload.Includes != nil {
		for _, user := range payload.Includes.Users {
			users[user.ID] = struct {
				Name     string
				Username string
			}{Name: user.Name, Username: user.Username}
		}
	}

	bookmarks := make([]XBookmark, 0, min(limit, len(payload.Data)))
	for i, tweet := range payload.Data {
		if i >= limit {
			break
		}
		user := users[tweet.AuthorID]
		body := tweet.Text
		contentType := "tweet"
		title := ""
		preview := ""
		if tweet.Article != nil && tweet.Article.PlainText != "" {
			contentType = "article"
			title = tweet.Article.Title
			body = tweet.Article.PlainText
			preview = tweet.Article.PreviewText
		}

		expandedURL := ""
		if tweet.Entities != nil {
			for _, item := range tweet.Entities.URLs {
				if item.UnwoundURL != "" {
					expandedURL = item.UnwoundURL
					break
				}
				if item.ExpandedURL != "" {
					expandedURL = item.ExpandedURL
					break
				}
			}
		}

		sourceURL := "https://x.com/i/web/status/" + tweet.ID
		if user.Username != "" && contentType == "article" {
			sourceURL = "https://x.com/" + user.Username + "/article/" + tweet.ID
		} else if user.Username != "" {
			sourceURL = "https://x.com/" + user.Username + "/status/" + tweet.ID
		}

		bookmarks = append(bookmarks, XBookmark{
			ID:            tweet.ID,
			ContentType:   contentType,
			Text:          tweet.Text,
			Title:         title,
			Body:          body,
			PreviewText:   preview,
			ExpandedURL:   expandedURL,
			AuthorID:      tweet.AuthorID,
			AuthorName:    user.Name,
			Username:      user.Username,
			CreatedAt:     tweet.CreatedAt,
			PublicMetrics: tweet.PublicMetrics,
			SourceURL:     sourceURL,
		})
	}

	return bookmarks, nil
}

func (s *Service) refreshXAccessToken(ctx context.Context) (string, error) {
	if strings.TrimSpace(s.cfg.XClientID) == "" || strings.TrimSpace(s.cfg.XClientSecret) == "" {
		s.logger.Warn("x token refresh skipped", "reason", "X_CLIENT_ID or X_CLIENT_SECRET missing")
		return strings.TrimSpace(os.Getenv("X_USER_ACCESS_TOKEN")), nil
	}

	start := time.Now()
	body := "grant_type=refresh_token"
	if refreshToken := strings.TrimSpace(os.Getenv("X_REFRESH_TOKEN")); refreshToken != "" {
		body += "&refresh_token=" + refreshToken
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(s.cfg.XClientID+":"+s.cfg.XClientSecret)))

	var payload xTokenResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.x.com/2/oauth2/token", headers, strings.NewReader(body), &payload); err != nil {
		return "", fmt.Errorf("X token refresh failed: %w", err)
	}
	if payload.Error != "" {
		return "", fmt.Errorf("X token refresh failed: %s %s", payload.Error, payload.ErrorDetail)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", fmt.Errorf("X token refresh returned no access token")
	}
	if strings.TrimSpace(payload.RefreshToken) == "" {
		return "", fmt.Errorf("X token refresh returned no rotated refresh token")
	}
	if err := s.rotateOneCLIXSecrets(ctx, payload.AccessToken, payload.RefreshToken); err != nil {
		return "", err
	}
	s.logger.Info("x token refresh completed", "duration_ms", time.Since(start).Milliseconds(), "expires_in", payload.ExpiresIn, "scope", payload.Scope)
	return payload.AccessToken, nil
}

func (s *Service) rotateOneCLIXSecrets(ctx context.Context, accessToken string, refreshToken string) error {
	if !s.cfg.OneCLIGateway {
		return nil
	}
	if s.cfg.OneCLIXAccessSecretID == "" || s.cfg.OneCLIXRefreshSecretID == "" {
		return fmt.Errorf("OneCLI X secret IDs are not configured")
	}
	if err := s.updateOneCLISecret(ctx, s.cfg.OneCLIXAccessSecretID, accessToken); err != nil {
		return fmt.Errorf("update OneCLI X access token secret: %w", err)
	}
	if err := s.updateOneCLISecret(ctx, s.cfg.OneCLIXRefreshSecretID, refreshToken); err != nil {
		return fmt.Errorf("update OneCLI X refresh token secret: %w", err)
	}
	return nil
}

func (s *Service) updateOneCLISecret(ctx context.Context, id string, value string) error {
	updateCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(updateCtx, s.cfg.OneCLIBin, "secrets", "update", "--id", id, "--value", value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	var response struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output, &response); err != nil && len(output) > 0 {
		return fmt.Errorf("decode onecli response: %w", err)
	}
	if response.Status != "" && response.Status != "updated" {
		return fmt.Errorf("unexpected onecli status %q", response.Status)
	}
	return nil
}

func xAccessHeaders(accessToken string) http.Header {
	if accessToken != "" {
		header := http.Header{}
		header.Set("Authorization", "Bearer "+accessToken)
		return header
	}
	return authHeader("X_USER_ACCESS_TOKEN", "Bearer {value}")
}
