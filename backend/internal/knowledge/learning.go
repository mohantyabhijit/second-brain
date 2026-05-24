package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

const embeddingDimensions = 1536

var keywordPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9-]{2,}`)

const (
	digestThemeEvidenceLimit     = 180
	digestSummaryLimit           = 260
	digestSourceEvidenceLimit    = 200
	digestConnectionLimit        = 220
	digestMaxSourceNoteCount     = 5
	digestMaxInsightCount        = 5
	digestMaxInsightClusterCount = 6
	digestMaxConnectionCount     = 5
	insightClusterThreshold      = 0.25
)

var stopwords = map[string]bool{
	"about": true, "after": true, "again": true, "also": true, "and": true, "are": true, "because": true,
	"but": true, "can": true, "from": true, "have": true, "into": true, "just": true, "like": true,
	"more": true, "not": true, "only": true, "should": true, "source": true, "that": true, "the": true,
	"their": true, "there": true, "this": true, "through": true, "video": true, "was": true, "what": true,
	"when": true, "where": true, "which": true, "with": true, "would": true, "you": true, "your": true,
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type clusteredInsight struct {
	insight  Insight
	sourceID string
}

type insightClusterAccumulator struct {
	key       string
	label     string
	signature []string
	insights  []clusteredInsight
	sourceIDs map[string]bool
}

func (s *Service) enrichProcessedSources(ctx context.Context, sources []ProcessedSource) []ProcessedSource {
	type embeddingPlan struct {
		sourceIndex int
		recordIndex int
		text        string
	}
	plans := []embeddingPlan{}
	for index := range sources {
		sources[index].OwnerID = s.cfg.OwnerID
		sources[index].Chunks = chunkText(sources[index].Synthesis.Summary.ID, sources[index].Synthesis.Summary.Summary+" "+sources[index].Synthesis.Summary.Quote)
		if len(sources[index].Chunks) == 0 {
			sources[index].Chunks = []SourceChunk{chunkForText(0, sources[index].Title+" "+sources[index].Synthesis.Summary.Summary)}
		}
		sources[index].Keywords = topKeywords(strings.Join([]string{
			sources[index].Title,
			sources[index].Synthesis.Summary.Summary,
			sources[index].Synthesis.Summary.Quote,
		}, " "), 8)
		sources[index].Entities = entitiesFromKeywords(sources[index].Keywords, sources[index].Synthesis.Summary.Quote)
		if sources[index].Cached {
			continue
		}

		model := s.embeddingModel()
		if len(sources[index].Chunks) > 0 {
			chunk := sources[index].Chunks[0]
			chunkIndex := chunk.Index
			sources[index].Embeddings = append(sources[index].Embeddings, EmbeddingRecord{
				Type:       "chunk",
				Label:      sources[index].Title,
				Model:      model,
				Dimensions: embeddingDimensions,
				ChunkIndex: &chunkIndex,
			})
			plans = append(plans, embeddingPlan{sourceIndex: index, recordIndex: len(sources[index].Embeddings) - 1, text: chunk.Content})
		}
		sources[index].Embeddings = append(sources[index].Embeddings, EmbeddingRecord{
			Type:       "summary",
			Label:      sources[index].Title,
			Model:      model,
			Dimensions: embeddingDimensions,
		})
		plans = append(plans, embeddingPlan{sourceIndex: index, recordIndex: len(sources[index].Embeddings) - 1, text: sources[index].Synthesis.Summary.Summary})
		for _, insight := range sources[index].Synthesis.Insights {
			embeddingText := insight.EmbeddingText
			if embeddingText == "" {
				embeddingText = insightEmbeddingText(
					fallback(insight.CanonicalInsight, insight.Insight),
					fallback(insight.Domain, "general"),
					fallback(insight.InsightType, "principle"),
					insight.Topics,
				)
			}
			sources[index].Embeddings = append(sources[index].Embeddings, EmbeddingRecord{
				Type:       "insight",
				Label:      insight.ID,
				Model:      model,
				Dimensions: embeddingDimensions,
			})
			plans = append(plans, embeddingPlan{sourceIndex: index, recordIndex: len(sources[index].Embeddings) - 1, text: embeddingText})
		}
		for _, entity := range sources[index].Entities {
			sources[index].Embeddings = append(sources[index].Embeddings, EmbeddingRecord{
				Type:       "entity",
				Label:      entity.Label,
				Model:      model,
				Dimensions: embeddingDimensions,
			})
			plans = append(plans, embeddingPlan{sourceIndex: index, recordIndex: len(sources[index].Embeddings) - 1, text: entity.Label + " " + entity.Evidence})
		}
	}

	inputs := make([]string, 0, len(plans))
	for _, plan := range plans {
		inputs = append(inputs, plan.text)
	}
	vectors := s.embeddingLiterals(ctx, inputs)
	for index, plan := range plans {
		sources[plan.sourceIndex].Embeddings[plan.recordIndex].Vector = vectors[index]
	}
	return sources
}

func (s *Service) embeddingModel() string {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return "local-hash-embedding-v1"
	}
	return s.cfg.OpenAIEmbeddingModel
}

func (s *Service) embeddingLiterals(ctx context.Context, texts []string) []string {
	if len(texts) == 0 {
		return nil
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return deterministicEmbeddingLiterals(texts)
	}

	inputs := make([]string, 0, len(texts))
	for _, text := range texts {
		inputs = append(inputs, truncate(text, 6000))
	}
	requestBody := map[string]any{
		"model": s.cfg.OpenAIEmbeddingModel,
		"input": inputs,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return deterministicEmbeddingLiterals(texts)
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response embeddingResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", headers, bytes.NewReader(raw), &response); err != nil {
		return deterministicEmbeddingLiterals(texts)
	}
	if response.Error != nil && response.Error.Message != "" {
		return deterministicEmbeddingLiterals(texts)
	}
	if len(response.Data) != len(texts) {
		return deterministicEmbeddingLiterals(texts)
	}
	literals := make([]string, 0, len(response.Data))
	for index, item := range response.Data {
		if len(item.Embedding) != embeddingDimensions {
			literals = append(literals, deterministicEmbeddingLiteral(texts[index]))
			continue
		}
		literals = append(literals, vectorLiteral(item.Embedding))
	}
	return literals
}

func deterministicEmbeddingLiterals(texts []string) []string {
	literals := make([]string, 0, len(texts))
	for _, text := range texts {
		literals = append(literals, deterministicEmbeddingLiteral(text))
	}
	return literals
}

func deterministicEmbeddingLiteral(text string) string {
	values := make([]float64, embeddingDimensions)
	seed := sha256.Sum256([]byte(text))
	for i := range values {
		hash := sha256.Sum256(append(seed[:], byte(i), byte(i>>8)))
		raw := binary.BigEndian.Uint64(hash[:8])
		values[i] = (float64(raw%2000000)/1000000.0 - 1.0) / math.Sqrt(embeddingDimensions)
	}
	return vectorLiteral(values)
}

func vectorLiteral(values []float64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%.8f", value))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func chunkText(id string, text string) []SourceChunk {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	const chunkSize = 2400
	chunks := []SourceChunk{}
	for start, index := 0, 0; start < len(trimmed); start, index = start+chunkSize, index+1 {
		end := start + chunkSize
		if end > len(trimmed) {
			end = len(trimmed)
		}
		chunk := chunkForText(index, trimmed[start:end])
		if id != "" && chunk.Content == "" {
			chunk.Content = id
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func chunkForText(index int, text string) SourceChunk {
	raw := []byte(strings.TrimSpace(text))
	checksum := sha256.Sum256(raw)
	return SourceChunk{
		Index:         index,
		Content:       string(raw),
		TokenEstimate: max(1, len(raw)/4),
		Checksum:      hex.EncodeToString(checksum[:]),
	}
}

func topKeywords(text string, limit int) []string {
	counts := map[string]int{}
	for _, match := range keywordPattern.FindAllString(strings.ToLower(text), -1) {
		match = strings.Trim(match, "-")
		if len(match) < 3 || stopwords[match] {
			continue
		}
		counts[match]++
	}
	type scored struct {
		keyword string
		count   int
	}
	scoredKeywords := make([]scored, 0, len(counts))
	for keyword, count := range counts {
		scoredKeywords = append(scoredKeywords, scored{keyword: keyword, count: count})
	}
	sort.Slice(scoredKeywords, func(i, j int) bool {
		if scoredKeywords[i].count == scoredKeywords[j].count {
			return scoredKeywords[i].keyword < scoredKeywords[j].keyword
		}
		return scoredKeywords[i].count > scoredKeywords[j].count
	})
	keywords := make([]string, 0, min(limit, len(scoredKeywords)))
	for _, item := range scoredKeywords {
		if len(keywords) >= limit {
			break
		}
		keywords = append(keywords, item.keyword)
	}
	return keywords
}

func entitiesFromKeywords(keywords []string, evidence string) []EntityRecord {
	entities := make([]EntityRecord, 0, min(5, len(keywords)))
	for _, keyword := range keywords {
		if len(entities) >= 5 {
			break
		}
		entities = append(entities, EntityRecord{
			Label:      keyword,
			Kind:       "keyword",
			Confidence: "medium",
			Evidence:   truncate(evidence, 240),
		})
	}
	return entities
}

func buildThemeClusters(sources []ProcessedSource) []ThemeCluster {
	type themeAccumulator struct {
		label    string
		evidence []string
		sources  []string
		score    float64
	}
	accumulators := map[string]*themeAccumulator{}
	for _, source := range sources {
		for _, keyword := range source.Keywords {
			accumulator, ok := accumulators[keyword]
			if !ok {
				accumulator = &themeAccumulator{label: titleCaseKeyword(keyword)}
				accumulators[keyword] = accumulator
			}
			sourceID := string(source.SourceType) + ":" + source.ExternalID
			if !slices.Contains(accumulator.sources, sourceID) {
				accumulator.sources = append(accumulator.sources, sourceID)
				accumulator.score++
			}
			if len(accumulator.evidence) < 3 {
				accumulator.evidence = append(accumulator.evidence, truncate(source.Synthesis.Summary.Quote, 180))
			}
		}
	}
	clusters := []ThemeCluster{}
	for key, accumulator := range accumulators {
		if accumulator.score < 2 {
			continue
		}
		clusters = append(clusters, ThemeCluster{
			ID:       "theme-" + key,
			Label:    accumulator.label,
			Evidence: strings.Join(nonEmpty(accumulator.evidence), " | "),
			Score:    accumulator.score,
			Sources:  accumulator.sources,
		})
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Score == clusters[j].Score {
			return clusters[i].Label < clusters[j].Label
		}
		return clusters[i].Score > clusters[j].Score
	})
	if len(clusters) > 8 {
		return clusters[:8]
	}
	return clusters
}

func buildInsightClusters(sources []ProcessedSource) []InsightCluster {
	accumulators := []*insightClusterAccumulator{}
	byExactKey := map[string]*insightClusterAccumulator{}
	for _, source := range sources {
		sourceID := string(source.SourceType) + ":" + source.ExternalID
		for _, insight := range source.Synthesis.Insights {
			key := insightClusterKey(insight)
			if key == "" {
				continue
			}
			signature := insightSignature(insight)
			accumulator := byExactKey[key]
			if accumulator == nil {
				accumulator = bestInsightCluster(accumulators, signature)
			}
			if accumulator == nil {
				accumulator = &insightClusterAccumulator{
					key:       key,
					label:     insightClusterLabel(insight, key),
					signature: signature,
					sourceIDs: map[string]bool{},
				}
				accumulators = append(accumulators, accumulator)
				byExactKey[key] = accumulator
			}
			accumulator.insights = append(accumulator.insights, clusteredInsight{insight: insight, sourceID: sourceID})
			accumulator.sourceIDs[sourceID] = true
		}
	}
	clusters := []InsightCluster{}
	for _, accumulator := range accumulators {
		if len(accumulator.insights) < 2 {
			continue
		}
		insightIDs := make([]string, 0, len(accumulator.insights))
		examples := make([]string, 0, min(3, len(accumulator.insights)))
		for _, item := range accumulator.insights {
			insight := item.insight
			if insight.ID == "" {
				continue
			}
			insightIDs = append(insightIDs, insight.ID)
			if len(examples) < 3 {
				examples = append(examples, fallback(insight.CanonicalInsight, insight.Insight))
			}
		}
		if len(insightIDs) < 2 || len(examples) == 0 {
			continue
		}
		sourceDiversityBonus := 0.5 * float64(len(accumulator.sourceIDs)-1)
		clusters = append(clusters, InsightCluster{
			ID:                       "insight-cluster-" + storagePathSegment(accumulator.key),
			Label:                    accumulator.label,
			CanonicalInsight:         examples[0],
			Summary:                  "Recurring mechanism across insights: " + strings.Join(examples, " / "),
			Layer:                    "similar_insight",
			Score:                    float64(len(insightIDs)) + sourceDiversityBonus,
			RepresentativeInsightIDs: insightIDs[:min(3, len(insightIDs))],
			InsightIDs:               insightIDs,
		})
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Score == clusters[j].Score {
			return clusters[i].Label < clusters[j].Label
		}
		return clusters[i].Score > clusters[j].Score
	})
	if len(clusters) > 8 {
		return clusters[:8]
	}
	return clusters
}

func normalizeResultInsightEngine(result *Result) {
	if result == nil {
		return
	}
	if len(result.InsightClusters) == 0 && len(result.Insights) > 0 {
		result.InsightClusters = buildInsightClustersFromInsights(result.Insights)
	}
	result.Insights = rankInsights(result.Insights, result.InsightClusters)
}

func buildInsightClustersFromInsights(insights []Insight) []InsightCluster {
	if len(insights) == 0 {
		return nil
	}
	sourcesByID := map[string]*ProcessedSource{}
	order := []string{}
	for _, insight := range insights {
		sourceType := SourceType(strings.TrimSpace(insight.Source))
		if sourceType == "" {
			sourceType = SourceType("unknown")
		}
		sourceID := strings.TrimSpace(insight.SourceID)
		if sourceID == "" {
			sourceID = "unknown"
		}
		key := string(sourceType) + ":" + sourceID
		source := sourcesByID[key]
		if source == nil {
			source = &ProcessedSource{
				SourceType: sourceType,
				ExternalID: sourceID,
				SourceURL:  insight.SourceURL,
				Title:      insight.Title,
				Synthesis: SynthesisRecord{
					Insights: []Insight{},
				},
			}
			sourcesByID[key] = source
			order = append(order, key)
		}
		source.Synthesis.Insights = append(source.Synthesis.Insights, insight)
	}
	sources := make([]ProcessedSource, 0, len(order))
	for _, key := range order {
		sources = append(sources, *sourcesByID[key])
	}
	return buildInsightClusters(sources)
}

func rankInsights(insights []Insight, clusters []InsightCluster) []Insight {
	if len(insights) < 2 {
		return insights
	}
	clusterScores := map[string]float64{}
	for _, cluster := range clusters {
		for _, insightID := range cluster.InsightIDs {
			if cluster.Score > clusterScores[insightID] {
				clusterScores[insightID] = cluster.Score
			}
		}
	}
	ranked := slices.Clone(insights)
	sort.SliceStable(ranked, func(i, j int) bool {
		left := insightRankScore(ranked[i], clusterScores[ranked[i].ID])
		right := insightRankScore(ranked[j], clusterScores[ranked[j].ID])
		if left == right {
			return ranked[i].ID < ranked[j].ID
		}
		return left > right
	})
	return ranked
}

func insightRankScore(insight Insight, clusterScore float64) float64 {
	confidenceBonus := 0.0
	switch normalizedConfidence(insight.Confidence) {
	case "high":
		confidenceBonus = 0.15
	case "medium":
		confidenceBonus = 0.08
	}
	sourceDiversityBonus := math.Min(clusterScore, 5) * 0.06
	qualityBonus := 0.0
	if insight.Quality != nil {
		qualityBonus = normalizedScore(insight.Quality.Overall, 0) * 0.18
	}
	return insight.ImportanceScore*0.34 + insight.ActionabilityScore*0.23 + insight.NoveltyScore*0.17 + qualityBonus + confidenceBonus + sourceDiversityBonus
}

func bestInsightCluster(accumulators []*insightClusterAccumulator, signature []string) *insightClusterAccumulator {
	if len(signature) == 0 {
		return nil
	}
	var best *insightClusterAccumulator
	bestScore := 0.0
	for _, accumulator := range accumulators {
		score := tokenJaccard(signature, accumulator.signature)
		if score > bestScore {
			bestScore = score
			best = accumulator
		}
	}
	if bestScore < insightClusterThreshold {
		return nil
	}
	return best
}

func insightSignature(insight Insight) []string {
	text := strings.Join([]string{
		insight.Mechanism,
		insight.CanonicalInsight,
		insight.AbstractInsight,
		insight.PracticalText,
		strings.Join(insight.Topics, " "),
		strings.Join(insight.Entities, " "),
	}, " ")
	return topKeywords(canonicalInsightText(text), 24)
}

func tokenJaccard(left []string, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	leftSet := map[string]bool{}
	for _, value := range left {
		leftSet[value] = true
	}
	rightSet := map[string]bool{}
	for _, value := range right {
		rightSet[value] = true
	}
	intersection := 0
	for value := range leftSet {
		if rightSet[value] {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func insightClusterKey(insight Insight) string {
	key := strings.ToLower(strings.TrimSpace(insight.Mechanism))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(insight.CanonicalInsight))
	}
	if key == "" && len(insight.Topics) > 0 {
		key = strings.Join(insight.Topics[:min(2, len(insight.Topics))], "-")
	}
	return strings.Trim(key, ". ")
}

func insightClusterLabel(insight Insight, key string) string {
	label := strings.TrimSpace(insight.Mechanism)
	if label == "" {
		label = strings.TrimSpace(insight.CanonicalInsight)
	}
	if label == "" {
		label = key
	}
	return truncate(canonicalInsightText(label), 96)
}

func buildSourceConnections(sources []ProcessedSource) []SourceConnection {
	connections := []SourceConnection{}
	for i := range sources {
		for j := i + 1; j < len(sources); j++ {
			shared := sharedKeywords(sources[i].Keywords, sources[j].Keywords)
			if len(shared) < 2 {
				continue
			}
			leftID := string(sources[i].SourceType) + ":" + sources[i].ExternalID
			rightID := string(sources[j].SourceType) + ":" + sources[j].ExternalID
			connections = append(connections, SourceConnection{
				ID:            "connection-" + sources[i].ExternalID + "-" + sources[j].ExternalID,
				LeftSourceID:  leftID,
				RightSourceID: rightID,
				Relationship:  "related_to",
				Evidence:      fmt.Sprintf("Both sources discuss %s. %s / %s", strings.Join(shared, ", "), truncate(sources[i].Synthesis.Summary.Quote, 120), truncate(sources[j].Synthesis.Summary.Quote, 120)),
				Confidence:    "medium",
				SharedSignals: shared,
			})
			if len(connections) >= 12 {
				return connections
			}
		}
	}
	return connections
}

func sharedKeywords(left []string, right []string) []string {
	values := []string{}
	for _, item := range left {
		if slices.Contains(right, item) {
			values = append(values, item)
		}
		if len(values) >= 5 {
			break
		}
	}
	return values
}

func titleCaseKeyword(keyword string) string {
	if keyword == "" {
		return ""
	}
	return strings.ToUpper(keyword[:1]) + keyword[1:]
}

func nonEmpty(values []string) []string {
	filtered := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func buildDigestIssue(cfgDigestTimezone string, cfgDigestTime string, generatedAt time.Time, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) DigestIssue {
	location, err := time.LoadLocation(cfgDigestTimezone)
	if err != nil {
		location = time.FixedZone("Asia/Singapore", 8*60*60)
	}
	localDate := generatedAt.In(location)
	hour, minute := parseDigestClock(cfgDigestTime)
	scheduledFor := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), hour, minute, 0, 0, location)
	digestDate := scheduledFor.Format("2006-01-02")
	insights = selectDigestInsights(generatedAt, insights, digestMaxInsightCount)
	subject := "Five signals worth rereading"
	lines := []string{"# " + subject, ""}
	lines = append(lines, "A source-linked newsletter from Abhijit's Second Brain, built from the saved ideas most worth turning into a note or decision.", "")
	if len(insights) > 0 {
		lines = append(lines, "## Editor's Note")
		lines = append(lines, "The useful pattern in this run is not volume; it is the handful of saved ideas that still have enough signal to change what you read, build, or ignore next.", "")
		lines = append(lines, "## In This Issue")
		for _, insight := range insights {
			link := insight.SourceURL
			title := fallback(insight.Title, "Source-backed insight")
			if link == "" {
				lines = append(lines, fmt.Sprintf("- **%s**: %s", title, truncateDigestText(insight.Insight, digestSummaryLimit)))
			} else {
				lines = append(lines, fmt.Sprintf("- **[%s](%s)**: %s", title, link, truncateDigestText(insight.Insight, digestSummaryLimit)))
			}
		}
		lines = append(lines, "", "## The Newsletter")
		for index, insight := range insights {
			title := fallback(insight.Title, "Source-backed insight")
			lines = append(lines, fmt.Sprintf("### %d. %s", index+1, title))
			lines = append(lines, "Why it matters: "+truncateDigestText(insight.Insight, digestSummaryLimit))
			if insight.Evidence != "" {
				lines = append(lines, "Source note: "+truncateDigestText(insight.Evidence, digestSourceEvidenceLimit))
			}
			if insight.SourceURL != "" {
				lines = append(lines, fmt.Sprintf("Read the original: [%s](%s)", title, insight.SourceURL))
			}
			lines = append(lines, "")
		}
		lines = append(lines, "")
	}
	if len(insights) == 0 && len(themes) > 0 {
		lines = append(lines, "## The Lead")
		for _, theme := range themes {
			lines = append(lines, fmt.Sprintf("- **%s** showed up across %.0f source(s). %s", theme.Label, theme.Score, truncateDigestText(theme.Evidence, digestThemeEvidenceLimit)))
		}
		lines = append(lines, "")
	}
	if len(insights) == 0 && len(summaries) > 0 {
		lines = append(lines, "## What To Read")
		for index, summary := range summaries {
			if index >= digestMaxSourceNoteCount {
				lines = append(lines, fmt.Sprintf("- %d more source note(s) kept in the app.", len(summaries)-index))
				break
			}
			lines = append(lines, fmt.Sprintf("- **[%s](%s)**: %s", summary.Title, summary.SourceURL, truncateDigestText(summary.Summary, digestSummaryLimit)))
			if summary.Quote != "" {
				lines = append(lines, "  Evidence: "+truncateDigestText(summary.Quote, digestSourceEvidenceLimit))
			}
		}
		lines = append(lines, "")
	}
	if len(insights) > 0 {
		lines = append(lines, "## One Thing To Do Next")
		lines = append(lines, "Pick the one signal that still feels unresolved, open the source, and turn it into a short note before the next refresh.")
	}
	bodyMarkdown := strings.Join(lines, "\n")
	return DigestIssue{
		DigestDate:     digestDate,
		ScheduledFor:   scheduledFor.UTC(),
		IdempotencyKey: "daily:" + digestDate + ":" + digestBodyFingerprint(bodyMarkdown),
		Subject:        subject,
		BodyMarkdown:   bodyMarkdown,
		Status:         "generated",
	}
}

func selectDigestInsights(generatedAt time.Time, insights []Insight, limit int) []Insight {
	insights = digestQualityInsights(insights)
	if limit <= 0 || len(insights) <= limit {
		return insights
	}
	selected := slices.Clone(insights)
	location := generatedAt.UTC()
	seed := fmt.Sprintf("%s:%02d:%d", location.Format("2006-01-02"), location.Hour()/2, len(insights))
	sort.SliceStable(selected, func(i int, j int) bool {
		return digestInsightPickScore(seed, selected[i]) < digestInsightPickScore(seed, selected[j])
	})
	picked := make([]Insight, 0, limit)
	used := map[string]bool{}
	for _, source := range []string{"youtube", "x"} {
		if len(picked) >= limit {
			break
		}
		for _, insight := range selected {
			if insight.Source == source && !used[insightDigestIdentity(insight)] {
				picked = append(picked, insight)
				used[insightDigestIdentity(insight)] = true
				break
			}
		}
	}
	for _, insight := range selected {
		if len(picked) >= limit {
			break
		}
		identity := insightDigestIdentity(insight)
		if used[identity] {
			continue
		}
		picked = append(picked, insight)
		used[identity] = true
	}
	return picked
}

func digestQualityInsights(insights []Insight) []Insight {
	filtered := []Insight{}
	for _, insight := range insights {
		if insight.Quality != nil && insight.Quality.Overall > 0 && insight.Quality.Overall < 0.7 {
			continue
		}
		filtered = append(filtered, insight)
	}
	if len(filtered) == 0 {
		return insights
	}
	return filtered
}

func digestInsightPickScore(seed string, insight Insight) uint64 {
	identity := insightDigestIdentity(insight)
	sum := sha256.Sum256([]byte(seed + ":" + identity))
	return binary.BigEndian.Uint64(sum[:8])
}

func insightDigestIdentity(insight Insight) string {
	return strings.Join([]string{insight.ID, insight.Source, insight.Title, insight.SourceURL, insight.Insight}, "|")
}

func parseDigestClock(value string) (int, int) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 18, 0
	}
	hour := parseSmallInt(parts[0], 18)
	minute := parseSmallInt(parts[1], 0)
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 18, 0
	}
	return hour, minute
}

func parseSmallInt(value string, fallbackValue int) int {
	parsed := 0
	if value == "" {
		return fallbackValue
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return fallbackValue
		}
		parsed = parsed*10 + int(r-'0')
	}
	return parsed
}

func digestBodyFingerprint(bodyMarkdown string) string {
	sum := sha256.Sum256([]byte(bodyMarkdown))
	return hex.EncodeToString(sum[:])[:12]
}

func truncateDigestText(value string, limit int) string {
	trimmed := strings.Join(strings.Fields(value), " ")
	if trimmed == "" || limit <= 0 {
		return ""
	}
	if limit <= 3 {
		return truncate(trimmed, limit)
	}
	if len([]rune(trimmed)) <= limit {
		return trimmed
	}
	truncated := strings.TrimSpace(truncate(trimmed, limit-3))
	if truncated == "" {
		return "..."
	}
	return truncated + "..."
}
