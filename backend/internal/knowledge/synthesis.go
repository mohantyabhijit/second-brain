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

const synthesisPromptVersion = "source-grounded-insights-v1"
const extractiveSynthesisModel = "extractive-fallback-v1"

type promptSynthesisResponse struct {
	Decision   Decision `json:"decision"`
	Summary    string   `json:"summary"`
	Confidence string   `json:"confidence"`
	Quote      string   `json:"quote"`
	Insights   []struct {
		Title      string `json:"title"`
		Insight    string `json:"insight"`
		Evidence   string `json:"evidence"`
		Confidence string `json:"confidence"`
	} `json:"insights"`
	ActionItems []struct {
		Title     string `json:"title"`
		Action    string `json:"action"`
		Rationale string `json:"rationale"`
		Priority  string `json:"priority"`
	} `json:"action_items"`
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
		insights = append(insights, Insight{
			ID:          fmt.Sprintf("%s-%s-insight-%d", candidate.sourceType, candidate.externalID, index+1),
			Source:      string(candidate.sourceType),
			SourceID:    candidate.externalID,
			Title:       fallback(item.Title, "Insight"),
			Insight:     item.Insight,
			Evidence:    fallback(item.Evidence, summary.Quote),
			SourceURL:   candidate.sourceURL,
			Confidence:  normalizedConfidence(item.Confidence),
			CacheStatus: cacheStatus,
			GeneratedAt: &generatedAt,
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
			"Find the most insightful claims and practical action items.",
			"Do not add facts that are not supported by the source text.",
			"Use this JSON shape: {\"decision\":\"read_now|later|skip\",\"summary\":\"...\",\"confidence\":\"high|medium|low\",\"quote\":\"short supporting quote\",\"insights\":[{\"title\":\"...\",\"insight\":\"...\",\"evidence\":\"...\",\"confidence\":\"high|medium|low\"}],\"action_items\":[{\"title\":\"...\",\"action\":\"...\",\"rationale\":\"...\",\"priority\":\"high|medium|low\"}]}",
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
	insights := []struct {
		Title      string `json:"title"`
		Insight    string `json:"insight"`
		Evidence   string `json:"evidence"`
		Confidence string `json:"confidence"`
	}{}
	for _, sentence := range rankedSentences(sentences) {
		if len(insights) >= 3 {
			break
		}
		insights = append(insights, struct {
			Title      string `json:"title"`
			Insight    string `json:"insight"`
			Evidence   string `json:"evidence"`
			Confidence string `json:"confidence"`
		}{
			Title:      "Source-backed insight",
			Insight:    sentence,
			Evidence:   sentence,
			Confidence: summary.Confidence,
		})
	}
	if len(insights) == 0 && summary.Summary != "" {
		insights = append(insights, struct {
			Title      string `json:"title"`
			Insight    string `json:"insight"`
			Evidence   string `json:"evidence"`
			Confidence string `json:"confidence"`
		}{Title: "Source-backed insight", Insight: summary.Summary, Evidence: summary.Quote, Confidence: summary.Confidence})
	}

	action := "Review this source and extract one reusable note for the knowledge inbox."
	if candidate.sourceType == SourceTypeYouTube {
		action = "Turn the strongest transcript moment into a timestamped note or follow-up question."
	}
	actions := []struct {
		Title     string `json:"title"`
		Action    string `json:"action"`
		Rationale string `json:"rationale"`
		Priority  string `json:"priority"`
	}{{
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
