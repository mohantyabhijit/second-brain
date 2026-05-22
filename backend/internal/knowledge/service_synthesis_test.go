package knowledge

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

type cacheStore struct {
	cached map[string]SynthesisRecord
}

func (s cacheStore) ReadLatest(ctx context.Context) (*Result, error) {
	return nil, nil
}

func (s cacheStore) ReadCachedSyntheses(ctx context.Context, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error) {
	return s.cached, nil
}

func (s cacheStore) SaveRun(ctx context.Context, result Result, sources []ProcessedSource) error {
	return nil
}

func (s cacheStore) SaveFeedback(ctx context.Context, event FeedbackEvent) error {
	return nil
}

func (s cacheStore) SaveDigest(ctx context.Context, digest DigestIssue) (*DigestIssue, error) {
	return &digest, nil
}

func TestProcessSourceCandidatesUsesCachedSynthesis(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	candidate := sourceCandidate{
		sourceType:   SourceTypeX,
		externalID:   "tweet-1",
		sourceURL:    "https://x.com/example/status/tweet-1",
		title:        "Cached post",
		body:         "This source has already been synthesized.",
		artifactKind: "tweet",
		contentType:  "text/plain; charset=utf-8",
	}
	captureHash := candidate.captureHash()
	key := SynthesisCacheKey{
		SourceType:    SourceTypeX,
		ExternalID:    candidate.externalID,
		CaptureHash:   captureHash,
		PromptVersion: synthesisPromptVersion,
		Model:         extractiveSynthesisModel,
	}
	store := cacheStore{cached: map[string]SynthesisRecord{
		key.String(): {
			SourceType:    SourceTypeX,
			ExternalID:    candidate.externalID,
			CaptureHash:   captureHash,
			PromptVersion: synthesisPromptVersion,
			Model:         extractiveSynthesisModel,
			Summary: Summary{
				ID:            candidate.externalID,
				Source:        string(SourceTypeX),
				Title:         "Cached post",
				SourceURL:     candidate.sourceURL,
				Decision:      DecisionReadNow,
				Summary:       "Cached summary",
				Confidence:    "medium",
				CaptureHash:   captureHash,
				PromptVersion: synthesisPromptVersion,
				Model:         extractiveSynthesisModel,
			},
		},
	}}
	service := NewService(config.Config{SupabaseStorageBucket: "sources"}, store, http.DefaultClient)

	processed, blockers := service.processSourceCandidates(context.Background(), []sourceCandidate{candidate})
	if len(blockers) != 0 {
		t.Fatalf("unexpected blockers: %v", blockers)
	}
	if len(processed) != 1 {
		t.Fatalf("expected one processed source, got %d", len(processed))
	}
	if !processed[0].Cached {
		t.Fatal("expected synthesis cache hit")
	}
	if processed[0].Synthesis.Summary.CacheStatus != "cached" {
		t.Fatalf("expected cached status, got %q", processed[0].Synthesis.Summary.CacheStatus)
	}
	if processed[0].Artifact.Path != "" || processed[0].SummaryArtifact.Path != "" {
		t.Fatalf("expected cached source to skip artifact rewrites, got %#v %#v", processed[0].Artifact, processed[0].SummaryArtifact)
	}

	enriched := service.enrichProcessedSources(context.Background(), processed)
	if len(enriched[0].Embeddings) != 0 {
		t.Fatalf("expected cached source to skip embedding recompute, got %d embedding(s)", len(enriched[0].Embeddings))
	}
}

func TestFallbackSynthesisBuildsFirstClassInsightFields(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	candidate := sourceCandidate{
		sourceType:   SourceTypeX,
		externalID:   "tweet-2",
		sourceURL:    "https://x.com/example/status/tweet-2",
		title:        "Small teams",
		body:         "Small teams move faster because coordination overhead stays low. Adding people can slow tightly coupled projects.",
		artifactKind: "tweet",
		contentType:  "text/plain; charset=utf-8",
	}
	service := NewService(config.Config{SupabaseStorageBucket: "sources"}, cacheStore{}, http.DefaultClient)

	record := service.synthesizeCandidate(context.Background(), candidate, candidate.captureHash(), "generated")
	if len(record.Insights) == 0 {
		t.Fatal("expected fallback synthesis to produce insights")
	}
	insight := record.Insights[0]
	if insight.RawInsight == "" || insight.CanonicalInsight == "" || insight.Mechanism == "" {
		t.Fatalf("expected normalized insight forms, got %#v", insight)
	}
	if insight.InsightType == "" || insight.Domain == "" || insight.ExplicitOrInferred == "" {
		t.Fatalf("expected insight classification fields, got %#v", insight)
	}
	if insight.EmbeddingText == "" {
		t.Fatal("expected insight embedding text")
	}

	processed := []ProcessedSource{{
		SourceType:  candidate.sourceType,
		ExternalID:  candidate.externalID,
		SourceURL:   candidate.sourceURL,
		Title:       candidate.title,
		CaptureHash: candidate.captureHash(),
		Synthesis:   record,
	}}
	enriched := service.enrichProcessedSources(context.Background(), processed)
	foundInsightEmbedding := false
	for _, embedding := range enriched[0].Embeddings {
		if embedding.Type == "insight" && embedding.Label == insight.ID && embedding.Vector != "" {
			foundInsightEmbedding = true
		}
	}
	if !foundInsightEmbedding {
		t.Fatalf("expected insight embedding, got %#v", enriched[0].Embeddings)
	}
}

func TestPromptSynthesisBuildsMultipleNormalizedInsights(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	candidate := sourceCandidate{
		sourceType:   SourceTypeX,
		externalID:   "tweet-3",
		sourceURL:    "https://x.com/example/status/tweet-3",
		title:        "Team speed",
		body:         "Small teams move quickly because they coordinate less. Alignment work grows when more people join tightly coupled work.",
		artifactKind: "tweet",
		contentType:  "text/plain; charset=utf-8",
	}
	requestBody := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(raw)
		return jsonResponse(`{
			"output_text": "{\"decision\":\"read_now\",\"summary\":\"The source explains why coordination costs slow teams.\",\"confidence\":\"high\",\"quote\":\"Small teams move quickly because they coordinate less.\",\"insights\":[{\"title\":\"Coordination cost\",\"insight\":\"Small teams move quickly because they coordinate less.\",\"raw_insight\":\"Small teams move quickly because they coordinate less.\",\"canonical_insight\":\"Team speed improves when coordination overhead is low.\",\"abstract_insight\":\"Lower system complexity improves execution speed.\",\"practical_text\":\"Keep teams small when work is tightly coupled.\",\"mechanism\":\"Lower coordination overhead increases speed.\",\"insight_type\":\"warning\",\"domain\":\"organizations\",\"topics\":[\"Coordination\",\"team-size\",\"coordination\"],\"entities\":[\"Teams\"],\"evidence\":\"Small teams move quickly because they coordinate less.\",\"evidence_refs\":[{\"chunkIndex\":0,\"quote\":\"Small teams move quickly because they coordinate less.\"}],\"explicit_or_inferred\":\"explicit\",\"confidence\":\"high\",\"importance_score\":1.2,\"novelty_score\":0.7,\"actionability_score\":0.6},{\"title\":\"Alignment load\",\"insight\":\"Alignment work grows when more people join tightly coupled work.\",\"canonical_insight\":\"Tightly coupled work gets slower as alignment load rises.\",\"mechanism\":\"Rising alignment load slows tightly coupled work.\",\"insight_type\":\"tactic\",\"domain\":\"organizations\",\"topics\":[\"alignment\"],\"evidence\":\"Alignment work grows when more people join tightly coupled work.\",\"confidence\":\"medium\"}],\"action_items\":[]}"
		}`), nil
	})}
	service := NewService(config.Config{
		OpenAISynthesisModel: "test-synthesis-model",
	}, cacheStore{}, client)

	record := service.synthesizeCandidate(context.Background(), candidate, candidate.captureHash(), "generated")

	if !strings.Contains(requestBody, "canonical_insight") || !strings.Contains(requestBody, "mechanism") {
		t.Fatalf("expected prompt to request first-class insight fields, got %s", requestBody)
	}
	if len(record.Insights) != 2 {
		t.Fatalf("expected two insights, got %d", len(record.Insights))
	}
	first := record.Insights[0]
	if first.CanonicalInsight != "Team speed improves when coordination overhead is low." {
		t.Fatalf("unexpected canonical insight: %#v", first)
	}
	if first.ImportanceScore != 1 {
		t.Fatalf("expected importance score to be clamped to 1, got %f", first.ImportanceScore)
	}
	if got := strings.Join(first.Topics, ","); got != "coordination,team-size" {
		t.Fatalf("expected normalized unique topics, got %q", got)
	}
	if len(first.EvidenceRefs) != 1 || first.EvidenceRefs[0].ChunkIndex == nil || *first.EvidenceRefs[0].ChunkIndex != 0 {
		t.Fatalf("expected evidence ref with chunk index, got %#v", first.EvidenceRefs)
	}
	if first.EmbeddingText == "" || !strings.Contains(first.EmbeddingText, "Domain: organizations.") || !strings.Contains(first.EmbeddingText, "Type: warning.") {
		t.Fatalf("expected embedding text to include canonical text and metadata, got %q", first.EmbeddingText)
	}
}

func TestBuildInsightClustersGroupsRepeatedMechanismsAcrossSources(t *testing.T) {
	sources := []ProcessedSource{
		{
			SourceType: SourceTypeX,
			ExternalID: "tweet-a",
			Synthesis: SynthesisRecord{Insights: []Insight{
				{
					ID:               "x-tweet-a-insight-1",
					Insight:          "Small teams move quickly because coordination is simpler.",
					CanonicalInsight: "Team speed improves when coordination overhead is low.",
					Mechanism:        "Lower coordination overhead increases speed.",
					InsightType:      "principle",
				},
				{
					ID:               "x-tweet-a-insight-2",
					Insight:          "Founders should talk to users every week.",
					CanonicalInsight: "Frequent user contact improves product judgement.",
					Mechanism:        "Frequent user contact improves product judgement.",
					InsightType:      "tactic",
				},
			}},
		},
		{
			SourceType: SourceTypeYouTube,
			ExternalID: "video-a",
			Synthesis: SynthesisRecord{Insights: []Insight{
				{
					ID:               "youtube-video-a-insight-1",
					Insight:          "Adding people can slow tightly coupled work because coordination grows.",
					CanonicalInsight: "Team speed drops as coordination overhead rises.",
					Mechanism:        "Lower coordination overhead increases speed.",
					InsightType:      "warning",
				},
			}},
		},
	}

	clusters := buildInsightClusters(sources)

	if len(clusters) != 1 {
		t.Fatalf("expected one repeated-mechanism cluster, got %#v", clusters)
	}
	cluster := clusters[0]
	if cluster.Layer != "similar_insight" {
		t.Fatalf("expected similar insight cluster layer, got %q", cluster.Layer)
	}
	if cluster.Score != 2 {
		t.Fatalf("expected score to count grouped insights, got %f", cluster.Score)
	}
	if got := strings.Join(cluster.InsightIDs, ","); got != "x-tweet-a-insight-1,youtube-video-a-insight-1" {
		t.Fatalf("unexpected clustered insight IDs: %q", got)
	}
	if strings.Contains(strings.Join(cluster.InsightIDs, ","), "x-tweet-a-insight-2") {
		t.Fatalf("one-off insight should not be grouped: %#v", cluster)
	}
}
