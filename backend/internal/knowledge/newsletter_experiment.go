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

const defaultNewsletterJudgeModel = "gpt-4o-mini"

type NewsletterExperimentOptions struct {
	Iterations     int
	GeneratedAt    time.Time
	GeneratorModel string
	JudgeModel     string
	ImproverModel  string
}

type NewsletterExperimentReport struct {
	ID                  string                           `json:"id"`
	GeneratedAt         time.Time                        `json:"generatedAt"`
	DigestDate          string                           `json:"digestDate"`
	PromptVersion       string                           `json:"promptVersion"`
	IterationsRequested int                              `json:"iterationsRequested"`
	GeneratorModel      string                           `json:"generatorModel"`
	JudgeModel          string                           `json:"judgeModel"`
	ImproverModel       string                           `json:"improverModel"`
	Input               NewsletterExperimentInputSummary `json:"input"`
	Runs                []NewsletterExperimentRun        `json:"runs"`
	BaselineScore       float64                          `json:"baselineScore"`
	FinalScore          float64                          `json:"finalScore"`
	Improvement         float64                          `json:"improvement"`
}

type NewsletterExperimentInputSummary struct {
	SummaryCount        int      `json:"summaryCount"`
	InsightCount        int      `json:"insightCount"`
	SelectedInsightIDs  []string `json:"selectedInsightIds"`
	ThemeCount          int      `json:"themeCount"`
	InsightClusterCount int      `json:"insightClusterCount"`
	ConnectionCount     int      `json:"connectionCount"`
}

type NewsletterExperimentRun struct {
	Iteration       int                      `json:"iteration"`
	PromptAddendum  []string                 `json:"promptAddendum,omitempty"`
	Subject         string                   `json:"subject"`
	BodyMarkdown    string                   `json:"bodyMarkdown"`
	Score           float64                  `json:"score"`
	Judge           NewsletterJudgeScores    `json:"judge"`
	RevisionForNext NewsletterPromptRevision `json:"revisionForNext,omitempty"`
}

type NewsletterJudgeScores struct {
	Overall        float64  `json:"overall"`
	Grounding      float64  `json:"grounding"`
	Synthesis      float64  `json:"synthesis"`
	EditorialVoice float64  `json:"editorialVoice"`
	Usefulness     float64  `json:"usefulness"`
	Structure      float64  `json:"structure"`
	SourceLinking  float64  `json:"sourceLinking"`
	Rationale      string   `json:"rationale"`
	Strengths      []string `json:"strengths"`
	Improvements   []string `json:"improvements"`
}

type NewsletterPromptRevision struct {
	Summary       string   `json:"summary,omitempty"`
	AddendumLines []string `json:"addendumLines,omitempty"`
}

func (s *Service) RunNewsletterPromptExperiment(ctx context.Context, opts NewsletterExperimentOptions) (*NewsletterExperimentReport, error) {
	ctx, span := s.startOperationSpan(ctx, "newsletter-prompt-experiment", "newsletter-eval", "experiment")
	defer span.End()
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		err := fmt.Errorf("OPENAI_API_KEY is required for newsletter prompt experiments")
		setSpanError(span, err)
		return nil, err
	}
	if opts.Iterations < 0 {
		opts.Iterations = 5
	}
	if opts.Iterations > 10 {
		opts.Iterations = 10
	}
	if opts.GeneratedAt.IsZero() {
		opts.GeneratedAt = time.Now().UTC()
	}
	if strings.TrimSpace(opts.GeneratorModel) == "" {
		opts.GeneratorModel = s.cfg.OpenAISynthesisModel
	}
	if strings.TrimSpace(opts.JudgeModel) == "" {
		opts.JudgeModel = defaultNewsletterJudgeModel
	}
	if strings.TrimSpace(opts.ImproverModel) == "" {
		opts.ImproverModel = opts.GeneratorModel
	}

	latest, err := s.ReadLatest(ctx)
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}
	if latest == nil {
		err := fmt.Errorf("no knowledge run is available for newsletter prompt experiment")
		setSpanError(span, err)
		return nil, err
	}
	if !hasDigestInputs(latest.Summaries, latest.Insights) {
		err := fmt.Errorf("no source-grounded digest inputs are available")
		setSpanError(span, err)
		return nil, err
	}

	selectedInsights := selectDigestInsights(opts.GeneratedAt, latest.Insights, digestMaxInsightCount)
	base := buildDigestIssue(s.cfg.DigestTimezone, s.cfg.DigestTime, opts.GeneratedAt, latest.Summaries, selectedInsights, latest.Themes, latest.InsightClusters, latest.Connections)
	inputJSON := digestPromptInput(latest.Summaries, selectedInsights, latest.Themes, latest.InsightClusters, latest.Connections)
	report := &NewsletterExperimentReport{
		ID:                  "newsletter-eval-" + opts.GeneratedAt.UTC().Format("20060102T150405Z"),
		GeneratedAt:         opts.GeneratedAt.UTC(),
		DigestDate:          base.DigestDate,
		PromptVersion:       digestPromptVersion,
		IterationsRequested: opts.Iterations,
		GeneratorModel:      opts.GeneratorModel,
		JudgeModel:          opts.JudgeModel,
		ImproverModel:       opts.ImproverModel,
		Input: NewsletterExperimentInputSummary{
			SummaryCount:        len(latest.Summaries),
			InsightCount:        len(latest.Insights),
			SelectedInsightIDs:  insightIDs(selectedInsights),
			ThemeCount:          len(latest.Themes),
			InsightClusterCount: len(latest.InsightClusters),
			ConnectionCount:     len(latest.Connections),
		},
		Runs: []NewsletterExperimentRun{},
	}

	addendum := []string{}
	for iteration := 0; iteration <= opts.Iterations; iteration++ {
		digest, err := s.generateExperimentNewsletter(ctx, opts.GeneratorModel, base, latest.Summaries, selectedInsights, latest.Themes, latest.InsightClusters, latest.Connections, addendum)
		if err != nil {
			err := fmt.Errorf("generate experiment newsletter iteration %d: %w", iteration, err)
			setSpanError(span, err)
			setSpanOutputSummary(span, map[string]any{"runs": len(report.Runs), "digest_date": report.DigestDate})
			return report, err
		}
		judge, err := s.judgeExperimentNewsletter(ctx, opts.JudgeModel, inputJSON, digest)
		if err != nil {
			err := fmt.Errorf("judge experiment newsletter iteration %d: %w", iteration, err)
			setSpanError(span, err)
			setSpanOutputSummary(span, map[string]any{"runs": len(report.Runs), "digest_date": report.DigestDate})
			return report, err
		}
		run := NewsletterExperimentRun{
			Iteration:      iteration,
			PromptAddendum: append([]string(nil), addendum...),
			Subject:        digest.Subject,
			BodyMarkdown:   digest.BodyMarkdown,
			Score:          judge.Overall,
			Judge:          judge,
		}
		report.Runs = append(report.Runs, run)

		if iteration == opts.Iterations {
			break
		}
		revision, err := s.reviseExperimentPrompt(ctx, opts.ImproverModel, inputJSON, addendum, digest, judge)
		if err != nil {
			err := fmt.Errorf("revise experiment prompt after iteration %d: %w", iteration, err)
			setSpanError(span, err)
			setSpanOutputSummary(span, map[string]any{"runs": len(report.Runs), "digest_date": report.DigestDate})
			return report, err
		}
		revision.AddendumLines = normalizeExperimentAddendum(revision.AddendumLines)
		if len(revision.AddendumLines) == 0 {
			revision.AddendumLines = fallbackExperimentAddendum(judge)
		}
		report.Runs[len(report.Runs)-1].RevisionForNext = revision
		addendum = revision.AddendumLines
	}

	if len(report.Runs) > 0 {
		report.BaselineScore = report.Runs[0].Score
		report.FinalScore = report.Runs[len(report.Runs)-1].Score
		report.Improvement = report.FinalScore - report.BaselineScore
	}
	setSpanOutputSummary(span, map[string]any{"runs": len(report.Runs), "baseline_score": report.BaselineScore, "final_score": report.FinalScore, "improvement": report.Improvement, "digest_date": report.DigestDate})
	return report, nil
}

func (s *Service) generateExperimentNewsletter(ctx context.Context, model string, base DigestIssue, summaries []Summary, insights []Insight, themes []ThemeCluster, insightClusters []InsightCluster, connections []SourceConnection, addendum []string) (DigestIssue, error) {
	inputJSON := truncate(digestPromptInput(summaries, insights, themes, insightClusters, connections), 16000)
	var payload promptDigestResponse
	var err error
	if len(addendum) == 0 {
		prompt := s.digestNewsletterPrompt(ctx, base, inputJSON)
		payload, err = s.promptDigestWithPrompt(ctx, model, prompt, 3000)
	} else {
		promptLines := digestNewsletterPromptLines(base)
		promptLines = prompts.AppendExperimentAddendum(promptLines, addendum)
		promptLines = prompts.AppendInputJSON(promptLines, inputJSON)
		payload, err = s.promptDigestWithLines(ctx, model, promptLines, 3000)
	}
	if err != nil {
		return DigestIssue{}, err
	}
	if strings.TrimSpace(payload.Subject) == "" {
		return DigestIssue{}, fmt.Errorf("newsletter synthesis returned an empty subject")
	}
	bodyMarkdown := strings.TrimSpace(payload.BodyMarkdown)
	if bodyMarkdown == "" && len(payload.BodyLines) > 0 {
		bodyMarkdown = strings.TrimSpace(strings.Join(payload.BodyLines, "\n"))
	}
	bodyMarkdown = ensureDigestSourceLinks(bodyMarkdown, insights)
	if strings.TrimSpace(bodyMarkdown) == "" {
		return DigestIssue{}, fmt.Errorf("newsletter synthesis returned an empty body")
	}

	digest := base
	digest.Subject = strings.TrimSpace(payload.Subject)
	digest.BodyMarkdown = bodyMarkdown
	digest.IdempotencyKey = "experiment:" + base.DigestDate + ":" + digestBodyFingerprint(bodyMarkdown)
	digest.Status = "evaluated"
	return digest, nil
}

func (s *Service) judgeExperimentNewsletter(ctx context.Context, model string, inputJSON string, digest DigestIssue) (NewsletterJudgeScores, error) {
	newsletterJSON := mustCompactJSON(map[string]string{
		"subject":       digest.Subject,
		"body_markdown": digest.BodyMarkdown,
	})
	prompt := prompts.NewsletterExperimentJudge(truncate(inputJSON, 12000), newsletterJSON)
	text, err := s.promptExperimentText(ctx, model, prompt, 1400)
	if err != nil {
		return NewsletterJudgeScores{}, err
	}
	var judge NewsletterJudgeScores
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &judge); err != nil {
		return NewsletterJudgeScores{}, err
	}
	judge = normalizeJudgeScores(judge)
	return judge, nil
}

func (s *Service) reviseExperimentPrompt(ctx context.Context, model string, inputJSON string, currentAddendum []string, digest DigestIssue, judge NewsletterJudgeScores) (NewsletterPromptRevision, error) {
	feedbackJSON := mustCompactJSON(judge)
	currentPromptJSON := mustCompactJSON(currentAddendum)
	newsletterJSON := mustCompactJSON(map[string]string{
		"subject":       digest.Subject,
		"body_markdown": digest.BodyMarkdown,
	})
	prompt := prompts.NewsletterExperimentImprover(currentPromptJSON, feedbackJSON, newsletterJSON, truncate(inputJSON, 10000))
	text, err := s.promptExperimentText(ctx, model, prompt, 1200)
	if err != nil {
		return NewsletterPromptRevision{}, err
	}
	var revision NewsletterPromptRevision
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &revision); err != nil {
		return NewsletterPromptRevision{}, err
	}
	return revision, nil
}

func (s *Service) promptExperimentText(ctx context.Context, model string, input string, maxOutputTokens int) (string, error) {
	if strings.TrimSpace(model) == "" {
		model = s.cfg.OpenAISynthesisModel
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 1000
	}
	ctx, span := s.startObservationSpan(ctx, observationOptions{
		Name:          "newsletter-experiment-model-call",
		Type:          "generation",
		Model:         model,
		PromptVersion: digestPromptVersion,
		Tags:          []string{"newsletter-eval", "experiment", "generation"},
		InputSummary: map[string]any{
			"input_chars": len(input),
		},
		ModelParams: map[string]any{
			"max_output_tokens": maxOutputTokens,
		},
	})
	defer span.End()
	requestBody := map[string]any{
		"model":             model,
		"input":             input,
		"max_output_tokens": maxOutputTokens,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		setSpanError(span, err)
		return "", err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &response); err != nil {
		setSpanError(span, err)
		return "", err
	}
	if response.Error != nil && response.Error.Message != "" {
		err := fmt.Errorf("%s", response.Error.Message)
		setSpanError(span, err)
		return "", err
	}
	setOpenAIUsage(span, response.Usage)
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
		err := fmt.Errorf("model returned empty output")
		setSpanError(span, err)
		return "", err
	}
	text = strings.TrimSpace(text)
	setSpanOutputSummary(span, map[string]any{"output_chars": len(text)})
	return text, nil
}

func insightIDs(insights []Insight) []string {
	ids := make([]string, 0, len(insights))
	for _, insight := range insights {
		ids = append(ids, insight.ID)
	}
	return ids
}

func normalizeJudgeScores(judge NewsletterJudgeScores) NewsletterJudgeScores {
	judge.Grounding = normalizeJudgeScore(judge.Grounding)
	judge.Synthesis = normalizeJudgeScore(judge.Synthesis)
	judge.EditorialVoice = normalizeJudgeScore(judge.EditorialVoice)
	judge.Usefulness = normalizeJudgeScore(judge.Usefulness)
	judge.Structure = normalizeJudgeScore(judge.Structure)
	judge.SourceLinking = normalizeJudgeScore(judge.SourceLinking)
	judge.Overall = normalizeJudgeScore(judge.Overall)
	if judge.Overall == 0 {
		total := 0.0
		count := 0.0
		for _, score := range []float64{judge.Grounding, judge.Synthesis, judge.EditorialVoice, judge.Usefulness, judge.Structure, judge.SourceLinking} {
			if score > 0 {
				total += score
				count++
			}
		}
		if count > 0 {
			judge.Overall = total / count
		}
	}
	return judge
}

func normalizeJudgeScore(score float64) float64 {
	if score <= 0 {
		return 0
	}
	if score <= 10 {
		score *= 10
	}
	if score > 100 {
		return 100
	}
	return score
}

func normalizeExperimentAddendum(lines []string) []string {
	normalized := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		normalized = append(normalized, line)
		if len(normalized) >= 7 {
			break
		}
	}
	return normalized
}

func fallbackExperimentAddendum(judge NewsletterJudgeScores) []string {
	feedback := strings.Join(judge.Improvements, "; ")
	if strings.TrimSpace(feedback) == "" {
		feedback = judge.Rationale
	}
	if strings.TrimSpace(feedback) == "" {
		feedback = "make the next issue more grounded, more synthetic, and more useful"
	}
	return prompts.FallbackExperimentAddendum(truncateDigestText(feedback, 360))
}

func mustCompactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
