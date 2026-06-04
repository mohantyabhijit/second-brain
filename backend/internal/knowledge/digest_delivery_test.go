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
		DigestDate:      "2026-05-23",
		ScheduledFor:    time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		IdempotencyKey:  "daily:2026-05-23",
		Subject:         "Second Brain digest",
		BodyMarkdown:    "# Digest",
		Status:          "generated",
		IllustrationURL: "https://example.com/second-brain/api/digests/digest-1/illustration",
		IllustrationAlt: "Black-and-white sketch",
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
		if !strings.Contains(htmlBody, `<img src="https://example.com/second-brain/api/digests/digest-1/illustration"`) {
			t.Fatalf("expected digest illustration image, got %s", htmlBody)
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

func TestBuildDigestIssueCreatesEnvelopeOnly(t *testing.T) {
	digest := buildDigestIssue(
		"Asia/Singapore",
		"18:00",
		time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC),
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	if digest.DigestDate != "2026-05-23" {
		t.Fatalf("expected digest date, got %#v", digest)
	}
	if digest.Subject != "Abhijit's Second Brain" || digest.Status != "generated" {
		t.Fatalf("expected generated envelope, got %#v", digest)
	}
	if digest.BodyMarkdown != "" {
		t.Fatalf("expected no deterministic fallback body, got %s", digest.BodyMarkdown)
	}
}

func TestBuildDigestIssueDoesNotFallbackToInsights(t *testing.T) {
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

	if digest.BodyMarkdown != "" {
		t.Fatalf("expected OpenAI-only digest body generation, got %s", digest.BodyMarkdown)
	}
}

func TestEnsureDigestSourceLinksKeepsNarrativeFormat(t *testing.T) {
	body := "# Abhijit's Second Brain - 2026-05-24\n\n## The Lead\n\n- A useful point about systems."
	result := ensureDigestSourceLinks(body, []Insight{{
		Title:     "Original idea",
		SourceURL: "https://example.com/original",
	}})

	for _, banned := range []string{"##", "The Lead", "Sources"} {
		if strings.Contains(result, banned) {
			t.Fatalf("expected narrative-only digest body without %q, got %s", banned, result)
		}
	}
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			t.Fatalf("expected narrative-only digest body without bullet lines, got %s", result)
		}
	}
	if !strings.Contains(result, "[Original idea](https://example.com/original)") {
		t.Fatalf("expected missing source to be linked naturally, got %s", result)
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

type digestSourceStore struct {
	cacheStore
	refs []DigestSourceRef
}

func (s digestSourceStore) ReadNewDigestSources(ctx context.Context, ownerID string, promptVersion string, model string) ([]DigestSourceRef, error) {
	return s.refs, nil
}

type noNewDigestSourceStore struct {
	cacheStore
	latest       *Result
	saved        *DigestIssue
	readNewCalls int
}

func (s *noNewDigestSourceStore) ReadLatest(ctx context.Context) (*Result, error) {
	return s.latest, nil
}

func (s *noNewDigestSourceStore) ReadNewDigestSources(ctx context.Context, ownerID string, promptVersion string, model string) ([]DigestSourceRef, error) {
	s.readNewCalls++
	return nil, nil
}

func (s *noNewDigestSourceStore) SaveDigest(ctx context.Context, digest DigestIssue) (*DigestIssue, error) {
	s.saved = &digest
	return &digest, nil
}

func TestGenerateDigestFallsBackToLatestRunWhenNoNewDigestSources(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("RESEND_API_KEY", "resend-key")
	store := &noNewDigestSourceStore{latest: &Result{
		GeneratedAt: time.Now().UTC().Add(-time.Hour),
		Summaries: []Summary{{
			ID:         "source-1",
			Source:     "x",
			Title:      "Saved source",
			SourceURL:  "https://x.com/example/status/source-1",
			Summary:    "A useful saved idea.",
			Confidence: "high",
		}},
		Insights: []Insight{{
			ID:         "insight-1",
			Source:     "x",
			SourceID:   "source-1",
			Title:      "Saved insight",
			Insight:    "A reusable insight from the saved source.",
			Evidence:   "The source supports it.",
			SourceURL:  "https://x.com/example/status/source-1",
			Confidence: "high",
		}},
		Digest: &DigestIssue{
			DigestDate:   "2026-05-31",
			ScheduledFor: time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC),
			Status:       "sent",
			BodyMarkdown: "# Previous digest",
		},
	}}
	requests := []string{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.String())
		switch request.URL.String() {
		case "https://api.openai.com/v1/responses":
			return jsonResponse(`{"output_text":"{\"subject\":\"Continuity digest\",\"body_markdown\":\"# Continuity digest\\n\\nUse the latest saved inputs.\"}"}`), nil
		case "https://api.openai.com/v1/images/generations":
			return jsonResponse(`{"data":[{"b64_json":"aGVsbG8="}]}`), nil
		case "https://api.resend.com/emails":
			return jsonResponse(`{"id":"email-continuity"}`), nil
		default:
			t.Fatalf("unexpected URL: %s", request.URL.String())
			return nil, nil
		}
	})}
	service := NewService(config.Config{
		DigestEmailFrom:      "Second Brain <digest@example.com>",
		DigestEmailTo:        "abhijit@example.com",
		DigestTimezone:       "UTC",
		DigestTime:           "18:00",
		OpenAISynthesisModel: "gpt-5.5",
		OpenAIImageModel:     "gpt-image-1",
		ResendAPIKey:         "resend-key",
	}, store, client)

	digest, err := service.GenerateDigest(context.Background())
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}
	if digest.Status != "sent" || digest.Subject != "Continuity digest" {
		t.Fatalf("expected sent continuity digest, got %#v", digest)
	}
	if store.saved == nil {
		t.Fatal("expected digest to be saved")
	}
	if len(store.saved.SourceRefs) != 0 {
		t.Fatalf("expected continuity digest not to claim new source refs, got %#v", store.saved.SourceRefs)
	}
	if !slices.Contains(requests, "https://api.openai.com/v1/responses") ||
		!slices.Contains(requests, "https://api.openai.com/v1/images/generations") ||
		!slices.Contains(requests, "https://api.resend.com/emails") {
		t.Fatalf("expected synthesis, illustration, and delivery requests, got %#v", requests)
	}
}

func TestGenerateDigestDoesNotResendAlreadySentDigestForToday(t *testing.T) {
	digestDate := digestDateFor("UTC", "18:00", time.Now().UTC())
	store := &noNewDigestSourceStore{latest: &Result{
		GeneratedAt: time.Now().UTC(),
		Digest: &DigestIssue{
			ID:           "digest-today",
			DigestDate:   digestDate,
			Status:       "sent",
			Subject:      "Already sent",
			BodyMarkdown: "# Already sent",
		},
	}}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("expected already-sent digest to avoid outbound request, got %s", request.URL.String())
		return nil, nil
	})}
	service := NewService(config.Config{DigestTimezone: "UTC", DigestTime: "18:00"}, store, client)

	digest, err := service.GenerateDigest(context.Background())
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}
	if digest.ID != "digest-today" {
		t.Fatalf("expected existing digest, got %#v", digest)
	}
	if store.saved != nil {
		t.Fatal("expected no duplicate save for already-sent digest")
	}
	if store.readNewCalls != 0 {
		t.Fatalf("expected no new-source lookup, got %d", store.readNewCalls)
	}
}

func TestDigestInputsUseOnlyNewSourceRefs(t *testing.T) {
	generatedAt := time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)
	service := NewService(config.Config{}, digestSourceStore{refs: []DigestSourceRef{{
		Source:     "x",
		ExternalID: "new",
		SourceURL:  "https://x.com/example/status/new",
		Title:      "New source",
	}}}, http.DefaultClient)
	latest := &Result{
		GeneratedAt: generatedAt,
		Summaries: []Summary{
			{ID: "old", Source: "x", Title: "Old source", SourceURL: "https://x.com/example/status/old"},
			{ID: "new", Source: "x", Title: "New source", SourceURL: "https://x.com/example/status/new"},
		},
		Insights: []Insight{
			{ID: "old-insight", Source: "x", SourceID: "old", Title: "Old source", SourceURL: "https://x.com/example/status/old"},
			{ID: "new-insight", Source: "x", SourceID: "new", Title: "New source", SourceURL: "https://x.com/example/status/new"},
		},
		Themes: []ThemeCluster{{
			ID:      "theme-new",
			Label:   "New",
			Sources: []string{"x:old", "x:new"},
		}},
		InsightClusters: []InsightCluster{{
			ID:                       "cluster-new",
			InsightIDs:               []string{"old-insight", "new-insight"},
			RepresentativeInsightIDs: []string{"old-insight", "new-insight"},
		}},
		Connections: []SourceConnection{{
			ID:            "connection-old-new",
			LeftSourceID:  "x:old",
			RightSourceID: "x:new",
		}},
	}

	refs, summaries, insights, themes, clusters, connections, err := service.digestInputsForLatest(context.Background(), latest)
	if err != nil {
		t.Fatalf("filter digest inputs: %v", err)
	}
	if len(refs) != 1 || refs[0].ExternalID != "new" {
		t.Fatalf("expected only the new source ref, got %#v", refs)
	}
	if len(summaries) != 1 || summaries[0].ID != "new" {
		t.Fatalf("expected only the new summary, got %#v", summaries)
	}
	if len(insights) != 1 || insights[0].ID != "new-insight" {
		t.Fatalf("expected only the new insight, got %#v", insights)
	}
	if len(themes) != 1 || !slices.Equal(themes[0].Sources, []string{"x:new"}) {
		t.Fatalf("expected theme sources to be narrowed to new sources, got %#v", themes)
	}
	if len(clusters) != 1 || !slices.Equal(clusters[0].InsightIDs, []string{"new-insight"}) {
		t.Fatalf("expected insight cluster to be narrowed to new insights, got %#v", clusters)
	}
	if len(connections) != 0 {
		t.Fatalf("expected cross-source connection with old source to be dropped, got %#v", connections)
	}
}

func TestNewDigestSourceRefsFromLatestUsesLastDigestCutoff(t *testing.T) {
	cutoff := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	oldGeneratedAt := cutoff.Add(-time.Hour)
	newGeneratedAt := cutoff.Add(time.Hour)
	latest := &Result{
		GeneratedAt: newGeneratedAt,
		Digest: &DigestIssue{
			ScheduledFor: cutoff,
		},
		Summaries: []Summary{
			{ID: "old", Source: "x", Title: "Old source", GeneratedAt: &oldGeneratedAt},
			{ID: "new", Source: "x", Title: "New source", GeneratedAt: &newGeneratedAt},
		},
	}

	refs := newDigestSourceRefsFromLatest(latest)
	if len(refs) != 1 || refs[0].ExternalID != "new" {
		t.Fatalf("expected only sources generated after last digest, got %#v", refs)
	}
}
