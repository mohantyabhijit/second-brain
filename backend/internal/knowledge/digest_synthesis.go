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

const digestPromptVersion = "five-insight-brief-v1"

type promptDigestResponse struct {
	Subject      string   `json:"subject"`
	BodyMarkdown string   `json:"body_markdown"`
	BodyLines    []string `json:"body_lines"`
}

func (s *Service) composeDigestIssue(ctx context.Context, generatedAt time.Time, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) DigestIssue {
	digestInsights := selectDigestInsights(generatedAt, insights, digestMaxInsightCount)
	digest := buildDigestIssue(s.cfg.DigestTimezone, s.cfg.DigestTime, generatedAt, summaries, digestInsights, themes, insightClusters, connections)
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return digest
	}
	payload, err := s.promptDigest(ctx, digest, summaries, digestInsights, themes, insightClusters, connections)
	if err != nil {
		s.logger.Warn("newsletter prompt synthesis failed; using structured digest", "error", err)
		return digest
	}
	if strings.TrimSpace(payload.Subject) != "" {
		digest.Subject = strings.TrimSpace(payload.Subject)
	}
	bodyMarkdown := strings.TrimSpace(payload.BodyMarkdown)
	if bodyMarkdown == "" && len(payload.BodyLines) > 0 {
		bodyMarkdown = strings.TrimSpace(strings.Join(payload.BodyLines, "\n"))
	}
	if bodyMarkdown != "" {
		digest.BodyMarkdown = ensureDigestSourceLinks(bodyMarkdown, digestInsights)
		digest.IdempotencyKey = "daily:" + digest.DigestDate + ":" + digestBodyFingerprint(digest.BodyMarkdown)
	}
	return digest
}

func (s *Service) promptDigest(ctx context.Context, base DigestIssue, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) (promptDigestResponse, error) {
	requestBody := map[string]any{
		"model": s.cfg.OpenAISynthesisModel,
		"input": strings.Join([]string{
			"You are the editor of a personal Second Brain newsletter.",
			"Write a short, crisp personal newsletter from exactly five saved insights when five are provided.",
			"Use a simple format: one sharp subject, one brief opener, five numbered insight bullets, and one concrete next move.",
			"Keep the full body under 320 words. Paragraphs are one sentence. Bullets are short enough for a phone screen.",
			"Write like a thoughtful human editor: direct, specific, lightly conversational, never cheesy.",
			"Make it interesting through clear contrast and useful framing, not hype, emojis, memes, or unsupported jokes.",
			"Subject should be 30-50 characters and preview-friendly. The first paragraph should be one crisp sentence about the pattern in the five insights.",
			"Stay source-grounded. Do not add facts, claims, quotes, links, or named entities that are not present in the input.",
			"Every insight bullet must link the insight title directly to the original X bookmark or YouTube video sourceUrl. Do not include an insight bullet without a markdown source link.",
			"End with one concrete next move, not a motivational sign-off.",
			"Return JSON only with this shape: {\"subject\":\"short inbox-ready subject\",\"body_lines\":[\"# title\",\"\",\"short opener\",\"\",\"## Five Signals\",\"1. ...\"]}",
			"Put each markdown line in body_lines as a separate JSON string. Do not return a multiline body_markdown string.",
			"Use markdown links for sources exactly as provided.",
			"Prompt version: " + digestPromptVersion,
			"Digest date: " + base.DigestDate,
			"",
			"INPUT JSON:",
			truncate(digestPromptInput(summaries, insights, themes, insightClusters, connections), 16000),
		}, "\n"),
		"max_output_tokens": 1200,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return promptDigestResponse{}, err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &response); err != nil {
		return promptDigestResponse{}, err
	}
	if response.Error != nil && response.Error.Message != "" {
		return promptDigestResponse{}, fmt.Errorf(response.Error.Message)
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
	var payload promptDigestResponse
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &payload); err != nil {
		return promptDigestResponse{}, err
	}
	return payload, nil
}

func digestPromptInput(summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) string {
	payload := map[string]any{
		"summaries":        summaries[:min(len(summaries), digestMaxSourceNoteCount)],
		"insights":         insights[:min(len(insights), digestMaxInsightCount)],
		"themes":           themes[:min(len(themes), 6)],
		"insight_clusters": insightClusters[:min(len(insightClusters), digestMaxInsightClusterCount)],
		"connections":      connections[:min(len(connections), digestMaxConnectionCount)],
		"requirements":     []string{"exactly five insights when five are provided", "short and crisp", "simple newsletter format", "source grounded", "human newsletter voice", "phone first", "every insight has markdown source link", "keep links intact", "no unsupported facts"},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func ensureDigestSourceLinks(bodyMarkdown string, insights []Insight) string {
	body := strings.TrimSpace(bodyMarkdown)
	if body == "" {
		return body
	}
	missing := []string{}
	for _, insight := range insights {
		if len(missing) >= digestMaxInsightCount {
			break
		}
		if strings.TrimSpace(insight.SourceURL) == "" || strings.Contains(body, insight.SourceURL) {
			continue
		}
		title := fallback(insight.Title, "Source-backed insight")
		missing = append(missing, fmt.Sprintf("- [%s](%s)", title, insight.SourceURL))
	}
	if len(missing) == 0 {
		return body
	}
	return body + "\n\n## Original Sources\n" + strings.Join(missing, "\n")
}
