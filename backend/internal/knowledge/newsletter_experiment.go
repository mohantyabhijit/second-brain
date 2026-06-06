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
	"go.opentelemetry.io/otel/trace"
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
	BestScore           float64                          `json:"bestScore"`
	BestIteration       int                              `json:"bestIteration"`
	ChampionImprovement float64                          `json:"championImprovement"`
	ChampionAddendum    []string                         `json:"championAddendum,omitempty"`
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
	reportID := "newsletter-eval-" + opts.GeneratedAt.UTC().Format("20060102T150405Z")
	ctx, span := s.startOperationSpanWithSession(ctx, "newsletter-prompt-experiment", reportID, "newsletter-eval", "experiment")
	defer span.End()
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		err := fmt.Errorf("OPENAI_API_KEY is required for newsletter prompt experiments")
		setSpanError(span, err)
		return nil, err
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
		ID:                  reportID,
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
			s.finalizeNewsletterExperiment(ctx, span, report)
			setSpanOutputSummary(span, map[string]any{"runs": len(report.Runs), "digest_date": report.DigestDate})
			return report, err
		}
		judge, err := s.judgeExperimentNewsletter(ctx, opts.JudgeModel, inputJSON, digest)
		if err != nil {
			err := fmt.Errorf("judge experiment newsletter iteration %d: %w", iteration, err)
			setSpanError(span, err)
			s.finalizeNewsletterExperiment(ctx, span, report)
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
		champion := report.Runs[bestNewsletterRunIndex(report.Runs)]
		championDigest := base
		championDigest.Subject = champion.Subject
		championDigest.BodyMarkdown = champion.BodyMarkdown
		revision, err := s.reviseExperimentPrompt(ctx, opts.ImproverModel, inputJSON, champion.PromptAddendum, championDigest, champion.Judge)
		if err != nil {
			err := fmt.Errorf("revise experiment prompt after iteration %d: %w", iteration, err)
			setSpanError(span, err)
			s.finalizeNewsletterExperiment(ctx, span, report)
			setSpanOutputSummary(span, map[string]any{"runs": len(report.Runs), "digest_date": report.DigestDate})
			return report, err
		}
		revision.AddendumLines = normalizeExperimentAddendum(revision.AddendumLines)
		if len(revision.AddendumLines) == 0 {
			revision.AddendumLines = fallbackExperimentAddendum(champion.Judge)
		}
		report.Runs[len(report.Runs)-1].RevisionForNext = revision
		addendum = revision.AddendumLines
	}

	s.finalizeNewsletterExperiment(ctx, span, report)
	setSpanOutputSummary(span, map[string]any{
		"runs":                 len(report.Runs),
		"baseline_score":       report.BaselineScore,
		"final_score":          report.FinalScore,
		"improvement":          report.Improvement,
		"best_score":           report.BestScore,
		"best_iteration":       report.BestIteration,
		"champion_improvement": report.ChampionImprovement,
		"digest_date":          report.DigestDate,
	})
	return report, nil
}

func (s *Service) finalizeNewsletterExperiment(ctx context.Context, span trace.Span, report *NewsletterExperimentReport) {
	if report == nil || len(report.Runs) == 0 {
		return
	}
	report.BaselineScore = report.Runs[0].Score
	report.FinalScore = report.Runs[len(report.Runs)-1].Score
	report.Improvement = report.FinalScore - report.BaselineScore
	report.BestIteration = bestNewsletterRunIndex(report.Runs)
	champion := report.Runs[report.BestIteration]
	report.BestScore = champion.Score
	report.ChampionImprovement = report.BestScore - report.BaselineScore
	report.ChampionAddendum = append([]string(nil), champion.PromptAddendum...)
	final := report.Runs[len(report.Runs)-1]
	s.scoreTrace(ctx, span, "newsletter_baseline_quality", normalizeLangfuseScore(report.BaselineScore), "NUMERIC", report.Runs[0].Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_final_quality", normalizeLangfuseScore(report.FinalScore), "NUMERIC", final.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_improvement", report.Improvement/100, "NUMERIC", "final quality minus baseline quality")
	s.scoreTrace(ctx, span, "newsletter_champion_quality", normalizeLangfuseScore(report.BestScore), "NUMERIC", champion.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_champion_improvement", report.ChampionImprovement/100, "NUMERIC", "best quality minus baseline quality")
	s.scoreTrace(ctx, span, "newsletter_grounding", normalizeLangfuseScore(champion.Judge.Grounding), "NUMERIC", champion.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_synthesis", normalizeLangfuseScore(champion.Judge.Synthesis), "NUMERIC", champion.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_editorial_voice", normalizeLangfuseScore(champion.Judge.EditorialVoice), "NUMERIC", champion.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_usefulness", normalizeLangfuseScore(champion.Judge.Usefulness), "NUMERIC", champion.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_structure", normalizeLangfuseScore(champion.Judge.Structure), "NUMERIC", champion.Judge.Rationale)
	s.scoreTrace(ctx, span, "newsletter_source_linking", normalizeLangfuseScore(champion.Judge.SourceLinking), "NUMERIC", champion.Judge.Rationale)
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
	text, err := s.promptExperimentText(ctx, "newsletter-experiment-judge", model, prompt, 1400)
	if err != nil {
		return NewsletterJudgeScores{}, err
	}
	var judge NewsletterJudgeScores
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &judge); err != nil {
		return NewsletterJudgeScores{}, err
	}
	if err := validateJudgeScores(judge); err != nil {
		repairPrompt := prompts.NewsletterExperimentJudgeRepair(prompt, extractJSONObject(text))
		repairedText, repairErr := s.promptExperimentText(ctx, "newsletter-experiment-judge-retry", model, repairPrompt, 1400)
		if repairErr != nil {
			return NewsletterJudgeScores{}, fmt.Errorf("repair inconsistent judge scores after %v: %w", err, repairErr)
		}
		if repairErr := json.Unmarshal([]byte(extractJSONObject(repairedText)), &judge); repairErr != nil {
			return NewsletterJudgeScores{}, repairErr
		}
		if repairErr := validateJudgeScores(judge); repairErr != nil {
			return NewsletterJudgeScores{}, repairErr
		}
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
	text, err := s.promptExperimentText(ctx, "newsletter-experiment-improver", model, prompt, 1200)
	if err != nil {
		return NewsletterPromptRevision{}, err
	}
	var revision NewsletterPromptRevision
	if err := json.Unmarshal([]byte(extractJSONObject(text)), &revision); err != nil {
		return NewsletterPromptRevision{}, err
	}
	return revision, nil
}

func (s *Service) promptExperimentText(ctx context.Context, name string, model string, input string, maxOutputTokens int) (string, error) {
	if strings.TrimSpace(model) == "" {
		model = s.cfg.OpenAISynthesisModel
	}
	if maxOutputTokens <= 0 {
		maxOutputTokens = 1000
	}
	ctx, span := s.startObservationSpan(ctx, observationOptions{
		Name:          name,
		Type:          "generation",
		Model:         model,
		PromptVersion: digestPromptVersion,
		Tags:          []string{"newsletter-eval", "experiment", "generation"},
		InputSummary: map[string]any{
			"input_chars": len(input),
		},
		InputContent: input,
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
	if err := s.requestExperimentResponse(ctx, headers, raw, &response); err != nil {
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
	s.setSpanOutput(span, map[string]any{"output_chars": len(text)}, text)
	return text, nil
}

func (s *Service) requestExperimentResponse(ctx context.Context, headers http.Header, raw []byte, response *openAIResponse) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		*response = openAIResponse{}
		err = s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), response)
		if err == nil || !isTransientProviderError(err) || attempt == 1 {
			return err
		}
		s.log(ctx).Warn("retrying newsletter experiment model call after transient provider error", "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
	return err
}

func isTransientProviderError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{" 502 ", " 503 ", " 504 ", "connection timeout", "connection reset", "reset before headers"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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
	judge.Overall = judgeDimensionAverage(judge)
	return judge
}

func normalizeJudgeScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func validateJudgeScores(judge NewsletterJudgeScores) error {
	scores := []float64{judge.Overall, judge.Grounding, judge.Synthesis, judge.EditorialVoice, judge.Usefulness, judge.Structure, judge.SourceLinking}
	for _, score := range scores {
		if score < 0 || score > 100 {
			return fmt.Errorf("judge score %.1f is outside the 0-100 range", score)
		}
	}
	average := judgeDimensionAverage(judge)
	if difference(judge.Overall, average) > 12 {
		return fmt.Errorf("judge overall %.1f is inconsistent with dimension average %.1f", judge.Overall, average)
	}
	return nil
}

func judgeDimensionAverage(judge NewsletterJudgeScores) float64 {
	total := judge.Grounding + judge.Synthesis + judge.EditorialVoice + judge.Usefulness + judge.Structure + judge.SourceLinking
	return total / 6
}

func difference(left float64, right float64) float64 {
	if left > right {
		return left - right
	}
	return right - left
}

func bestNewsletterRunIndex(runs []NewsletterExperimentRun) int {
	best := 0
	for index := 1; index < len(runs); index++ {
		if runs[index].Score > runs[best].Score {
			best = index
		}
	}
	return best
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
