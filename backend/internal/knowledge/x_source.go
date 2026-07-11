package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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
	Meta *struct {
		ResultCount int    `json:"result_count"`
		NextToken   string `json:"next_token"`
	} `json:"meta"`
}

type XAuthenticatedProfile struct {
	ID       string
	Name     string
	Username string
}

type xTokenRotationRecord struct {
	RotatedAt            time.Time `json:"rotatedAt"`
	AccessTokenExpiresAt time.Time `json:"accessTokenExpiresAt,omitempty"`
	ExpiresInSeconds     int       `json:"expiresInSeconds,omitempty"`
	Scope                string    `json:"scope,omitempty"`
	TokenType            string    `json:"tokenType,omitempty"`
	KeychainTokenSuffix  string    `json:"keychainTokenSuffix,omitempty"`
	OneCLIGateway        bool      `json:"onecliGateway"`
	ExpectedUsername     string    `json:"expectedUsername,omitempty"`
	ReauthorizeCommand   string    `json:"reauthorizeCommand,omitempty"`
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

const (
	oneCLIXAccessSecretName  = "Second Brain X user access token"
	oneCLIXRefreshSecretName = "Second Brain X refresh token"
)

func (s *Service) fetchXBookmarks(ctx context.Context, limit int) ([]XBookmark, error) {
	accessToken, err := s.refreshXAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	if accessToken == "" && os.Getenv("X_USER_ACCESS_TOKEN") == "" && !s.cfg.OneCLIGateway {
		return nil, errors.New(credentialHint("X_USER_ACCESS_TOKEN"))
	}

	profile, err := s.fetchXAuthenticatedProfile(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	maxResults := 100
	if limit > 0 && limit < maxResults {
		maxResults = limit
	}

	bookmarks := make([]XBookmark, 0)
	seenTokens := map[string]bool{}
	nextToken := ""
	headers := xAccessHeaders(accessToken)
	for {
		requestURL := "https://api.x.com/2/users/" + profile.ID + "/bookmarks"
		requestURL = appendQueryValue(requestURL, "max_results", strconv.Itoa(maxResults))
		requestURL = appendQueryValue(requestURL, "tweet.fields", "article,created_at,public_metrics,author_id,entities")
		requestURL = appendQueryValue(requestURL, "expansions", "author_id,article.cover_media")
		requestURL = appendQueryValue(requestURL, "media.fields", "url,preview_image_url,type,alt_text")
		requestURL = appendQueryValue(requestURL, "user.fields", "username,name")
		requestURL = appendQueryValue(requestURL, "pagination_token", nextToken)

		var payload xBookmarkResponse
		if err := s.requestJSON(ctx, http.MethodGet, requestURL, headers, nil, &payload); err != nil {
			return nil, fmt.Errorf("X bookmarks failed: %w", err)
		}

		bookmarks = append(bookmarks, bookmarksFromXPayload(payload, remainingLimit(limit, len(bookmarks)))...)
		if limit > 0 && len(bookmarks) >= limit {
			return bookmarks[:limit], nil
		}
		if payload.Meta == nil || strings.TrimSpace(payload.Meta.NextToken) == "" {
			return bookmarks, nil
		}
		nextToken = payload.Meta.NextToken
		if seenTokens[nextToken] {
			return bookmarks, nil
		}
		seenTokens[nextToken] = true
	}
}

func (s *Service) CheckXAuth(ctx context.Context) (*XAuthenticatedProfile, error) {
	accessToken, err := s.refreshXAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	if accessToken == "" && os.Getenv("X_USER_ACCESS_TOKEN") == "" && !s.cfg.OneCLIGateway {
		return nil, errors.New(credentialHint("X_USER_ACCESS_TOKEN"))
	}
	return s.fetchXAuthenticatedProfile(ctx, accessToken)
}

func (s *Service) fetchXAuthenticatedProfile(ctx context.Context, accessToken string) (*XAuthenticatedProfile, error) {
	return s.fetchXAuthenticatedProfileWithClient(ctx, s.client, accessToken)
}

func (s *Service) fetchXAuthenticatedProfileDirect(ctx context.Context, accessToken string) (*XAuthenticatedProfile, error) {
	return s.fetchXAuthenticatedProfileWithClient(ctx, directHTTPClient(30*time.Second), accessToken)
}

func (s *Service) fetchXAuthenticatedProfileWithClient(ctx context.Context, client *http.Client, accessToken string) (*XAuthenticatedProfile, error) {
	headers := xAccessHeaders(accessToken)
	var me xUserResponse
	if err := requestJSONWithClient(ctx, client, http.MethodGet, "https://api.x.com/2/users/me?user.fields=username,name", headers, nil, &me); err != nil {
		return nil, fmt.Errorf("X /2/users/me failed: %w", err)
	}
	if me.Data == nil || me.Data.ID == "" {
		return nil, fmt.Errorf("X /2/users/me did not return an authenticated user id")
	}
	profile := &XAuthenticatedProfile{
		ID:       me.Data.ID,
		Name:     me.Data.Name,
		Username: me.Data.Username,
	}
	expectedUsername := normalizeXUsername(s.cfg.XExpectedUsername)
	if expectedUsername != "" && normalizeXUsername(profile.Username) != expectedUsername {
		return nil, fmt.Errorf("X authenticated profile mismatch: expected @%s, got @%s. Re-run npm run x:oauth with the correct X account", expectedUsername, normalizeXUsername(profile.Username))
	}
	return profile, nil
}

func bookmarksFromXPayload(payload xBookmarkResponse, limit int) []XBookmark {
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
	return bookmarks
}

func remainingLimit(limit int, count int) int {
	if limit <= 0 {
		return 100
	}
	remaining := limit - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

func normalizeXUsername(username string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(username), "@"))
}

func (s *Service) refreshXAccessToken(ctx context.Context) (string, error) {
	if accessToken, ok, err := s.getValidXAccessToken(ctx); ok || err != nil {
		return accessToken, err
	}
	if s.cfg.XRequireStoredOAuth {
		return "", s.xReauthorizationError()
	}

	clientID := strings.TrimSpace(s.cfg.XClientID)
	refreshToken := strings.TrimSpace(os.Getenv("X_REFRESH_TOKEN"))
	if clientID == "" || (refreshToken == "" && !s.cfg.OneCLIGateway) {
		s.log(ctx).Warn("x token refresh skipped", "reason", "X_CLIENT_ID or X_REFRESH_TOKEN missing")
		return strings.TrimSpace(os.Getenv("X_USER_ACCESS_TOKEN")), nil
	}

	start := time.Now()
	payload, err := s.requestXTokenRefresh(ctx, refreshToken)
	if err != nil {
		if xTokenRefreshNeedsReauthorization(err) {
			return "", s.xReauthorizationError()
		}
		if configuredXAccessTokenAvailable(s.cfg.OneCLIGateway) {
			s.log(ctx).Warn("x token refresh failed; falling back to configured X access token", "error", err)
			return strings.TrimSpace(os.Getenv("X_USER_ACCESS_TOKEN")), nil
		}
		return "", fmt.Errorf("X token refresh failed: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		if configuredXAccessTokenAvailable(s.cfg.OneCLIGateway) {
			s.log(ctx).Warn("x token refresh returned no access token; falling back to configured X access token")
			return strings.TrimSpace(os.Getenv("X_USER_ACCESS_TOKEN")), nil
		}
		return "", fmt.Errorf("X token refresh returned no access token")
	}
	if strings.TrimSpace(payload.RefreshToken) == "" {
		if configuredXAccessTokenAvailable(s.cfg.OneCLIGateway) {
			s.log(ctx).Warn("x token refresh returned no rotated refresh token; falling back to configured X access token")
			return strings.TrimSpace(os.Getenv("X_USER_ACCESS_TOKEN")), nil
		}
		return "", fmt.Errorf("X token refresh returned no rotated refresh token")
	}
	rotatedAt := time.Now().UTC()
	expiresAt := rotatedAt.Add(2 * time.Hour)
	if payload.ExpiresIn > 0 {
		expiresAt = rotatedAt.Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	if err := os.Setenv("X_USER_ACCESS_TOKEN", strings.TrimSpace(payload.AccessToken)); err != nil {
		return "", fmt.Errorf("set rotated X access token: %w", err)
	}
	if err := os.Setenv("X_REFRESH_TOKEN", strings.TrimSpace(payload.RefreshToken)); err != nil {
		return "", fmt.Errorf("set rotated X refresh token: %w", err)
	}
	if strings.TrimSpace(s.cfg.XTokenEncryptionKey) != "" {
		if err := s.saveXTokenSet(ctx, XTokenSet{
			AccessToken:     strings.TrimSpace(payload.AccessToken),
			RefreshToken:    strings.TrimSpace(payload.RefreshToken),
			AccessExpiresAt: expiresAt,
			Scope:           strings.TrimSpace(payload.Scope),
			TokenType:       strings.TrimSpace(payload.TokenType),
			UpdatedAt:       rotatedAt,
		}, nil); err != nil {
			s.log(ctx).Warn("persist rotated X tokens to shared store failed", "error", err)
		}
	}
	if err := s.recordXTokenRotation(ctx, payload, rotatedAt); err != nil {
		s.log(ctx).Warn("record X token rotation metadata failed", "error", err)
	}
	if err := persistXTokensToKeychain(ctx, s.cfg.XKeychainTokenSuffix, payload.AccessToken, payload.RefreshToken); err != nil {
		s.log(ctx).Warn("persist rotated X tokens to Keychain failed", "error", err)
	}
	if err := s.rotateOneCLIXSecrets(ctx, payload.AccessToken, payload.RefreshToken); err != nil {
		return "", err
	}
	s.log(ctx).Info("x token refresh completed", "duration_ms", time.Since(start).Milliseconds(), "expires_in", payload.ExpiresIn, "scope", payload.Scope)
	return payload.AccessToken, nil
}

func (s *Service) requestXTokenRefresh(ctx context.Context, refreshToken string) (xTokenResponse, error) {
	clientID := strings.TrimSpace(s.cfg.XClientID)
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	if refreshToken != "" {
		form.Set("refresh_token", refreshToken)
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	if strings.TrimSpace(s.cfg.XClientSecret) != "" {
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(clientID+":"+strings.TrimSpace(s.cfg.XClientSecret))))
	} else {
		form.Set("client_id", clientID)
	}

	var payload xTokenResponse
	tokenClient := s.client
	if s.cfg.XTokenRefreshDirect {
		tokenClient = directHTTPClient(30 * time.Second)
	}
	if err := requestJSONWithClient(ctx, tokenClient, http.MethodPost, "https://api.x.com/2/oauth2/token", headers, strings.NewReader(form.Encode()), &payload); err != nil {
		return xTokenResponse{}, err
	}
	if payload.Error != "" {
		return xTokenResponse{}, fmt.Errorf("%s %s", payload.Error, payload.ErrorDetail)
	}
	return payload, nil
}

func (s *Service) recordXTokenRotation(ctx context.Context, payload xTokenResponse, rotatedAt time.Time) error {
	path := strings.TrimSpace(s.cfg.XTokenRotationPath)
	if path == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	expiresAt := time.Time{}
	if payload.ExpiresIn > 0 {
		expiresAt = rotatedAt.Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	record := xTokenRotationRecord{
		RotatedAt:            rotatedAt.UTC(),
		AccessTokenExpiresAt: expiresAt.UTC(),
		ExpiresInSeconds:     payload.ExpiresIn,
		Scope:                strings.TrimSpace(payload.Scope),
		TokenType:            strings.TrimSpace(payload.TokenType),
		KeychainTokenSuffix:  strings.TrimSpace(s.cfg.XKeychainTokenSuffix),
		OneCLIGateway:        s.cfg.OneCLIGateway,
		ExpectedUsername:     strings.TrimPrefix(strings.TrimSpace(s.cfg.XExpectedUsername), "@"),
		ReauthorizeCommand:   strings.TrimSpace(s.cfg.XReauthorizeCommand),
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func (s *Service) xReauthorizationError() error {
	command := strings.TrimSpace(s.cfg.XReauthorizeCommand)
	if command == "" {
		command = "npm run x:oauth"
	}
	return fmt.Errorf("X refresh token is invalid or expired. Run %s to reauthorize X bookmarks", command)
}

func xTokenRefreshNeedsReauthorization(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "invalid_request") ||
		strings.Contains(message, "invalid_grant") ||
		strings.Contains(message, "value passed for the token was invalid")
}

func persistXTokensToKeychain(ctx context.Context, tokenSuffix string, accessToken string, refreshToken string) error {
	if strings.TrimSpace(os.Getenv("SECOND_BRAIN_SKIP_KEYCHAIN")) == "true" {
		return nil
	}
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		return nil
	}
	suffix := strings.TrimSpace(tokenSuffix)
	accessService, refreshService := xTokenKeychainServices(suffix)
	if err := saveKeychainSecret(ctx, user, accessService, accessToken); err != nil {
		return err
	}
	return saveKeychainSecret(ctx, user, refreshService, refreshToken)
}

func xTokenKeychainServices(tokenSuffix string) (string, string) {
	suffix := strings.TrimSpace(tokenSuffix)
	return "second-brain/X_USER_ACCESS_TOKEN" + suffix, "second-brain/X_REFRESH_TOKEN" + suffix
}

func saveKeychainSecret(ctx context.Context, user string, service string, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	keychainCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(keychainCtx, "security", "add-generic-password", "-U", "-a", user, "-s", service, "-w", value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", service, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func configuredXAccessTokenAvailable(oneCLIGateway bool) bool {
	return strings.TrimSpace(os.Getenv("X_USER_ACCESS_TOKEN")) != "" || oneCLIGateway
}

func (s *Service) rotateOneCLIXSecrets(ctx context.Context, accessToken string, refreshToken string) error {
	if !s.cfg.OneCLIGateway {
		return nil
	}
	accessSecretID := strings.TrimSpace(s.cfg.OneCLIXAccessSecretID)
	refreshSecretID := strings.TrimSpace(s.cfg.OneCLIXRefreshSecretID)
	if accessSecretID == "" || refreshSecretID == "" {
		resolvedAccessID, resolvedRefreshID, err := s.lookupOneCLIXSecretIDs(ctx)
		if err != nil {
			return err
		}
		if accessSecretID == "" {
			accessSecretID = resolvedAccessID
		}
		if refreshSecretID == "" {
			refreshSecretID = resolvedRefreshID
		}
	}
	if accessSecretID == "" || refreshSecretID == "" {
		return fmt.Errorf("OneCLI X secret IDs are not configured")
	}
	if err := s.updateOneCLISecret(ctx, accessSecretID, accessToken); err != nil {
		return fmt.Errorf("update OneCLI X access token secret: %w", err)
	}
	if err := s.updateOneCLISecret(ctx, refreshSecretID, refreshToken); err != nil {
		return fmt.Errorf("update OneCLI X refresh token secret: %w", err)
	}
	return nil
}

func (s *Service) lookupOneCLIXSecretIDs(ctx context.Context) (string, string, error) {
	listCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(listCtx, s.cfg.OneCLIBin, "secrets", "list", "--project", s.cfg.OneCLIProject)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("list OneCLI secrets: %v: %s", err, strings.TrimSpace(string(output)))
	}
	var response struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return "", "", fmt.Errorf("decode onecli secrets list: %w", err)
	}
	accessSecretID := ""
	refreshSecretID := ""
	for _, item := range response.Data {
		switch item.Name {
		case oneCLIXAccessSecretName:
			accessSecretID = item.ID
		case oneCLIXRefreshSecretName:
			refreshSecretID = item.ID
		}
	}
	return accessSecretID, refreshSecretID, nil
}

func (s *Service) updateOneCLISecret(ctx context.Context, id string, value string) error {
	updateCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(updateCtx, s.cfg.OneCLIBin, "secrets", "update", "--id", id, "--value", value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("update OneCLI secret: %w", err)
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
