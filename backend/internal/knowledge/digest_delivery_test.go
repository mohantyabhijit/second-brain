package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
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

	delivery := service.deliverDigest(context.Background(), digest, "")

	if delivery.Status != "sent" || delivery.ProviderMessageID != "email-1" {
		t.Fatalf("expected sent delivery, got %#v", delivery)
	}
}

func TestDeliverDigestUsesRecipientOverride(t *testing.T) {
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
		idempotencyKey := request.Header.Get("Idempotency-Key")
		if idempotencyKey == digest.IdempotencyKey || !strings.Contains(idempotencyKey, ":manual:") {
			t.Fatalf("expected one-off manual idempotency key, got %q", idempotencyKey)
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload struct {
			To []string `json:"to"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if !slices.Equal(payload.To, []string{"reader@example.com"}) {
			t.Fatalf("expected override recipient, got %#v", payload.To)
		}
		return jsonResponse(`{"id":"email-2"}`), nil
	})}
	service := NewService(config.Config{
		DigestEmailFrom: "Second Brain <digest@example.com>",
		ResendAPIKey:    "resend-key",
	}, cacheStore{}, client)

	delivery := service.deliverDigest(context.Background(), digest, "reader@example.com")

	if delivery.Status != "sent" || delivery.Recipient != "reader@example.com" {
		t.Fatalf("expected sent override delivery, got %#v", delivery)
	}
}

func TestSendProvidedDigestDoesNotRequireStoreRead(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "resend-key")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var payload struct {
			To []string `json:"to"`
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if !slices.Equal(payload.To, []string{"reader@example.com"}) {
			t.Fatalf("expected provided digest recipient, got %#v", payload.To)
		}
		return jsonResponse(`{"id":"email-provided"}`), nil
	})}
	service := NewService(config.Config{
		DigestEmailFrom: "Second Brain <digest@example.com>",
		ResendAPIKey:    "resend-key",
	}, cacheStore{}, client)

	digest, err := service.SendProvidedDigest(context.Background(), "reader@example.com", DigestIssue{
		DigestDate:   "2026-05-24",
		Subject:      "Displayed digest",
		BodyMarkdown: "# Displayed digest",
	})
	if err != nil {
		t.Fatalf("send provided digest: %v", err)
	}
	if digest.Status != "sent" || len(digest.Deliveries) != 1 || digest.Deliveries[0].ProviderMessageID != "email-provided" {
		t.Fatalf("expected sent provided digest, got %#v", digest)
	}
}

func TestBuildDigestIssueKeepsEmailReadable(t *testing.T) {
	longEvidence := strings.Repeat("very long transcript evidence ", 40)
	digest := buildDigestIssue(
		"Asia/Singapore",
		"18:00",
		time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		[]Summary{{
			Title:     "Long source",
			SourceURL: "https://example.com/source",
			Summary:   strings.Repeat("summary sentence ", 40),
			Quote:     longEvidence,
		}},
		nil,
		[]ThemeCluster{{
			Label:    "Money",
			Score:    2,
			Evidence: longEvidence,
		}},
		nil,
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

func TestBuildDigestIssueUsesFiveInsights(t *testing.T) {
	insights := make([]Insight, 0, 8)
	for index := 0; index < 8; index++ {
		insights = append(insights, Insight{
			ID:        string(rune('a' + index)),
			Source:    "x",
			Title:     "Insight title",
			Insight:   "A useful pattern worth keeping short.",
			Evidence:  "Saved source evidence.",
			SourceURL: "https://example.com/source",
		})
	}

	digest := buildDigestIssue(
		"Asia/Singapore",
		"18:00",
		time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		nil,
		insights,
		nil,
		nil,
		nil,
	)

	if got := strings.Count(digest.BodyMarkdown, "**[Insight title]("); got != 5 {
		t.Fatalf("expected five linked insights, got %d in %s", got, digest.BodyMarkdown)
	}
	if strings.Contains(digest.BodyMarkdown, "What To Read") {
		t.Fatalf("expected insight-only digest when insights exist, got %s", digest.BodyMarkdown)
	}
}

func TestDigestInsightSelectionIncludesYouTubeWhenAvailable(t *testing.T) {
	insights := []Insight{
		{ID: "x-1", Source: "x", Title: "X 1", Insight: "X insight", SourceURL: "https://x.com/1"},
		{ID: "x-2", Source: "x", Title: "X 2", Insight: "X insight", SourceURL: "https://x.com/2"},
		{ID: "x-3", Source: "x", Title: "X 3", Insight: "X insight", SourceURL: "https://x.com/3"},
		{ID: "x-4", Source: "x", Title: "X 4", Insight: "X insight", SourceURL: "https://x.com/4"},
		{ID: "x-5", Source: "x", Title: "X 5", Insight: "X insight", SourceURL: "https://x.com/5"},
		{ID: "youtube-1", Source: "youtube", Title: "YouTube", Insight: "YouTube insight", SourceURL: "https://youtube.com/watch?v=1"},
	}

	selected := selectDigestInsights(time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC), insights, 5)

	if len(selected) != 5 {
		t.Fatalf("expected five selected insights, got %d", len(selected))
	}
	if !slices.ContainsFunc(selected, func(insight Insight) bool { return insight.Source == "youtube" }) {
		t.Fatalf("expected selected insights to include YouTube when available: %#v", selected)
	}
	if !slices.ContainsFunc(selected, func(insight Insight) bool { return insight.Source == "x" }) {
		t.Fatalf("expected selected insights to include X when available: %#v", selected)
	}
}
