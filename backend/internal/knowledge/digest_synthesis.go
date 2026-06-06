package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	langfuseclient "github.com/abhijitmohanty/second-brain/backend/internal/platform/langfuse"
	"github.com/abhijitmohanty/second-brain/backend/prompts"
)

const digestPromptVersion = prompts.DigestPromptVersion
const digestPromptMaxOutputTokens = 5000

type digestPromptFormatError struct {
	err error
}

func (e *digestPromptFormatError) Error() string {
	return e.err.Error()
}

func (e *digestPromptFormatError) Unwrap() error {
	return e.err
}

type promptDigestResponse struct {
	Subject      string   `json:"subject"`
	BodyMarkdown string   `json:"body_markdown"`
	BodyLines    []string `json:"body_lines"`
}

type digestPromptPayload struct {
	Lines                  []string
	LangfusePromptName     string
	LangfusePromptVersion  int
	LangfusePromptLabel    string
	LangfusePromptSource   string
	LangfusePromptFallback string
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
	digest.IdempotencyKey = "daily:" + digest.DigestDate
	return digest, nil
}

func (s *Service) promptDigest(ctx context.Context, base DigestIssue, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) (promptDigestResponse, error) {
	inputJSON := truncate(digestPromptInput(summaries, insights, themes, insightClusters, connections), 16000)
	prompt := s.digestNewsletterPrompt(ctx, base, inputJSON)
	return s.promptDigestWithPrompt(ctx, s.cfg.OpenAISynthesisModel, prompt, digestPromptMaxOutputTokens)
}

func (s *Service) promptDigestWithLines(ctx context.Context, model string, promptLines []string, maxOutputTokens int) (promptDigestResponse, error) {
	return s.promptDigestWithPrompt(ctx, model, digestPromptPayload{
		Lines:                promptLines,
		LangfusePromptSource: "local",
	}, maxOutputTokens)
}

func (s *Service) promptDigestWithPrompt(ctx context.Context, model string, prompt digestPromptPayload, maxOutputTokens int) (promptDigestResponse, error) {
	if strings.TrimSpace(model) == "" {
		model = s.cfg.OpenAISynthesisModel
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 3000
	}
	payload, err := s.promptDigestOnce(ctx, model, prompt, maxOutputTokens)
	if err == nil {
		return payload, nil
	}
	var formatErr *digestPromptFormatError
	if !errors.As(err, &formatErr) {
		return promptDigestResponse{}, err
	}

	s.log(ctx).Warn("retrying digest newsletter synthesis after incomplete response", "error", err)
	retryPrompt := prompt
	retryPrompt.Lines = append([]string(nil), prompt.Lines...)
	retryPrompt.Lines = append(retryPrompt.Lines,
		"",
		"RETRY REQUIREMENT",
		"The previous response was incomplete or malformed JSON: "+err.Error(),
		"Return one complete valid JSON object only. Keep the essay concise enough to finish within the output limit.",
	)
	payload, retryErr := s.promptDigestOnce(ctx, model, retryPrompt, max(maxOutputTokens, digestPromptMaxOutputTokens))
	if retryErr != nil {
		return promptDigestResponse{}, fmt.Errorf("digest response retry after %v failed: %w", err, retryErr)
	}
	return payload, nil
}

func (s *Service) promptDigestOnce(ctx context.Context, model string, prompt digestPromptPayload, maxOutputTokens int) (promptDigestResponse, error) {
	promptText := strings.Join(prompt.Lines, "\n")
	metadata := map[string]string{
		"retry":         fmt.Sprintf("%t", containsPromptLine(prompt.Lines, "RETRY REQUIREMENT")),
		"prompt_source": prompt.LangfusePromptSource,
	}
	if prompt.LangfusePromptLabel != "" {
		metadata["prompt_label"] = prompt.LangfusePromptLabel
	}
	if prompt.LangfusePromptFallback != "" {
		metadata["prompt_fallback"] = prompt.LangfusePromptFallback
	}
	ctx, span := s.startObservationSpan(ctx, observationOptions{
		Name:          "digest-newsletter-synthesis",
		Type:          "generation",
		Model:         model,
		PromptVersion: digestPromptVersion,
		PromptName:    prompt.LangfusePromptName,
		PromptNumber:  prompt.LangfusePromptVersion,
		Tags:          []string{"digest", "newsletter", "generation"},
		Metadata:      metadata,
		InputSummary: map[string]any{
			"prompt_lines": len(prompt.Lines),
			"prompt_chars": len(promptText),
		},
		InputContent: promptText,
		ModelParams: map[string]any{
			"max_output_tokens": maxOutputTokens,
			"response_format":   "json_object",
		},
	})
	defer span.End()
	requestBody := map[string]any{
		"model":             model,
		"text":              map[string]any{"format": map[string]any{"type": "json_object"}},
		"input":             promptText,
		"max_output_tokens": maxOutputTokens,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		setSpanError(span, err)
		return promptDigestResponse{}, err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &response); err != nil {
		setSpanError(span, err)
		return promptDigestResponse{}, err
	}
	if response.Error != nil && response.Error.Message != "" {
		err := fmt.Errorf("%s", response.Error.Message)
		setSpanError(span, err)
		return promptDigestResponse{}, err
	}
	setOpenAIUsage(span, response.Usage)
	if strings.EqualFold(strings.TrimSpace(response.Status), "incomplete") {
		reason := "unknown"
		if response.IncompleteDetails != nil && strings.TrimSpace(response.IncompleteDetails.Reason) != "" {
			reason = strings.TrimSpace(response.IncompleteDetails.Reason)
		}
		err := &digestPromptFormatError{err: fmt.Errorf("model response incomplete: %s", reason)}
		setSpanError(span, err)
		return promptDigestResponse{}, err
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
	if strings.TrimSpace(text) == "" {
		err := &digestPromptFormatError{err: fmt.Errorf("model returned empty output")}
		setSpanError(span, err)
		return promptDigestResponse{}, err
	}
	var payload promptDigestResponse
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &payload); err != nil {
		formatErr := &digestPromptFormatError{err: fmt.Errorf("malformed model JSON: %w", err)}
		setSpanError(span, formatErr)
		return promptDigestResponse{}, formatErr
	}
	s.setSpanOutput(span, map[string]any{
		"subject_chars": len(payload.Subject),
		"body_lines":    len(payload.BodyLines),
		"body_chars":    len(payload.BodyMarkdown),
	}, text)
	return payload, nil
}

func (s *Service) digestNewsletterPrompt(ctx context.Context, base DigestIssue, inputJSON string) digestPromptPayload {
	localLines := prompts.AppendInputJSON(digestNewsletterPromptLines(base), inputJSON)
	localPrompt := digestPromptPayload{
		Lines:                localLines,
		LangfusePromptSource: "local",
	}
	if !s.cfg.LangfusePromptManagementEnabled || strings.TrimSpace(s.cfg.LangfuseBaseURL) == "" {
		return localPrompt
	}
	promptName := strings.TrimSpace(s.cfg.LangfuseDigestPromptName)
	if promptName == "" {
		promptName = "second-brain/digest-newsletter"
	}
	promptLabel := strings.TrimSpace(s.cfg.LangfusePromptLabel)
	if promptLabel == "" {
		promptLabel = "production"
	}
	client := langfuseclient.NewClient(s.cfg, s.client)
	managedPrompt, err := client.GetTextPrompt(ctx, promptName, promptLabel)
	if err != nil {
		s.log(ctx).Warn("using local digest prompt after langfuse prompt fetch failed", "name", promptName, "label", promptLabel, "error", err)
		localPrompt.LangfusePromptFallback = truncateTelemetryValue(err.Error())
		return localPrompt
	}
	compiled := langfuseclient.CompileTextPrompt(managedPrompt.Prompt, map[string]string{
		"digest_date": base.DigestDate,
		"input_json":  inputJSON,
	})
	return digestPromptPayload{
		Lines:                 strings.Split(compiled, "\n"),
		LangfusePromptName:    managedPrompt.Name,
		LangfusePromptVersion: managedPrompt.Version,
		LangfusePromptLabel:   promptLabel,
		LangfusePromptSource:  "langfuse",
	}
}

func containsPromptLine(lines []string, target string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
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
