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

const digestPromptVersion = "newsletter-stratechery-story-v5"

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
	promptLines := append(digestNewsletterPromptLines(base),
		"",
		"INPUT JSON:",
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
	return []string{
		"You are the editor of Abhijit's Second Brain, a personal research newsletter built from saved X bookmarks and YouTube videos.",
		"Write a complete newsletter essay, not a summary dump, status report, agenda, source list, or labeled brief.",
		"",
		"STYLE STUDY TO APPLY",
		"A Gmail review of Stratechery emails from email@stratechery.com showed two useful shapes: weekly issues that quickly frame the week's best ideas before explaining why each mattered, and article issues that build one argument from hook to context to thesis to evidence to implication. Use those mechanics without copying Stratechery's wording, branding, recurring lines, paywall language, title formulas, or section structure.",
		"The lesson is not to imitate layout. The lesson is to make the reader understand the stakes before the facts, then make every fact deepen the central argument.",
		"",
		"REQUIRED NEWSLETTER SHAPE",
		"Return one sharp subject and one coherent markdown body.",
		"The body starts with exactly one H1: '# Abhijit's Second Brain - <date>'.",
		"After the H1, write 7-11 short paragraphs that read as a single story.",
		"Paragraph 1 is a strong hook: one crisp sentence naming the tension, surprise, or question that connects today's saved sources.",
		"Paragraph 2 creates context: why this pattern matters now, what changed, and what a smart reader might otherwise miss.",
		"Paragraphs 3-7 build the argument with source-grounded facts. Move from the strongest idea to supporting evidence, then to contrast, then to synthesis.",
		"The final paragraph resolves the story and gives Abhijit one concrete next move: a note to write, a decision to revisit, or an experiment to run. Do not label it as a conclusion or action item.",
		"",
		"EDITORIAL STANDARD",
		"Make the newsletter feel like a human editor found the connective tissue between the inputs, not like an AI compressed five tabs.",
		"Use causality, contrast, and stakes: show what changed, who or what benefits, what becomes fragile, and why the pattern travels across domains.",
		"Prefer simple sentences when explaining mechanics, but do not flatten the argument. The reader should finish with a clearer model, not just a shorter reading list.",
		"Synthesize across inputs. Do not recap one source at a time unless the narrative genuinely needs that order.",
		"Write with calm authority: specific, curious, plain-spoken, and useful. Avoid hype, emojis, memes, generic life advice, filler transitions, and unsupported jokes.",
		"Keep the full body between 650 and 950 words when five insights are available. Keep paragraphs short enough for a phone screen.",
		"The issue will be paired with a simple black-on-white editorial sketch. Do not describe the illustration in the copy; make the writing stand on its own.",
		"",
		"GROUNDING RULES",
		"Use only facts, claims, named entities, numbers, quotes, and links from the supplied summaries, insights, evidence, themes, clusters, and connections.",
		"Every insight used must have a natural markdown link to its original X bookmark or YouTube sourceUrl, using the original insight or source title as link text.",
		"Never use link text that only says 'Source', 'Read more', or 'here'.",
		"Do not include a separate source section. Links should appear inside the prose at the point where the fact is used.",
		"If the inputs are thin, be honest by writing a tighter essay from the available evidence rather than inventing context.",
		"",
		"STRICT FORMAT BANS",
		"Do not use markdown headings after the H1.",
		"Never write section labels such as 'The Lead', 'The Rest Of The Brief', 'One Thing To Do Next', 'In This Issue', 'Conclusion', 'Takeaway', 'Sources', 'What To Read', or 'The Newsletter'.",
		"Do not use bullets, numbered lists, teaser cards, agenda blocks, quote cards, or source cards.",
		"",
		"OUTPUT CONTRACT",
		"Return JSON only with this shape: {\"subject\":\"inbox-ready subject\",\"body_lines\":[\"# Abhijit's Second Brain - 2026-05-24\",\"\",\"The surprising thing about today's saved ideas is ...\"]}",
		"Put each markdown line in body_lines as a separate JSON string. Do not return a multiline body_markdown string.",
		"Subject should be 35-65 characters and preview-friendly.",
		"Prompt version: " + digestPromptVersion,
		"Digest date: " + base.DigestDate,
	}
}

func digestPromptInput(summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection) string {
	payload := map[string]any{
		"summaries":        summaries[:min(len(summaries), digestMaxSourceNoteCount)],
		"insights":         insights[:min(len(insights), digestMaxInsightCount)],
		"themes":           themes[:min(len(themes), 6)],
		"insight_clusters": insightClusters[:min(len(insightClusters), digestMaxInsightClusterCount)],
		"connections":      connections[:min(len(connections), digestMaxConnectionCount)],
		"newsletter_style_notes": []string{
			"Stratechery-style mechanic to apply: hook, context, thesis, evidence, synthesis, implication.",
			"Make the story coherent before making it comprehensive.",
			"Create context before details: name the problem, why it matters now, and what changed.",
			"Weave source links into prose at the point of use.",
			"Use paragraph flow, not labeled sections, bullets, source cards, or agenda cards.",
			"Close by resolving the story with one practical next move for Abhijit.",
		},
		"requirements": []string{"exactly five insights when five are provided", "coherent newsletter essay", "source grounded", "human editorial voice", "phone first", "hook then context then thesis then evidence then synthesis then implication", "no markdown headings after H1", "no The Lead section", "no The Rest Of The Brief section", "no One Thing To Do Next section", "no In This Issue section", "no Conclusion section", "no Sources section", "no bullets", "no repeated Source-only links", "every insight has markdown source link", "keep links intact", "no unsupported facts"},
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
