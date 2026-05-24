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

const digestPromptVersion = "newsletter-editorial-v2"

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
			"You are the editor of Abhijit's Second Brain, a personal research newsletter built from saved X bookmarks and YouTube videos.",
			"Write a complete newsletter issue, not a summary dump and not a status report.",
			"Use the editorial lessons from high-retention newsletters sampled from Gmail: a clear masthead, a warm 'welcome back' opener, a context paragraph that explains why this set matters now, curiosity-driven section heads, plain-language explainers, tight source-grounding, and a useful reader action at the end.",
			"Blend these patterns: The Ken's narrative lead and named sections, Finshots' simple question-to-explanation flow, Morning Brew-style scannability, and The Atlas-style context setting before the lead story. Do not copy their wording, jokes, branding, or structure wholesale.",
			"Use this format: one sharp subject; '# Abhijit's Second Brain - <date>'; a 2-3 sentence welcome/context opener; '## The Lead' for the strongest idea; '## The Rest Of The Brief' for the remaining source-grounded ideas; and '## One Thing To Do Next'.",
			"Do not create an 'In This Issue' section, a teaser list, or a block of source cards. Move straight from context into the lead story.",
			"Each section should have a '### <specific question or tension>' heading, a short setup paragraph, a why-it-matters paragraph, and a natural source link using the original insight title. Never use link text that only says 'Source'.",
			"Keep the full body between 550 and 850 words when five insights are available. Paragraphs stay short enough for a phone screen.",
			"Write like a thoughtful human editor: direct, specific, curious, and useful. Avoid hype, emojis, memes, generic life advice, and unsupported jokes.",
			"Subject should be 35-65 characters and preview-friendly. The first paragraph should be one crisp sentence about the pattern connecting the five insights.",
			"Stay source-grounded. Do not add facts, claims, quotes, links, or named entities that are not present in the input.",
			"Every newsletter section must include or end with a markdown link to the original X bookmark or YouTube video sourceUrl. Do not include an insight without a source link.",
			"End with one concrete next move that helps Abhijit turn one idea into a note, decision, or experiment.",
			"Return JSON only with this shape: {\"subject\":\"inbox-ready subject\",\"body_lines\":[\"# Abhijit's Second Brain - 2026-05-24\",\"\",\"Welcome back. ...\",\"\",\"## The Lead\",\"### ...\"]}",
			"Put each markdown line in body_lines as a separate JSON string. Do not return a multiline body_markdown string.",
			"Use markdown links for sources exactly as provided.",
			"Prompt version: " + digestPromptVersion,
			"Digest date: " + base.DigestDate,
			"",
			"INPUT JSON:",
			truncate(digestPromptInput(summaries, insights, themes, insightClusters, connections), 16000),
		}, "\n"),
		"max_output_tokens": 3000,
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
		"newsletter_style_notes": []string{
			"Open with a human editorial frame before listing items.",
			"Create context before details: name the problem, why it matters now, and what changed.",
			"Give every item a reason to care, not just a compressed summary.",
			"Use question-led or tension-led section titles when the source material supports it.",
			"Do not use an agenda card section or repeated 'Source' links.",
			"Keep paragraphs short, concrete, and source-linked.",
			"Close with one practical next move rather than a generic sign-off.",
		},
		"requirements": []string{"exactly five insights when five are provided", "complete newsletter issue", "source grounded", "human editorial voice", "phone first", "context before details", "no In This Issue section", "no repeated Source-only links", "every insight has markdown source link", "keep links intact", "no unsupported facts"},
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
