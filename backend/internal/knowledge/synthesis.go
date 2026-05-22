package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const synthesisPromptVersion = "source-grounded-insights-v2"
const extractiveSynthesisModel = "extractive-fallback-v1"

type promptSynthesisResponse struct {
	Decision    Decision        `json:"decision"`
	Summary     string          `json:"summary"`
	Confidence  string          `json:"confidence"`
	Quote       string          `json:"quote"`
	Insights    []promptInsight `json:"insights"`
	ActionItems []promptAction  `json:"action_items"`
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
}

type promptAction struct {
	Title     string `json:"title"`
	Action    string `json:"action"`
	Rationale string `json:"rationale"`
	Priority  string `json:"priority"`
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
	payload, err := s.promptSynthesis(ctx, candidate, model)
	if err != nil {
		payload = fallbackSynthesis(candidate)
	}

	summary := Summary{
		ID:            candidate.externalID,
		Source:        string(candidate.sourceType),
		Title:         candidate.title,
		SourceURL:     candidate.sourceURL,
		Decision:      normalizedDecision(payload.Decision),
		Summary:       fallback(payload.Summary, "No source-grounded summary could be produced."),
		Quote:         payload.Quote,
		Confidence:    normalizedConfidence(payload.Confidence),
		Notes:         []string{"Prompt synthesis version: " + synthesisPromptVersion + "."},
		CacheStatus:   cacheStatus,
		CaptureHash:   captureHash,
		PromptVersion: synthesisPromptVersion,
		Model:         model,
		GeneratedAt:   &generatedAt,
	}
	if summary.Quote == "" {
		summary.Quote = truncate(candidate.body, 280)
	}
	if err != nil {
		summary.Notes = append(summary.Notes, "Extractive fallback used because prompt synthesis was unavailable: "+err.Error())
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
		"input": strings.Join([]string{
			"You are the source-grounded synthesis module for a personal second brain.",
			"Read the source text and return JSON only.",
			"Extract multiple distinct, atomic insight candidates. A source can produce many insights; do not merge unrelated ideas.",
			"Each insight must be meaningful on its own, grounded in source evidence, and different from the other insights.",
			"Do not add facts that are not supported by the source text.",
			"Use this JSON shape: {\"decision\":\"read_now|later|skip\",\"summary\":\"...\",\"confidence\":\"high|medium|low\",\"quote\":\"short supporting quote\",\"insights\":[{\"title\":\"...\",\"insight\":\"raw human-readable insight\",\"raw_insight\":\"...\",\"canonical_insight\":\"normalized form for similarity search\",\"abstract_insight\":\"cross-domain abstraction\",\"practical_text\":\"optional action rule\",\"mechanism\":\"underlying mechanism, not just topic\",\"insight_type\":\"principle|warning|tactic|framework|prediction|tradeoff|critique|mental_model|trend|question|contradiction\",\"domain\":\"...\",\"topics\":[\"...\"],\"entities\":[\"...\"],\"evidence\":\"short quote or paraphrase\",\"evidence_refs\":[{\"quote\":\"...\"}],\"explicit_or_inferred\":\"explicit|inferred\",\"confidence\":\"high|medium|low\",\"importance_score\":0.0,\"novelty_score\":0.0,\"actionability_score\":0.0}],\"action_items\":[{\"title\":\"...\",\"action\":\"...\",\"rationale\":\"...\",\"priority\":\"high|medium|low\"}]}",
			"Source type: " + string(candidate.sourceType),
			"Source title: " + candidate.title,
			"Source URL: " + candidate.sourceURL,
			"",
			truncate(candidate.body, 12000),
		}, "\n"),
		"max_output_tokens": 1800,
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
