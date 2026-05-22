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

func (s *Service) enrichProcessedSources(ctx context.Context, sources []ProcessedSource) []ProcessedSource {
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
		sources[index].Embeddings = s.embeddingRecordsForSource(ctx, sources[index])
	}
	return sources
}

func (s *Service) embeddingRecordsForSource(ctx context.Context, source ProcessedSource) []EmbeddingRecord {
	records := []EmbeddingRecord{}
	model := s.embeddingModel()

	if len(source.Chunks) > 0 {
		chunk := source.Chunks[0]
		chunkIndex := chunk.Index
		records = append(records, EmbeddingRecord{
			Type:       "chunk",
			Label:      source.Title,
			Model:      model,
			Dimensions: embeddingDimensions,
			Vector:     s.embeddingLiteral(ctx, chunk.Content),
			ChunkIndex: &chunkIndex,
		})
	}
	records = append(records, EmbeddingRecord{
		Type:       "summary",
		Label:      source.Title,
		Model:      model,
		Dimensions: embeddingDimensions,
		Vector:     s.embeddingLiteral(ctx, source.Synthesis.Summary.Summary),
	})
	for _, entity := range source.Entities {
		records = append(records, EmbeddingRecord{
			Type:       "entity",
			Label:      entity.Label,
			Model:      model,
			Dimensions: embeddingDimensions,
			Vector:     s.embeddingLiteral(ctx, entity.Label+" "+entity.Evidence),
		})
	}
	return records
}

func (s *Service) embeddingModel() string {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return "local-hash-embedding-v1"
	}
	return s.cfg.OpenAIEmbeddingModel
}

func (s *Service) embeddingLiteral(ctx context.Context, text string) string {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return deterministicEmbeddingLiteral(text)
	}

	requestBody := map[string]any{
		"model": s.cfg.OpenAIEmbeddingModel,
		"input": truncate(text, 6000),
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return deterministicEmbeddingLiteral(text)
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response embeddingResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/embeddings", headers, bytes.NewReader(raw), &response); err != nil {
		return deterministicEmbeddingLiteral(text)
	}
	if response.Error != nil && response.Error.Message != "" {
		return deterministicEmbeddingLiteral(text)
	}
	if len(response.Data) == 0 || len(response.Data[0].Embedding) != embeddingDimensions {
		return deterministicEmbeddingLiteral(text)
	}
	return vectorLiteral(response.Data[0].Embedding)
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

func buildDigestIssue(cfgDigestTimezone string, generatedAt time.Time, summaries []Summary, themes []ThemeCluster, connections []SourceConnection) DigestIssue {
	location, err := time.LoadLocation(cfgDigestTimezone)
	if err != nil {
		location = time.FixedZone("Asia/Singapore", 8*60*60)
	}
	localDate := generatedAt.In(location)
	scheduledFor := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 17, 0, 0, 0, location)
	digestDate := scheduledFor.Format("2006-01-02")
	subject := "Second Brain digest for " + digestDate
	lines := []string{"# " + subject, ""}
	if len(themes) > 0 {
		lines = append(lines, "## Themes")
		for _, theme := range themes {
			lines = append(lines, fmt.Sprintf("- %s: %.0f related source(s). %s", theme.Label, theme.Score, theme.Evidence))
		}
		lines = append(lines, "")
	}
	if len(summaries) > 0 {
		lines = append(lines, "## Source notes")
		for _, summary := range summaries {
			lines = append(lines, fmt.Sprintf("- [%s](%s): %s", summary.Title, summary.SourceURL, summary.Summary))
			if summary.Quote != "" {
				lines = append(lines, "  Evidence: "+summary.Quote)
			}
		}
		lines = append(lines, "")
	}
	if len(connections) > 0 {
		lines = append(lines, "## Connections")
		for _, connection := range connections {
			lines = append(lines, "- "+connection.Evidence)
		}
	}
	return DigestIssue{
		DigestDate:     digestDate,
		ScheduledFor:   scheduledFor.UTC(),
		IdempotencyKey: "daily:" + digestDate,
		Subject:        subject,
		BodyMarkdown:   strings.Join(lines, "\n"),
		Status:         "generated",
	}
}
