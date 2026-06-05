package knowledge

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

const defaultSampleDigestSourceCount = 3

type SampleDigestResult struct {
	TraceID              string            `json:"traceId,omitempty"`
	GeneratedAt          time.Time         `json:"generatedAt"`
	RequestedSourceCount int               `json:"requestedSourceCount"`
	SelectedSourceCount  int               `json:"selectedSourceCount"`
	Sources              []DigestSourceRef `json:"sources"`
	Digest               DigestIssue       `json:"digest"`
	Persisted            bool              `json:"persisted"`
	Delivered            bool              `json:"delivered"`
}

func (s *Service) GenerateSampleDigest(ctx context.Context, sourceCount int) (*SampleDigestResult, error) {
	sourceCount = normalizeSampleDigestSourceCount(sourceCount)
	ctx, span := s.startOperationSpan(ctx, "langfuse-sample-digest", "debug", "digest", "langfuse-check")
	traceID := ""
	if span.SpanContext().HasTraceID() {
		traceID = span.SpanContext().TraceID().String()
	}
	defer span.End()

	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		err := fmt.Errorf("OPENAI_API_KEY is required for sample digest synthesis")
		setSpanError(span, err)
		return nil, err
	}
	latest, err := s.readLatestCanonical(ctx)
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}
	refs, summaries, insights, themes, insightClusters, connections, err := sampleDigestInputs(latest, sourceCount)
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}

	generatedAt := time.Now().UTC()
	digestInsights := selectDigestInsights(generatedAt, insights, digestMaxInsightCount)
	digest := buildDigestIssue(s.cfg.DigestTimezone, s.cfg.DigestTime, generatedAt, summaries, digestInsights, themes, insightClusters, connections)
	digest.OwnerID = s.cfg.OwnerID
	digest.IdempotencyKey = "sample:" + generatedAt.Format("20060102T150405.000000000Z")

	payload, err := s.promptDigest(ctx, digest, summaries, digestInsights, themes, insightClusters, connections)
	if err != nil {
		setSpanError(span, err)
		return nil, fmt.Errorf("sample digest newsletter synthesis failed: %w", err)
	}
	if strings.TrimSpace(payload.Subject) == "" {
		err := fmt.Errorf("sample digest newsletter synthesis returned an empty subject")
		setSpanError(span, err)
		return nil, err
	}
	digest.Subject = strings.TrimSpace(payload.Subject)
	bodyMarkdown := strings.TrimSpace(payload.BodyMarkdown)
	if bodyMarkdown == "" && len(payload.BodyLines) > 0 {
		bodyMarkdown = strings.TrimSpace(strings.Join(payload.BodyLines, "\n"))
	}
	digest.BodyMarkdown = ensureDigestSourceLinks(bodyMarkdown, digestInsights)
	if strings.TrimSpace(digest.BodyMarkdown) == "" {
		err := fmt.Errorf("sample digest newsletter synthesis returned an empty body")
		setSpanError(span, err)
		return nil, err
	}
	digest.Status = "sample"
	digest.SourceRefs = refs

	result := &SampleDigestResult{
		TraceID:              traceID,
		GeneratedAt:          generatedAt,
		RequestedSourceCount: sourceCount,
		SelectedSourceCount:  len(refs),
		Sources:              refs,
		Digest:               digest,
		Persisted:            false,
		Delivered:            false,
	}
	setSpanOutputSummary(span, map[string]any{
		"trace_id":          traceID,
		"source_count":      len(refs),
		"summary_count":     len(summaries),
		"insight_count":     len(digestInsights),
		"digest_date":       digest.DigestDate,
		"digest_status":     digest.Status,
		"persisted":         false,
		"delivered":         false,
		"subject_chars":     len(digest.Subject),
		"body_markdown":     len(digest.BodyMarkdown),
		"requested_sources": sourceCount,
	})
	return result, nil
}

func normalizeSampleDigestSourceCount(sourceCount int) int {
	if sourceCount <= 0 {
		return defaultSampleDigestSourceCount
	}
	if sourceCount > 10 {
		return 10
	}
	return sourceCount
}

func sampleDigestInputs(latest *Result, sourceCount int) ([]DigestSourceRef, []Summary, []Insight, []ThemeCluster, []InsightCluster, []SourceConnection, error) {
	if latest == nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("no knowledge run is available for sample digest generation")
	}
	sourceRefs := sampleDigestSourceRefsFromLatest(latest)
	if len(sourceRefs) == 0 {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("no source-grounded inputs are available for sample digest generation")
	}
	shuffleDigestSourceRefs(sourceRefs)
	if len(sourceRefs) > sourceCount {
		sourceRefs = sourceRefs[:sourceCount]
	}
	allowed := digestSourceRefMap(sourceRefs)
	summaries := filterDigestSummaries(latest.Summaries, allowed)
	insights := filterDigestInsights(latest.Insights, allowed)
	sourceRefs = filterDigestSourceRefsForInputs(sourceRefs, summaries, insights)
	if len(sourceRefs) == 0 || !hasDigestInputs(summaries, insights) {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("sampled sources did not include digestable inputs")
	}
	allowed = digestSourceRefMap(sourceRefs)
	insightIDs := digestInsightIDSet(insights)
	return sourceRefs,
		summaries,
		insights,
		filterDigestThemes(latest.Themes, allowed),
		filterDigestInsightClusters(latest.InsightClusters, insightIDs),
		filterDigestConnections(latest.Connections, allowed),
		nil
}

func sampleDigestSourceRefsFromLatest(latest *Result) []DigestSourceRef {
	if latest == nil {
		return nil
	}
	refs := []DigestSourceRef{}
	refIndexes := map[string]int{}
	addRef := func(source string, externalID string, title string, sourceURL string, captureHash string, generatedAt *time.Time) {
		source = strings.TrimSpace(source)
		externalID = strings.TrimSpace(externalID)
		if source == "" || externalID == "" {
			return
		}
		seenAt := latest.GeneratedAt
		if seenAt.IsZero() {
			seenAt = time.Now().UTC()
		}
		if generatedAt != nil && !generatedAt.IsZero() {
			seenAt = generatedAt.UTC()
		}
		key := digestSourceKey(source, externalID)
		if index, exists := refIndexes[key]; exists {
			if refs[index].Title == "" {
				refs[index].Title = title
			}
			if refs[index].SourceURL == "" {
				refs[index].SourceURL = sourceURL
			}
			if refs[index].CaptureHash == "" {
				refs[index].CaptureHash = captureHash
			}
			return
		}
		seenAtCopy := seenAt.UTC()
		refIndexes[key] = len(refs)
		refs = append(refs, DigestSourceRef{
			Source:        source,
			ExternalID:    externalID,
			SourceURL:     sourceURL,
			Title:         title,
			CaptureHash:   captureHash,
			FirstSeenAt:   &seenAtCopy,
			SynthesizedAt: &seenAtCopy,
			DigestRole:    "sample_input",
		})
	}
	for _, summary := range latest.Summaries {
		addRef(summary.Source, summary.ID, summary.Title, summary.SourceURL, summary.CaptureHash, summary.GeneratedAt)
	}
	for _, insight := range latest.Insights {
		addRef(insight.Source, insight.SourceID, insight.Title, insight.SourceURL, "", insight.GeneratedAt)
	}
	return refs
}

func shuffleDigestSourceRefs(refs []DigestSourceRef) {
	for i := len(refs) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return
		}
		j := int(n.Int64())
		refs[i], refs[j] = refs[j], refs[i]
	}
}
