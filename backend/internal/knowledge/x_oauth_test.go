package knowledge

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestGetValidXAccessTokenUsesStoredTokenBeforeRefreshWindow(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	store := &xTokenTestStore{}
	service := NewService(config.Config{
		OwnerID:             "00000000-0000-0000-0000-000000000001",
		XTokenEncryptionKey: key,
	}, store, http.DefaultClient)
	if err := service.saveXTokenSet(context.Background(), XTokenSet{
		AccessToken:     "stored-access",
		RefreshToken:    "stored-refresh",
		AccessExpiresAt: time.Now().UTC().Add(time.Hour),
		UpdatedAt:       time.Now().UTC(),
	}, nil); err != nil {
		t.Fatalf("save token set: %v", err)
	}

	token, ok, err := service.getValidXAccessToken(context.Background())
	if err != nil {
		t.Fatalf("get valid token: %v", err)
	}
	if !ok || token != "stored-access" {
		t.Fatalf("expected stored token, ok=%v token=%q", ok, token)
	}
	if strings.Contains(store.tokens.AccessTokenCiphertext, "stored-access") || strings.Contains(store.tokens.RefreshTokenCiphertext, "stored-refresh") {
		t.Fatalf("expected stored tokens to be encrypted, got %#v", store.tokens)
	}
}

func TestGetValidXAccessTokenRefreshesAndPersistsRotatedToken(t *testing.T) {
	t.Setenv("SECOND_BRAIN_SKIP_KEYCHAIN", "true")
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	store := &xTokenTestStore{}
	requestBody := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(raw)
		return jsonResponse(`{"access_token":"rotated-access","refresh_token":"rotated-refresh","expires_in":7200,"scope":"tweet.read offline.access"}`), nil
	})}
	service := NewService(config.Config{
		OwnerID:             "00000000-0000-0000-0000-000000000001",
		XClientID:           "client-id",
		XClientSecret:       "client-secret",
		XTokenEncryptionKey: key,
	}, store, client)
	if err := service.saveXTokenSet(context.Background(), XTokenSet{
		AccessToken:     "expired-access",
		RefreshToken:    "stored refresh",
		AccessExpiresAt: time.Now().UTC().Add(time.Minute),
		UpdatedAt:       time.Now().UTC(),
	}, nil); err != nil {
		t.Fatalf("save token set: %v", err)
	}

	token, ok, err := service.getValidXAccessToken(context.Background())
	if err != nil {
		t.Fatalf("get valid token: %v", err)
	}
	if !ok || token != "rotated-access" {
		t.Fatalf("expected rotated token, ok=%v token=%q", ok, token)
	}
	if !strings.Contains(requestBody, "refresh_token=stored+refresh") {
		t.Fatalf("expected stored refresh token in refresh request, got %s", requestBody)
	}
	saved, err := service.readStoredXTokenSet(context.Background())
	if err != nil {
		t.Fatalf("read saved token set: %v", err)
	}
	if saved.RefreshToken != "rotated-refresh" {
		t.Fatalf("expected persisted rotated refresh token, got %q", saved.RefreshToken)
	}
}

type xTokenTestStore struct {
	cacheStore
	tokens *EncryptedXTokens
}

func (s *xTokenTestStore) ReadXTokens(ctx context.Context, ownerID string) (*EncryptedXTokens, error) {
	return s.tokens, nil
}

func (s *xTokenTestStore) SaveXTokens(ctx context.Context, tokens EncryptedXTokens) error {
	copy := tokens
	s.tokens = &copy
	return nil
}
