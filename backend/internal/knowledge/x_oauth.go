package knowledge

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	xAuthorizeURL       = "https://x.com/i/oauth2/authorize"
	xTokenURL           = "https://api.x.com/2/oauth2/token"
	xRefreshLeadTime    = 10 * time.Minute
	xOAuthStateTTL      = 10 * time.Minute
	defaultSessionHours = 1
)

var errXTokenStoreUnavailable = errors.New("x token store unavailable")

type XTokenSet struct {
	AccessToken     string
	RefreshToken    string
	AccessExpiresAt time.Time
	Scope           string
	TokenType       string
	UpdatedAt       time.Time
}

type EncryptedXTokens struct {
	OwnerID                string
	AccessTokenCiphertext  string
	RefreshTokenCiphertext string
	AccessExpiresAt        time.Time
	Scope                  string
	TokenType              string
	AuthenticatedXUserID   string
	AuthenticatedXUsername string
	AuthenticatedXName     string
	UpdatedAt              time.Time
}

type XAuthStatus struct {
	Configured      bool       `json:"configured"`
	Authorized      bool       `json:"authorized"`
	Username        string     `json:"username,omitempty"`
	AccessExpiresAt *time.Time `json:"accessExpiresAt,omitempty"`
	UpdatedAt       *time.Time `json:"updatedAt,omitempty"`
}

type XOAuthResult struct {
	Profile         XAuthenticatedProfile
	AccessExpiresAt time.Time
}

type xOAuthState struct {
	Verifier  string
	CreatedAt time.Time
}

func (s *Service) BeginXOAuth(ctx context.Context) (string, error) {
	if err := s.validateXOAuthConfig(); err != nil {
		return "", err
	}
	state, err := randomBase64URL(32)
	if err != nil {
		return "", err
	}
	verifier := oauth2.GenerateVerifier()
	now := time.Now().UTC()

	s.xOAuthMu.Lock()
	s.pruneExpiredXOAuthStatesLocked(now)
	s.xOAuthStates[state] = xOAuthState{Verifier: verifier, CreatedAt: now}
	s.xOAuthMu.Unlock()

	config := s.xOAuthConfig()
	return config.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
	), nil
}

func (s *Service) CompleteXOAuth(ctx context.Context, state string, code string) (*XOAuthResult, error) {
	if err := s.validateXOAuthConfig(); err != nil {
		return nil, err
	}
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	if state == "" || code == "" {
		return nil, fmt.Errorf("OAuth callback requires state and code")
	}
	stored, ok := s.consumeXOAuthState(state)
	if !ok {
		return nil, fmt.Errorf("OAuth state is missing or expired")
	}

	config := s.xOAuthConfig()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, s.client)
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(stored.Verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange X OAuth code: %w", err)
	}
	if strings.TrimSpace(token.AccessToken) == "" || strings.TrimSpace(token.RefreshToken) == "" {
		return nil, fmt.Errorf("X token response did not include both access_token and refresh_token; confirm offline.access is enabled")
	}

	profile, err := s.fetchXAuthenticatedProfile(ctx, token.AccessToken)
	if err != nil {
		return nil, err
	}
	tokenSet := XTokenSet{
		AccessToken:     token.AccessToken,
		RefreshToken:    token.RefreshToken,
		AccessExpiresAt: token.Expiry,
		TokenType:       token.TokenType,
		UpdatedAt:       time.Now().UTC(),
	}
	if scopes := token.Extra("scope"); scopes != nil {
		tokenSet.Scope = fmt.Sprint(scopes)
	}
	if tokenSet.AccessExpiresAt.IsZero() {
		tokenSet.AccessExpiresAt = time.Now().UTC().Add(2 * time.Hour)
	}
	if err := s.saveXTokenSet(ctx, tokenSet, profile); err != nil {
		return nil, err
	}
	if err := s.recordXTokenRotation(ctx, xTokenResponse{
		TokenType:    tokenSet.TokenType,
		ExpiresIn:    secondsUntil(tokenSet.UpdatedAt, tokenSet.AccessExpiresAt),
		AccessToken:  tokenSet.AccessToken,
		RefreshToken: tokenSet.RefreshToken,
		Scope:        tokenSet.Scope,
	}, tokenSet.UpdatedAt); err != nil {
		s.logger.Warn("record X OAuth authorization metadata failed", "error", err)
	}
	if err := persistXTokensToKeychain(ctx, s.cfg.XKeychainTokenSuffix, tokenSet.AccessToken, tokenSet.RefreshToken); err != nil {
		s.logger.Warn("persist X OAuth tokens to Keychain failed", "error", err)
	}
	if err := s.rotateOneCLIXSecrets(ctx, tokenSet.AccessToken, tokenSet.RefreshToken); err != nil {
		s.logger.Warn("persist X OAuth tokens to OneCLI failed", "error", err)
	}
	return &XOAuthResult{Profile: *profile, AccessExpiresAt: tokenSet.AccessExpiresAt}, nil
}

func (s *Service) XAuthStatus(ctx context.Context) XAuthStatus {
	status := XAuthStatus{
		Configured: strings.TrimSpace(s.cfg.XClientID) != "" &&
			strings.TrimSpace(s.cfg.XClientSecret) != "" &&
			strings.TrimSpace(s.cfg.XRedirectURI) != "" &&
			strings.TrimSpace(s.cfg.XTokenEncryptionKey) != "",
	}
	record, err := s.store.ReadXTokens(ctx, s.cfg.OwnerID)
	if err != nil || record == nil {
		return status
	}
	status.Authorized = true
	status.Username = record.AuthenticatedXUsername
	status.AccessExpiresAt = &record.AccessExpiresAt
	status.UpdatedAt = &record.UpdatedAt
	return status
}

func (s *Service) getValidXAccessToken(ctx context.Context) (string, bool, error) {
	tokenSet, err := s.readStoredXTokenSet(ctx)
	if err != nil {
		return "", false, err
	}
	if tokenSet == nil {
		return "", false, nil
	}
	if time.Now().UTC().Before(tokenSet.AccessExpiresAt.Add(-xRefreshLeadTime)) {
		return tokenSet.AccessToken, true, nil
	}
	refreshed, err := s.refreshXTokenSet(ctx, tokenSet.RefreshToken)
	if err != nil {
		return "", true, err
	}
	if err := s.saveXTokenSet(ctx, refreshed, nil); err != nil {
		return "", true, err
	}
	if err := persistXTokensToKeychain(ctx, s.cfg.XKeychainTokenSuffix, refreshed.AccessToken, refreshed.RefreshToken); err != nil {
		s.logger.Warn("persist stored X token refresh to Keychain failed", "error", err)
	}
	if err := s.rotateOneCLIXSecrets(ctx, refreshed.AccessToken, refreshed.RefreshToken); err != nil {
		s.logger.Warn("persist stored X token refresh to OneCLI failed", "error", err)
	}
	return refreshed.AccessToken, true, nil
}

func (s *Service) refreshXTokenSet(ctx context.Context, refreshToken string) (XTokenSet, error) {
	clientID := strings.TrimSpace(s.cfg.XClientID)
	if clientID == "" || strings.TrimSpace(refreshToken) == "" {
		return XTokenSet{}, fmt.Errorf("X_CLIENT_ID and stored X refresh token are required")
	}
	payload, err := s.requestXTokenRefresh(ctx, refreshToken)
	if err != nil {
		if xTokenRefreshNeedsReauthorization(err) {
			return XTokenSet{}, s.xReauthorizationError()
		}
		return XTokenSet{}, fmt.Errorf("X token refresh failed: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" {
		return XTokenSet{}, fmt.Errorf("X token refresh did not return rotated access and refresh tokens")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Hour)
	if payload.ExpiresIn > 0 {
		expiresAt = now.Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	if err := s.recordXTokenRotation(ctx, payload, now); err != nil {
		s.logger.Warn("record X token rotation metadata failed", "error", err)
	}
	return XTokenSet{
		AccessToken:     strings.TrimSpace(payload.AccessToken),
		RefreshToken:    strings.TrimSpace(payload.RefreshToken),
		AccessExpiresAt: expiresAt,
		Scope:           strings.TrimSpace(payload.Scope),
		TokenType:       strings.TrimSpace(payload.TokenType),
		UpdatedAt:       now,
	}, nil
}

func (s *Service) saveXTokenSet(ctx context.Context, tokenSet XTokenSet, profile *XAuthenticatedProfile) error {
	accessCiphertext, err := s.encryptXToken(tokenSet.AccessToken)
	if err != nil {
		return err
	}
	refreshCiphertext, err := s.encryptXToken(tokenSet.RefreshToken)
	if err != nil {
		return err
	}
	record := EncryptedXTokens{
		OwnerID:                s.cfg.OwnerID,
		AccessTokenCiphertext:  accessCiphertext,
		RefreshTokenCiphertext: refreshCiphertext,
		AccessExpiresAt:        tokenSet.AccessExpiresAt.UTC(),
		Scope:                  tokenSet.Scope,
		TokenType:              tokenSet.TokenType,
		UpdatedAt:              tokenSet.UpdatedAt.UTC(),
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	if profile != nil {
		record.AuthenticatedXUserID = profile.ID
		record.AuthenticatedXUsername = profile.Username
		record.AuthenticatedXName = profile.Name
	}
	return s.store.SaveXTokens(ctx, record)
}

func (s *Service) readStoredXTokenSet(ctx context.Context) (*XTokenSet, error) {
	record, err := s.store.ReadXTokens(ctx, s.cfg.OwnerID)
	if err != nil {
		if errors.Is(err, errXTokenStoreUnavailable) {
			return nil, nil
		}
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	accessToken, err := s.decryptXToken(record.AccessTokenCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt stored X access token: %w", err)
	}
	refreshToken, err := s.decryptXToken(record.RefreshTokenCiphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt stored X refresh token: %w", err)
	}
	return &XTokenSet{
		AccessToken:     accessToken,
		RefreshToken:    refreshToken,
		AccessExpiresAt: record.AccessExpiresAt,
		Scope:           record.Scope,
		TokenType:       record.TokenType,
		UpdatedAt:       record.UpdatedAt,
	}, nil
}

func (s *Service) encryptXToken(value string) (string, error) {
	key, err := xTokenEncryptionKey(s.cfg.XTokenEncryptionKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func (s *Service) decryptXToken(value string) (string, error) {
	key, err := xTokenEncryptionKey(s.cfg.XTokenEncryptionKey)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext is too short")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func xTokenEncryptionKey(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("X_TOKEN_ENCRYPTION_KEY is required to store shared X OAuth tokens")
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, fmt.Errorf("X_TOKEN_ENCRYPTION_KEY must be 32 raw bytes or a base64-encoded 32-byte key")
}

func secondsUntil(start time.Time, end time.Time) int {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0
	}
	return int(end.Sub(start).Seconds())
}

func (s *Service) validateXOAuthConfig() error {
	missing := []string{}
	for key, value := range map[string]string{
		"X_CLIENT_ID":            s.cfg.XClientID,
		"X_REDIRECT_URI":         s.cfg.XRedirectURI,
		"X_TOKEN_ENCRYPTION_KEY": s.cfg.XTokenEncryptionKey,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if strings.TrimSpace(s.cfg.XClientSecret) == "" && !s.cfg.OneCLIGateway {
		missing = append(missing, "X_CLIENT_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s required for backend X OAuth", strings.Join(missing, ", "))
	}
	if _, err := xTokenEncryptionKey(s.cfg.XTokenEncryptionKey); err != nil {
		return err
	}
	return nil
}

func (s *Service) xOAuthConfig() oauth2.Config {
	authStyle := oauth2.AuthStyleInParams
	if strings.TrimSpace(s.cfg.XClientSecret) != "" {
		authStyle = oauth2.AuthStyleInHeader
	}
	return oauth2.Config{
		ClientID:     strings.TrimSpace(s.cfg.XClientID),
		ClientSecret: strings.TrimSpace(s.cfg.XClientSecret),
		RedirectURL:  strings.TrimSpace(s.cfg.XRedirectURI),
		Scopes:       s.cfg.XOAuthScopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   xAuthorizeURL,
			TokenURL:  xTokenURL,
			AuthStyle: authStyle,
		},
	}
}

func (s *Service) consumeXOAuthState(state string) (xOAuthState, bool) {
	now := time.Now().UTC()
	s.xOAuthMu.Lock()
	defer s.xOAuthMu.Unlock()
	s.pruneExpiredXOAuthStatesLocked(now)
	stored, ok := s.xOAuthStates[state]
	if ok {
		delete(s.xOAuthStates, state)
	}
	return stored, ok
}

func (s *Service) pruneExpiredXOAuthStatesLocked(now time.Time) {
	for state, stored := range s.xOAuthStates {
		if now.Sub(stored.CreatedAt) > xOAuthStateTTL {
			delete(s.xOAuthStates, state)
		}
	}
}

func randomBase64URL(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
