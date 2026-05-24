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

func (s cacheStore) ReadDigests(ctx context.Context, limit int) ([]DigestIssue, error) {
	return []DigestIssue{}, nil
}

func (s cacheStore) ReadDigestIllustration(ctx context.Context, ownerID string, digestID string) (*DigestIllustration, error) {
	return nil, nil
}

func (s cacheStore) SaveDigest(ctx context.Context, digest DigestIssue) (*DigestIssue, error) {
	return &digest, nil
}

func (s cacheStore) ReadXTokens(ctx context.Context, ownerID string) (*EncryptedXTokens, error) {
	return nil, nil
}

func (s cacheStore) SaveXTokens(ctx context.Context, tokens EncryptedXTokens) error {
	return nil
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

func TestProcessSourceCandidateDoesNotUseSourceCacheForYouTubeTranscript(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	candidate := sourceCandidate{
		sourceType:   SourceTypeYouTube,
		externalID:   "video-1",
		sourceURL:    "https://www.youtube.com/watch?v=video-1",
		title:        "Transcript-backed video",
		body:         "Full transcript text from Supadata should be synthesized instead of reusing metadata-only cache.",
		artifactKind: "transcript",
		contentType:  "text/plain; charset=utf-8",
	}
	captureHash := candidate.captureHash()
	sourceKey := SynthesisCacheKey{
		SourceType:    SourceTypeYouTube,
		ExternalID:    candidate.externalID,
		PromptVersion: synthesisPromptVersion,
		Model:         extractiveSynthesisModel,
	}
	sourceCached := map[string]SynthesisRecord{
		sourceKey.String(): {
			SourceType:    SourceTypeYouTube,
			ExternalID:    candidate.externalID,
			CaptureHash:   "metadata-only-hash",
			PromptVersion: synthesisPromptVersion,
			Model:         extractiveSynthesisModel,
			Summary: Summary{
				ID:            candidate.externalID,
				Source:        string(SourceTypeYouTube),
				Title:         candidate.title,
				SourceURL:     candidate.sourceURL,
				Decision:      DecisionLater,
				Summary:       "Transcript is unavailable; only chapter markers support this synthesis.",
				Confidence:    "low",
				CaptureHash:   "metadata-only-hash",
				PromptVersion: synthesisPromptVersion,
				Model:         extractiveSynthesisModel,
			},
		},
	}
	service := NewService(config.Config{SupabaseStorageBucket: "sources"}, cacheStore{}, http.DefaultClient)

	processed := service.processSourceCandidate(context.Background(), candidate, captureHash, map[string]SynthesisRecord{}, sourceCached)

	if processed.Cached {
		t.Fatal("expected YouTube transcript candidate to bypass source-level metadata cache")
	}
	if processed.CaptureHash != captureHash {
		t.Fatalf("expected current transcript capture hash %q, got %q", captureHash, processed.CaptureHash)
	}
	if strings.Contains(processed.Synthesis.Summary.Summary, "Transcript is unavailable") {
		t.Fatalf("expected fresh transcript-backed synthesis, got %#v", processed.Synthesis.Summary)
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
	if insight.Quality == nil || insight.Quality.Overall == 0 {
		t.Fatalf("expected fallback insight quality score, got %#v", insight.Quality)
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
			"output_text": "{\"decision\":\"read_now\",\"summary\":\"The source explains why coordination costs slow teams.\",\"confidence\":\"high\",\"quote\":\"Small teams move quickly because they coordinate less.\",\"quality\":{\"overall\":0.9,\"conciseness\":0.9,\"efficacy\":0.9,\"grounding\":0.95,\"novelty\":0.7,\"verdict\":\"pass\"},\"important_time_markers\":[],\"insights\":[{\"title\":\"Coordination cost\",\"insight\":\"Small teams move quickly because they coordinate less.\",\"raw_insight\":\"Small teams move quickly because they coordinate less.\",\"canonical_insight\":\"Team speed improves when coordination overhead is low.\",\"abstract_insight\":\"Lower system complexity improves execution speed.\",\"practical_text\":\"Keep teams small when work is tightly coupled.\",\"mechanism\":\"Lower coordination overhead increases speed.\",\"insight_type\":\"warning\",\"domain\":\"organizations\",\"topics\":[\"Coordination\",\"team-size\",\"coordination\"],\"entities\":[\"Teams\"],\"evidence\":\"Small teams move quickly because they coordinate less.\",\"evidence_refs\":[{\"chunkIndex\":0,\"quote\":\"Small teams move quickly because they coordinate less.\"}],\"explicit_or_inferred\":\"explicit\",\"confidence\":\"high\",\"importance_score\":1.2,\"novelty_score\":0.7,\"actionability_score\":0.6,\"quality\":{\"overall\":0.88,\"conciseness\":0.9,\"efficacy\":0.85,\"grounding\":0.95,\"novelty\":0.65,\"verdict\":\"pass\"}},{\"title\":\"Alignment load\",\"insight\":\"Alignment work grows when more people join tightly coupled work.\",\"canonical_insight\":\"Tightly coupled work gets slower as alignment load rises.\",\"mechanism\":\"Rising alignment load slows tightly coupled work.\",\"insight_type\":\"tactic\",\"domain\":\"organizations\",\"topics\":[\"alignment\"],\"evidence\":\"Alignment work grows when more people join tightly coupled work.\",\"confidence\":\"medium\"}],\"action_items\":[]}"
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
	if record.Summary.Quality == nil || record.Summary.Quality.Overall < 0.89 || first.Quality == nil || first.Quality.Overall < 0.87 {
		t.Fatalf("expected prompt quality scores, got summary=%#v insight=%#v", record.Summary.Quality, first.Quality)
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
	if cluster.Score != 2.5 {
		t.Fatalf("expected score to count grouped insights, got %f", cluster.Score)
	}
	if got := strings.Join(cluster.InsightIDs, ","); got != "x-tweet-a-insight-1,youtube-video-a-insight-1" {
		t.Fatalf("unexpected clustered insight IDs: %q", got)
	}
	if strings.Contains(strings.Join(cluster.InsightIDs, ","), "x-tweet-a-insight-2") {
		t.Fatalf("one-off insight should not be grouped: %#v", cluster)
	}
}

func TestBuildInsightClustersGroupsSimilarCanonicalInsightsAcrossSourceTypes(t *testing.T) {
	sources := []ProcessedSource{
		{
			SourceType: SourceTypeX,
			ExternalID: "tweet-b",
			Synthesis: SynthesisRecord{Insights: []Insight{
				{
					ID:               "x-tweet-b-insight-1",
					Insight:          "Agent workflows need eval loops before they can be trusted.",
					CanonicalInsight: "AI agents improve when evaluation loops measure task quality.",
					AbstractInsight:  "Closed-loop measurement improves automated systems.",
					Mechanism:        "Evaluation feedback exposes failure modes and improves iteration.",
					Topics:           []string{"agents", "evaluation", "workflow"},
				},
			}},
		},
		{
			SourceType: SourceTypeYouTube,
			ExternalID: "video-b",
			Synthesis: SynthesisRecord{Insights: []Insight{
				{
					ID:               "youtube-video-b-insight-1",
					Insight:          "Quality gets better when agents are scored against real workflow outcomes.",
					CanonicalInsight: "Evaluation loops improve AI agent workflow quality.",
					AbstractInsight:  "Closed-loop measurement improves automated systems.",
					Mechanism:        "Measured feedback reveals agent failure modes and directs iteration.",
					Topics:           []string{"ai agents", "evaluation", "quality"},
				},
			}},
		},
	}

	clusters := buildInsightClusters(sources)

	if len(clusters) != 1 {
		t.Fatalf("expected similar canonical insights to cluster across sources, got %#v", clusters)
	}
	if got := strings.Join(clusters[0].InsightIDs, ","); got != "x-tweet-b-insight-1,youtube-video-b-insight-1" {
		t.Fatalf("unexpected clustered insight IDs: %q", got)
	}
}

func TestRankInsightsPromotesScoredAndClusteredInsights(t *testing.T) {
	insights := []Insight{
		{ID: "low", Insight: "Low-signal note.", Confidence: "low", ImportanceScore: 0.2, NoveltyScore: 0.2, ActionabilityScore: 0.1},
		{ID: "clustered", Insight: "Repeated useful note.", Confidence: "medium", ImportanceScore: 0.6, NoveltyScore: 0.5, ActionabilityScore: 0.5},
		{ID: "high", Insight: "Important actionable note.", Confidence: "high", ImportanceScore: 0.9, NoveltyScore: 0.8, ActionabilityScore: 0.8},
	}
	clusters := []InsightCluster{{Score: 4, InsightIDs: []string{"clustered"}}}

	ranked := rankInsights(insights, clusters)

	if got := ranked[0].ID; got != "high" {
		t.Fatalf("expected strongest scored insight first, got %q", got)
	}
	if got := ranked[1].ID; got != "clustered" {
		t.Fatalf("expected clustered insight ahead of low-signal item, got %q", got)
	}
}

func TestNormalizeResultInsightEngineBuildsClustersForPersistedRuns(t *testing.T) {
	result := &Result{Insights: []Insight{
		{
			ID:               "x-1",
			Source:           "x",
			SourceID:         "tweet-c",
			Insight:          "Agent workflows need evaluation loops.",
			CanonicalInsight: "AI agent workflows improve when evaluation loops measure quality.",
			Mechanism:        "Evaluation feedback exposes failure modes and improves iteration.",
			Topics:           []string{"agents", "evaluation"},
			ImportanceScore:  0.6,
		},
		{
			ID:               "youtube-1",
			Source:           "youtube",
			SourceID:         "video-c",
			Insight:          "Agents get better when real workflow outcomes are scored.",
			CanonicalInsight: "Evaluation loops improve AI agent workflow quality.",
			Mechanism:        "Measured feedback reveals agent failure modes and directs iteration.",
			Topics:           []string{"agents", "evaluation"},
			ImportanceScore:  0.6,
		},
	}}

	normalizeResultInsightEngine(result)

	if len(result.InsightClusters) != 1 {
		t.Fatalf("expected persisted run insights to cluster, got %#v", result.InsightClusters)
	}
	if result.Insights[0].ID == "" {
		t.Fatalf("expected ranked insights to remain populated")
	}
}
