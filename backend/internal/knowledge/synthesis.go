package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/prompts"
)

const synthesisPromptVersion = prompts.SynthesisPromptVersion
const extractiveSynthesisModel = "extractive-fallback-v1"

type promptSynthesisResponse struct {
	Decision    Decision              `json:"decision"`
	Summary     string                `json:"summary"`
	Confidence  string                `json:"confidence"`
	Quote       string                `json:"quote"`
	Quality     QualityScore          `json:"quality"`
	TimeMarkers []ImportantTimeMarker `json:"important_time_markers"`
	Insights    []promptInsight       `json:"insights"`
	ActionItems []promptAction        `json:"action_items"`
}

type promptInsight struct {
	Title              string               `json:"title"`
	Insight            string               `json:"insight"`
	RawInsight         string               `json:"raw_insight"`
	CanonicalInsight   string               `json:"canonical_insight"`
	AbstractInsight    string               `json:"abstract_insight"`
	PracticalText      string               `json:"practical_text"`
	Mechanism          string               `json:"mechanism"`
	InsightType        string               `json:"insight_type"`
	Domain             string               `json:"domain"`
	Topics             []string             `json:"topics"`
	Entities           []string             `json:"entities"`
	Evidence           string               `json:"evidence"`
	EvidenceRefs       []InsightEvidenceRef `json:"evidence_refs"`
	ExplicitOrInferred string               `json:"explicit_or_inferred"`
	Confidence         string               `json:"confidence"`
	ImportanceScore    float64              `json:"importance_score"`
	NoveltyScore       float64              `json:"novelty_score"`
	ActionabilityScore float64              `json:"actionability_score"`
	Quality            QualityScore         `json:"quality"`
}

type promptAction struct {
	Title     string `json:"title"`
	Action    string `json:"action"`
	Rationale string `json:"rationale"`
	Priority  string `json:"priority"`
}

type promptJudgeResponse struct {
	OverallScore    float64                  `json:"overall_score"`
	Verdict         string                   `json:"verdict"`
	Rationale       string                   `json:"rationale"`
	RevisedResponse *promptSynthesisResponse `json:"revised_response"`
}

func (s *Service) synthesisModel() string {
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return extractiveSynthesisModel
	}
	return s.cfg.OpenAISynthesisModel
}

func (s *Service) synthesizeCandidate(ctx context.Context, candidate sourceCandidate, captureHash string, cacheStatus string) SynthesisRecord {
	model := s.synthesisModel()
	generatedAt := time.Now().UTC()
	payload, promptErr := s.promptSynthesis(ctx, candidate, model)
	var judgeErr error
	if promptErr == nil {
		payload, judgeErr = s.judgeSynthesis(ctx, candidate, model, payload)
	}
	if promptErr != nil {
		payload = fallbackSynthesis(candidate)
	}

	timeMarkers := normalizedTimeMarkers(payload.TimeMarkers, 8)
	if len(timeMarkers) == 0 && candidate.sourceType == SourceTypeYouTube {
		timeMarkers = extractTimeMarkers(candidate.body, 8)
	}

	summary := Summary{
		ID:                   candidate.externalID,
		Source:               string(candidate.sourceType),
		Title:                candidate.title,
		SourceURL:            candidate.sourceURL,
		Decision:             normalizedDecision(payload.Decision),
		Summary:              truncateSummary(fallback(payload.Summary, "No source-grounded summary could be produced.")),
		Quote:                truncateQuote(payload.Quote),
		Confidence:           normalizedConfidence(payload.Confidence),
		Notes:                []string{"Prompt synthesis version: " + synthesisPromptVersion + "."},
		Quality:              normalizedQualityScore(payload.Quality, 0.72),
		ImportantTimeMarkers: timeMarkers,
		CacheStatus:          cacheStatus,
		CaptureHash:          captureHash,
		PromptVersion:        synthesisPromptVersion,
		Model:                model,
		GeneratedAt:          &generatedAt,
	}
	if summary.Quote == "" {
		summary.Quote = truncateQuote(candidate.body)
	}
	if promptErr != nil {
		summary.Notes = append(summary.Notes, "Extractive fallback used because prompt synthesis was unavailable: "+promptErr.Error())
	}
	if judgeErr != nil {
		summary.Notes = append(summary.Notes, "LLM judge unavailable; synthesis kept first-pass output: "+judgeErr.Error())
	}

	insights := make([]Insight, 0, len(payload.Insights))
	for index, item := range payload.Insights {
		if strings.TrimSpace(item.Insight) == "" {
			continue
		}
		id := fmt.Sprintf("%s-%s-insight-%d", candidate.sourceType, candidate.externalID, index+1)
		rawInsight := fallback(item.RawInsight, item.Insight)
		canonicalInsight := fallback(item.CanonicalInsight, canonicalInsightText(rawInsight))
		abstractInsight := fallback(item.AbstractInsight, canonicalInsight)
		insightType := normalizedInsightType(item.InsightType)
		domain := fallback(strings.ToLower(strings.TrimSpace(item.Domain)), "general")
		topics := normalizedLabels(item.Topics, 8)
		if len(topics) == 0 {
			topics = topKeywords(rawInsight+" "+canonicalInsight, 6)
		}
		entities := normalizedLabels(item.Entities, 8)
		mechanism := fallback(item.Mechanism, canonicalInsight)
		evidence := fallback(item.Evidence, summary.Quote)
		evidenceRefs := item.EvidenceRefs
		if len(evidenceRefs) == 0 && strings.TrimSpace(evidence) != "" {
			evidenceRefs = []InsightEvidenceRef{{Quote: evidence}}
		}
		insights = append(insights, Insight{
			ID:                 id,
			Source:             string(candidate.sourceType),
			SourceID:           candidate.externalID,
			Title:              fallback(item.Title, "Insight"),
			Insight:            item.Insight,
			RawInsight:         rawInsight,
			CanonicalInsight:   canonicalInsight,
			AbstractInsight:    abstractInsight,
			PracticalText:      item.PracticalText,
			Mechanism:          mechanism,
			InsightType:        insightType,
			Domain:             domain,
			Topics:             topics,
			Entities:           entities,
			Evidence:           evidence,
			EvidenceRefs:       normalizedEvidenceRefs(evidenceRefs),
			SourceURL:          candidate.sourceURL,
			Confidence:         normalizedConfidence(item.Confidence),
			ExplicitOrInferred: normalizedExplicitOrInferred(item.ExplicitOrInferred),
			ImportanceScore:    normalizedScore(item.ImportanceScore, 0.5),
			NoveltyScore:       normalizedScore(item.NoveltyScore, 0.5),
			ActionabilityScore: normalizedScore(item.ActionabilityScore, 0.5),
			Quality:            normalizedQualityScore(item.Quality, summary.Quality.Overall),
			EmbeddingText:      insightEmbeddingText(canonicalInsight, domain, insightType, topics),
			CacheStatus:        cacheStatus,
			GeneratedAt:        &generatedAt,
		})
	}

	actionItems := make([]ActionItem, 0, len(payload.ActionItems))
	for index, item := range payload.ActionItems {
		if strings.TrimSpace(item.Action) == "" {
			continue
		}
		actionItems = append(actionItems, ActionItem{
			ID:          fmt.Sprintf("%s-%s-action-%d", candidate.sourceType, candidate.externalID, index+1),
			Source:      string(candidate.sourceType),
			SourceID:    candidate.externalID,
			Title:       fallback(item.Title, "Action item"),
			Action:      item.Action,
			Rationale:   fallback(item.Rationale, "Grounded in the source evidence."),
			SourceURL:   candidate.sourceURL,
			Priority:    normalizedPriority(item.Priority),
			CacheStatus: cacheStatus,
			GeneratedAt: &generatedAt,
		})
	}

	return SynthesisRecord{
		SourceType:    candidate.sourceType,
		ExternalID:    candidate.externalID,
		CaptureHash:   captureHash,
		PromptVersion: synthesisPromptVersion,
		Model:         model,
		Summary:       summary,
		Insights:      insights,
		ActionItems:   actionItems,
		GeneratedAt:   generatedAt,
	}
}

func (s *Service) promptSynthesis(ctx context.Context, candidate sourceCandidate, model string) (promptSynthesisResponse, error) {
	if model == extractiveSynthesisModel {
		return promptSynthesisResponse{}, fmt.Errorf("OPENAI_API_KEY is not present")
	}
	requestBody := map[string]any{
		"model": model,
		"text":  map[string]any{"format": map[string]any{"type": "json_object"}},
		"input": prompts.SourceSynthesis(
			string(candidate.sourceType),
			candidate.title,
			candidate.sourceURL,
			truncate(candidate.body, 12000),
		),
		"max_output_tokens": 4500,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return promptSynthesisResponse{}, err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &response); err != nil {
		return promptSynthesisResponse{}, err
	}
	if response.Error != nil && response.Error.Message != "" {
		return promptSynthesisResponse{}, fmt.Errorf(response.Error.Message)
	}
	text := response.OutputText
	if strings.TrimSpace(text) == "" {
		parts := []string{}
		for _, output := range response.Output {
			for _, content := range output.Content {
				if strings.TrimSpace(content.Text) != "" {
					parts = append(parts, strings.TrimSpace(content.Text))
				}
			}
		}
		text = strings.Join(parts, "\n")
	}
	var payload promptSynthesisResponse
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &payload); err != nil {
		compactPayload, retryErr := s.promptCompactSynthesis(ctx, candidate, model, err)
		if retryErr == nil {
			return compactPayload, nil
		}
		return promptSynthesisResponse{}, err
	}
	return payload, nil
}

func (s *Service) promptCompactSynthesis(ctx context.Context, candidate sourceCandidate, model string, parseErr error) (promptSynthesisResponse, error) {
	requestBody := map[string]any{
		"model": model,
		"text":  map[string]any{"format": map[string]any{"type": "json_object"}},
		"input": prompts.CompactSourceSynthesis(
			string(candidate.sourceType),
			candidate.title,
			candidate.sourceURL,
			truncate(candidate.body, 5000),
			parseErr.Error(),
		),
		"max_output_tokens": 1600,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return promptSynthesisResponse{}, err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &response); err != nil {
		return promptSynthesisResponse{}, err
	}
	if response.Error != nil && response.Error.Message != "" {
		return promptSynthesisResponse{}, fmt.Errorf(response.Error.Message)
	}
	text := response.OutputText
	if strings.TrimSpace(text) == "" {
		parts := []string{}
		for _, output := range response.Output {
			for _, content := range output.Content {
				if strings.TrimSpace(content.Text) != "" {
					parts = append(parts, strings.TrimSpace(content.Text))
				}
			}
		}
		text = strings.Join(parts, "\n")
	}
	var payload promptSynthesisResponse
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &payload); err != nil {
		return promptSynthesisResponse{}, err
	}
	return payload, nil
}

func (s *Service) judgeSynthesis(ctx context.Context, candidate sourceCandidate, model string, payload promptSynthesisResponse) (promptSynthesisResponse, error) {
	if !s.synthesisJudgeEnabled(model) {
		return payload, nil
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return payload, err
	}
	requestBody := map[string]any{
		"model": model,
		"text":  map[string]any{"format": map[string]any{"type": "json_object"}},
		"input": prompts.SourceSynthesisJudge(
			string(candidate.sourceType),
			candidate.title,
			candidate.sourceURL,
			truncate(candidate.body, 9000),
			string(payloadJSON),
		),
		"max_output_tokens": 2500,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return payload, err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &response); err != nil {
		return payload, err
	}
	if response.Error != nil && response.Error.Message != "" {
		return payload, fmt.Errorf(response.Error.Message)
	}
	text := response.OutputText
	if strings.TrimSpace(text) == "" {
		parts := []string{}
		for _, output := range response.Output {
			for _, content := range output.Content {
				if strings.TrimSpace(content.Text) != "" {
					parts = append(parts, strings.TrimSpace(content.Text))
				}
			}
		}
		text = strings.Join(parts, "\n")
	}
	var judge promptJudgeResponse
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &judge); err != nil {
		return payload, err
	}
	if judge.RevisedResponse != nil {
		revised := *judge.RevisedResponse
		if revised.Quality.Overall == 0 {
			revised.Quality = QualityScore{
				Overall:     normalizedScore(judge.OverallScore, 0.82),
				Conciseness: normalizedScore(judge.OverallScore, 0.82),
				Efficacy:    normalizedScore(judge.OverallScore, 0.82),
				Grounding:   normalizedScore(judge.OverallScore, 0.82),
				Novelty:     normalizedScore(judge.OverallScore, 0.72),
				Verdict:     fallback(judge.Verdict, "revised"),
				Rationale:   judge.Rationale,
			}
		}
		return revised, nil
	}
	if payload.Quality.Overall == 0 {
		payload.Quality = QualityScore{
			Overall:     normalizedScore(judge.OverallScore, 0.82),
			Conciseness: normalizedScore(judge.OverallScore, 0.82),
			Efficacy:    normalizedScore(judge.OverallScore, 0.82),
			Grounding:   normalizedScore(judge.OverallScore, 0.82),
			Novelty:     normalizedScore(judge.OverallScore, 0.72),
			Verdict:     fallback(judge.Verdict, "pass"),
			Rationale:   judge.Rationale,
		}
	}
	return payload, nil
}

func (s *Service) synthesisJudgeEnabled(model string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("OPENAI_SYNTHESIS_JUDGE_ENABLED")))
	if value == "false" || value == "0" || value == "off" {
		return false
	}
	if value == "true" || value == "1" || value == "on" {
		return true
	}
	return strings.Contains(strings.ToLower(model), "gpt-5")
}

func fallbackSynthesis(candidate sourceCandidate) promptSynthesisResponse {
	summary := extractiveSummary(rawSummaryInput{
		ID:        candidate.externalID,
		Source:    string(candidate.sourceType),
		Title:     candidate.title,
		SourceURL: candidate.sourceURL,
		Body:      candidate.body,
		Quote:     candidate.body,
	})
	sentences := sentenceSplit(candidate.body)
	insights := []promptInsight{}
	for _, sentence := range rankedSentences(sentences) {
		if len(insights) >= 3 {
			break
		}
		insights = append(insights, promptInsight{
			Title:              "Source-backed insight",
			Insight:            sentence,
			CanonicalInsight:   canonicalInsightText(sentence),
			Mechanism:          canonicalInsightText(sentence),
			InsightType:        "principle",
			Domain:             "general",
			Evidence:           sentence,
			ExplicitOrInferred: "explicit",
			Confidence:         summary.Confidence,
			ImportanceScore:    0.5,
			NoveltyScore:       0.4,
			ActionabilityScore: 0.4,
			Quality:            fallbackQualityScore(0.58, "extractive fallback"),
		})
	}
	if len(insights) == 0 && summary.Summary != "" {
		insights = append(insights, promptInsight{
			Title:              "Source-backed insight",
			Insight:            summary.Summary,
			CanonicalInsight:   canonicalInsightText(summary.Summary),
			Mechanism:          canonicalInsightText(summary.Summary),
			InsightType:        "principle",
			Domain:             "general",
			Evidence:           summary.Quote,
			ExplicitOrInferred: "inferred",
			Confidence:         summary.Confidence,
			ImportanceScore:    0.5,
			NoveltyScore:       0.4,
			ActionabilityScore: 0.4,
			Quality:            fallbackQualityScore(0.52, "extractive fallback"),
		})
	}

	action := "Review this source and extract one reusable note for the knowledge inbox."
	if candidate.sourceType == SourceTypeYouTube {
		action = "Turn the strongest transcript moment into a timestamped note or follow-up question."
	}
	actions := []promptAction{{
		Title:     "Follow up",
		Action:    action,
		Rationale: "The source contains enough concrete signal to preserve for later synthesis.",
		Priority:  priorityForDecision(summary.Decision),
	}}

	return promptSynthesisResponse{
		Decision:    summary.Decision,
		Summary:     summary.Summary,
		Confidence:  summary.Confidence,
		Quote:       summary.Quote,
		Quality:     fallbackQualityScore(0.55, "extractive fallback"),
		TimeMarkers: extractTimeMarkers(candidate.body, 6),
		Insights:    insights,
		ActionItems: actions,
	}
}

func rankedSentences(sentences []string) []string {
	scored := []string{}
	for _, sentence := range sentences {
		if len(scored) >= 3 {
			break
		}
		if decisionFor(sentence) == DecisionReadNow || len(sentence) > 90 {
			scored = append(scored, sentence)
		}
	}
	if len(scored) == 0 && len(sentences) > 0 {
		scored = append(scored, sentences[0])
	}
	return scored
}

func normalizedDecision(value Decision) Decision {
	switch value {
	case DecisionReadNow, DecisionLater, DecisionSkip:
		return value
	default:
		return DecisionLater
	}
}

func normalizedConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "low"
	}
}

func normalizedPriority(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func normalizedInsightType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "principle", "warning", "tactic", "framework", "prediction", "tradeoff", "critique", "mental_model", "trend", "question", "contradiction":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "principle"
	}
}

func normalizedExplicitOrInferred(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "explicit", "inferred":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "inferred"
	}
}

func normalizedScore(value float64, fallbackValue float64) float64 {
	if value == 0 {
		value = fallbackValue
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizedQualityScore(value QualityScore, fallbackOverall float64) *QualityScore {
	overall := normalizedScore(value.Overall, fallbackOverall)
	quality := &QualityScore{
		Overall:     overall,
		Conciseness: normalizedScore(value.Conciseness, overall),
		Efficacy:    normalizedScore(value.Efficacy, overall),
		Grounding:   normalizedScore(value.Grounding, overall),
		Novelty:     normalizedScore(value.Novelty, minFloat(overall, 0.75)),
		Verdict:     strings.TrimSpace(value.Verdict),
		Rationale:   truncateSummary(value.Rationale),
	}
	if quality.Verdict == "" {
		switch {
		case overall >= 0.82:
			quality.Verdict = "pass"
		case overall >= 0.6:
			quality.Verdict = "revise"
		default:
			quality.Verdict = "weak_source"
		}
	}
	return quality
}

func fallbackQualityScore(overall float64, rationale string) QualityScore {
	return QualityScore{
		Overall:     overall,
		Conciseness: overall,
		Efficacy:    overall,
		Grounding:   overall,
		Novelty:     minFloat(overall, 0.65),
		Verdict:     "weak_source",
		Rationale:   rationale,
	}
}

func normalizedTimeMarkers(markers []ImportantTimeMarker, limit int) []ImportantTimeMarker {
	normalized := []ImportantTimeMarker{}
	seen := map[int]bool{}
	for _, marker := range markers {
		label := strings.TrimSpace(marker.Label)
		why := truncateSummary(marker.WhyItMatters)
		quote := truncateQuote(marker.Quote)
		seconds := marker.Seconds
		if seconds <= 0 {
			if parsed, ok := parseTimestampSeconds(marker.Timestamp); ok {
				seconds = parsed
			}
		}
		if seconds < 0 || why == "" || seen[seconds] {
			continue
		}
		seen[seconds] = true
		if label == "" {
			label = "Important moment"
		}
		normalized = append(normalized, ImportantTimeMarker{
			Label:        label,
			Timestamp:    formatTimeMarkerTimestamp(seconds),
			Seconds:      seconds,
			WhyItMatters: why,
			Quote:        quote,
		})
		if len(normalized) >= limit {
			break
		}
	}
	return normalized
}

var timestampPattern = regexp.MustCompile(`[\[(](\d{1,2}:\d{2}(?::\d{2})?)[\])]\s*([^\n]+)`)
var sentencePattern = regexp.MustCompile(`[^.!?\n]+[.!?]?`)

func extractTimeMarkers(text string, limit int) []ImportantTimeMarker {
	if limit <= 0 {
		return nil
	}
	matches := timestampPattern.FindAllStringSubmatch(text, -1)
	markers := []ImportantTimeMarker{}
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		seconds, ok := parseTimestampSeconds(match[1])
		if !ok {
			continue
		}
		quote := strings.TrimSpace(match[2])
		if quote == "" {
			continue
		}
		markers = append(markers, ImportantTimeMarker{
			Label:        "Transcript moment",
			Timestamp:    formatTimeMarkerTimestamp(seconds),
			Seconds:      seconds,
			WhyItMatters: truncateSummary(quote),
			Quote:        truncateQuote(quote),
		})
		if len(markers) >= limit {
			break
		}
	}
	return normalizedTimeMarkers(markers, limit)
}

func estimatedTranscriptMarkers(text string, durationSeconds int, limit int) []ImportantTimeMarker {
	clean := strings.Join(strings.Fields(text), " ")
	if clean == "" || durationSeconds <= 0 || limit <= 0 {
		return nil
	}
	indexes := sentencePattern.FindAllStringIndex(clean, -1)
	if len(indexes) == 0 {
		return nil
	}
	targets := []float64{0.14, 0.38, 0.62, 0.82}
	markers := []ImportantTimeMarker{}
	used := map[int]bool{}
	for _, target := range targets {
		if len(markers) >= limit {
			break
		}
		targetIndex := int(float64(len(clean)) * target)
		bestIndex := -1
		bestDistance := len(clean) + 1
		for index, bounds := range indexes {
			if used[index] {
				continue
			}
			sentence := strings.TrimSpace(clean[bounds[0]:bounds[1]])
			if len(sentence) < 36 {
				continue
			}
			distance := absInt(bounds[0] - targetIndex)
			if distance < bestDistance {
				bestIndex = index
				bestDistance = distance
			}
		}
		if bestIndex < 0 {
			continue
		}
		used[bestIndex] = true
		bounds := indexes[bestIndex]
		sentence := strings.TrimSpace(clean[bounds[0]:bounds[1]])
		seconds := int((float64(bounds[0]) / float64(len(clean))) * float64(durationSeconds))
		if seconds > durationSeconds {
			seconds = durationSeconds
		}
		markers = append(markers, ImportantTimeMarker{
			Label:        markerLabel(sentence),
			Timestamp:    formatTimeMarkerTimestamp(seconds),
			Seconds:      seconds,
			WhyItMatters: truncateSummary(sentence),
			Quote:        truncateQuote(sentence),
		})
	}
	return normalizedTimeMarkers(markers, limit)
}

func markerLabel(sentence string) string {
	words := strings.Fields(strings.Trim(sentence, " .!?"))
	if len(words) == 0 {
		return "Transcript moment"
	}
	limit := minInt(len(words), 5)
	return strings.Trim(strings.Join(words[:limit], " "), " ,:;-")
}

func parseTimestampSeconds(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := 0
	for _, part := range parts {
		if part == "" {
			return 0, false
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return 0, false
		}
		total = total*60 + number
	}
	return total, true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func formatTimeMarkerTimestamp(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	rest := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, rest)
	}
	return fmt.Sprintf("%d:%02d", minutes, rest)
}

func truncateSummary(value string) string {
	return truncate(strings.Join(strings.Fields(value), " "), 420)
}

func truncateQuote(value string) string {
	return truncate(strings.Join(strings.Fields(value), " "), 320)
}

func minFloat(left float64, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func normalizedLabels(values []string, limit int) []string {
	seen := map[string]bool{}
	labels := []string{}
	for _, value := range values {
		label := strings.ToLower(strings.TrimSpace(value))
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
		if len(labels) >= limit {
			break
		}
	}
	return labels
}

func normalizedEvidenceRefs(refs []InsightEvidenceRef) []InsightEvidenceRef {
	normalized := []InsightEvidenceRef{}
	for _, ref := range refs {
		ref.Quote = strings.TrimSpace(ref.Quote)
		ref.ChunkID = strings.TrimSpace(ref.ChunkID)
		if ref.Quote == "" {
			continue
		}
		normalized = append(normalized, ref)
	}
	return normalized
}

func canonicalInsightText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSuffix(value, ".") + "."
}

func insightEmbeddingText(canonical string, domain string, insightType string, topics []string) string {
	parts := []string{strings.TrimSpace(canonical)}
	if strings.TrimSpace(domain) != "" {
		parts = append(parts, "Domain: "+strings.TrimSpace(domain)+".")
	}
	if strings.TrimSpace(insightType) != "" {
		parts = append(parts, "Type: "+strings.TrimSpace(insightType)+".")
	}
	if len(topics) > 0 {
		parts = append(parts, "Topics: "+strings.Join(topics, ", ")+".")
	}
	return strings.Join(nonEmpty(parts), " ")
}

func priorityForDecision(value Decision) string {
	if value == DecisionReadNow {
		return "high"
	}
	if value == DecisionSkip {
		return "low"
	}
	return "medium"
}

func extractJSONObject(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return trimmed[start : end+1]
	}
	return trimmed
}
