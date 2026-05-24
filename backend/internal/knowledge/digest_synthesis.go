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

	"github.com/abhijitmohanty/second-brain/backend/prompts"
)

const digestPromptVersion = prompts.DigestPromptVersion

type promptDigestResponse struct {
	Subject      string   `json:"subject"`
	BodyMarkdown string   `json:"body_markdown"`
	BodyLines    []string `json:"body_lines"`
}

func hasDigestInputs(summaries []Summary, insights []Insight) bool {
	return len(summaries) > 0 || len(insights) > 0
}

func (s *Service) composeDigestIssue(ctx context.Context, generatedAt time.Time, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) (DigestIssue, error) {
	digestInsights := selectDigestInsights(generatedAt, insights, digestMaxInsightCount)
	digest := buildDigestIssue(s.cfg.DigestTimezone, s.cfg.DigestTime, generatedAt, summaries, digestInsights, themes, insightClusters, connections)
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return DigestIssue{}, fmt.Errorf("OPENAI_API_KEY is required for digest newsletter synthesis")
	}
	payload, err := s.promptDigest(ctx, digest, summaries, digestInsights, themes, insightClusters, connections)
	if err != nil {
		return DigestIssue{}, fmt.Errorf("digest newsletter synthesis failed: %w", err)
	}
	if strings.TrimSpace(payload.Subject) == "" {
		return DigestIssue{}, fmt.Errorf("digest newsletter synthesis returned an empty subject")
	}
	digest.Subject = strings.TrimSpace(payload.Subject)
	bodyMarkdown := strings.TrimSpace(payload.BodyMarkdown)
	if bodyMarkdown == "" && len(payload.BodyLines) > 0 {
		bodyMarkdown = strings.TrimSpace(strings.Join(payload.BodyLines, "\n"))
	}
	digest.BodyMarkdown = ensureDigestSourceLinks(bodyMarkdown, digestInsights)
	if strings.TrimSpace(digest.BodyMarkdown) == "" {
		return DigestIssue{}, fmt.Errorf("digest newsletter synthesis returned an empty body")
	}
	digest.IdempotencyKey = "daily:" + digest.DigestDate + ":" + digestBodyFingerprint(digest.BodyMarkdown)
	if err := s.addDigestIllustration(ctx, &digest, digestInsights, themes, connections); err != nil {
		return DigestIssue{}, err
	}
	return digest, nil
}

func (s *Service) promptDigest(ctx context.Context, base DigestIssue, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) (promptDigestResponse, error) {
	promptLines := prompts.AppendInputJSON(
		digestNewsletterPromptLines(base),
		truncate(digestPromptInput(summaries, insights, themes, insightClusters, connections), 16000),
	)
	return s.promptDigestWithLines(ctx, s.cfg.OpenAISynthesisModel, promptLines, 3000)
}

func (s *Service) promptDigestWithLines(ctx context.Context, model string, promptLines []string, maxOutputTokens int) (promptDigestResponse, error) {
	if strings.TrimSpace(model) == "" {
		model = s.cfg.OpenAISynthesisModel
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 3000
	}
	requestBody := map[string]any{
		"model":             model,
		"input":             strings.Join(promptLines, "\n"),
		"max_output_tokens": maxOutputTokens,
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

func digestNewsletterPromptLines(base DigestIssue) []string {
	return prompts.DigestNewsletterLines(base.DigestDate)
}

func digestPromptInput(summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) string {
	payload := map[string]any{
		"summaries":              summaries[:min(len(summaries), digestMaxSourceNoteCount)],
		"insights":               insights[:min(len(insights), digestMaxInsightCount)],
		"themes":                 themes[:min(len(themes), 6)],
		"insight_clusters":       insightClusters[:min(len(insightClusters), digestMaxInsightClusterCount)],
		"connections":            connections[:min(len(connections), digestMaxConnectionCount)],
		"newsletter_style_notes": prompts.DigestNewsletterStyleNotes(),
		"requirements":           prompts.DigestNewsletterRequirements(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func normalizeDigestNewsletterBody(bodyMarkdown string) string {
	lines := strings.Split(strings.ReplaceAll(bodyMarkdown, "\r\n", "\n"), "\n")
	normalized := make([]string, 0, len(lines))
	h1Seen := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(normalized) > 0 && normalized[len(normalized)-1] != "" {
				normalized = append(normalized, "")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			if !h1Seen {
				normalized = append(normalized, trimmed)
				h1Seen = true
				continue
			}
			trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
		if strings.HasPrefix(trimmed, "##") {
			heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if isForbiddenDigestSectionLabel(heading) {
				continue
			}
			trimmed = heading
		}
		trimmed = stripListMarker(trimmed)
		normalized = append(normalized, trimmed)
	}
	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

func isForbiddenDigestSectionLabel(value string) bool {
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(value), ":. "))
	switch normalized {
	case "the lead", "the rest of the brief", "one thing to do next", "in this issue", "conclusion", "takeaway", "sources", "original sources", "what to read", "the newsletter":
		return true
	default:
		return false
	}
}

func stripListMarker(value string) string {
	if strings.HasPrefix(value, "- ") || strings.HasPrefix(value, "* ") {
		return strings.TrimSpace(value[2:])
	}
	for index, r := range value {
		if r < '0' || r > '9' {
			if index > 0 && (r == '.' || r == ')') && len(value) > index+1 && value[index+1] == ' ' {
				return strings.TrimSpace(value[index+2:])
			}
			break
		}
	}
	return value
}

func ensureDigestSourceLinks(bodyMarkdown string, insights []Insight) string {
	body := normalizeDigestNewsletterBody(bodyMarkdown)
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
		missing = append(missing, fmt.Sprintf("[%s](%s)", title, insight.SourceURL))
	}
	if len(missing) == 0 {
		return body
	}
	return body + "\n\nThe original pieces behind this story are " + inlineMarkdownList(missing) + "."
}

func inlineMarkdownList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + ", and " + values[len(values)-1]
	}
}
