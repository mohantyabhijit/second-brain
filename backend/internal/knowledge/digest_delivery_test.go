package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestDeliverDigestUsesResendIdempotencyHeader(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "resend-key")
	digest := DigestIssue{
		DigestDate:     "2026-05-23",
		ScheduledFor:   time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		IdempotencyKey: "daily:2026-05-23",
		Subject:        "Second Brain digest",
		BodyMarkdown:   "# Digest",
		Status:         "generated",
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.resend.com/emails" {
			t.Fatalf("unexpected URL: %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer resend-key" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("Idempotency-Key") != digest.IdempotencyKey {
			t.Fatalf("unexpected idempotency header: %q", request.Header.Get("Idempotency-Key"))
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		htmlBody, ok := payload["html"].(string)
		if !ok || !strings.Contains(htmlBody, "<h1") {
			t.Fatalf("expected html payload with heading, got %#v", payload["html"])
		}
		if strings.Contains(htmlBody, "# Digest") {
			t.Fatalf("expected rendered HTML, got raw markdown: %s", htmlBody)
		}
		if textBody, ok := payload["text"].(string); !ok || strings.Contains(textBody, "# Digest") {
			t.Fatalf("expected cleaned text fallback, got %#v", payload["text"])
		}
		return jsonResponse(`{"id":"email-1"}`), nil
	})}
	service := NewService(config.Config{
		DigestEmailFrom: "Second Brain <digest@example.com>",
		DigestEmailTo:   "abhijit@example.com",
		ResendAPIKey:    "resend-key",
	}, cacheStore{}, client)

	delivery := service.deliverDigest(context.Background(), digest)

	if delivery.Status != "sent" || delivery.ProviderMessageID != "email-1" {
		t.Fatalf("expected sent delivery, got %#v", delivery)
	}
}

func TestBuildDigestIssueKeepsEmailReadable(t *testing.T) {
	longEvidence := strings.Repeat("very long transcript evidence ", 40)
	digest := buildDigestIssue(
		"Asia/Singapore",
		time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		[]Summary{{
			Title:     "Long source",
			SourceURL: "https://example.com/source",
			Summary:   strings.Repeat("summary sentence ", 40),
			Quote:     longEvidence,
		}},
		[]ThemeCluster{{
			Label:    "Money",
			Score:    2,
			Evidence: longEvidence,
		}},
		nil,
	)

	if strings.Contains(digest.BodyMarkdown, strings.Repeat("very long transcript evidence ", 10)) {
		t.Fatalf("digest contains unbounded evidence: %s", digest.BodyMarkdown)
	}
	if !strings.Contains(digest.BodyMarkdown, "...") {
		t.Fatalf("expected truncated digest copy, got %s", digest.BodyMarkdown)
	}
	htmlBody := digestHTML(digest)
	if !strings.Contains(htmlBody, `<a href="https://example.com/source"`) {
		t.Fatalf("expected rendered source link, got %s", htmlBody)
	}
}
