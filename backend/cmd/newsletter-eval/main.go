package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	iterations := flag.Int("iterations", intEnv("NEWSLETTER_EVAL_ITERATIONS", 5), "prompt-improvement iterations after baseline")
	outputDir := flag.String("out", valueEnv("NEWSLETTER_EVAL_OUTPUT_DIR", "../data/runtime/newsletter-experiments"), "directory for JSON and markdown reports")
	generatorModel := flag.String("generator-model", valueEnv("NEWSLETTER_EVAL_GENERATOR_MODEL", cfg.OpenAISynthesisModel), "newsletter generator model")
	judgeModel := flag.String("judge-model", valueEnv("NEWSLETTER_EVAL_JUDGE_MODEL", "gpt-4o-mini"), "smaller judge model")
	improverModel := flag.String("improver-model", valueEnv("NEWSLETTER_EVAL_IMPROVER_MODEL", *generatorModel), "prompt improver model")
	timeout := flag.Duration("timeout", durationEnv("NEWSLETTER_EVAL_TIMEOUT", 20*time.Minute), "experiment timeout")
	inspectInputs := flag.Bool("inspect-inputs", false, "print latest digest input counts without model calls")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var store knowledge.Store
	var closeStore func()
	if cfg.DatabaseURL == "" {
		logger.Info("using local knowledge run file", "path", cfg.KnowledgeRunPath)
		store = localfile.New(cfg.KnowledgeRunPath)
		closeStore = func() {}
	} else {
		postgresStore, err := postgres.New(ctx, cfg.DatabaseURL)
		if err != nil {
			logger.Error("connect postgres", "error", err)
			os.Exit(1)
		}
		store = postgresStore
		closeStore = postgresStore.Close
	}
	defer closeStore()

	service := knowledge.NewService(cfg, store, httpclient.New())
	service.SetLogger(logger)
	if *inspectInputs {
		if err := printLatestInputCounts(ctx, service); err != nil {
			logger.Error("inspect newsletter inputs", "error", err)
			os.Exit(1)
		}
		return
	}

	report, err := service.RunNewsletterPromptExperiment(ctx, knowledge.NewsletterExperimentOptions{
		Iterations:     *iterations,
		GeneratorModel: *generatorModel,
		JudgeModel:     *judgeModel,
		ImproverModel:  *improverModel,
	})

	var jsonPath, mdPath string
	if report != nil {
		var writeErr error
		jsonPath, mdPath, writeErr = writeExperimentReport(*outputDir, report)
		if writeErr != nil {
			logger.Error("write experiment report", "error", writeErr)
			os.Exit(1)
		}
		logger.Info("newsletter experiment report written", "json", jsonPath, "markdown", mdPath)
		printScoreTable(report)
	}
	if err != nil {
		logger.Error("newsletter experiment failed", "error", err)
		os.Exit(1)
	}
}

func printLatestInputCounts(ctx context.Context, service *knowledge.Service) error {
	latest, err := service.ReadLatest(ctx)
	if err != nil {
		return err
	}
	if latest == nil {
		return fmt.Errorf("no knowledge run is available")
	}
	fmt.Printf("generated_at=%s\n", latest.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("x_bookmarks=%d\n", len(latest.XBookmarks))
	fmt.Printf("youtube_items=%d\n", len(latest.YouTubeItems))
	fmt.Printf("summaries=%d\n", len(latest.Summaries))
	fmt.Printf("insights=%d\n", len(latest.Insights))
	fmt.Printf("themes=%d\n", len(latest.Themes))
	fmt.Printf("insight_clusters=%d\n", len(latest.InsightClusters))
	fmt.Printf("connections=%d\n", len(latest.Connections))
	fmt.Printf("blockers=%d\n", len(latest.Blockers))
	return nil
}

func writeExperimentReport(outputDir string, report *knowledge.NewsletterExperimentReport) (string, string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", err
	}
	base := report.ID
	jsonPath := filepath.Join(outputDir, base+".json")
	mdPath := filepath.Join(outputDir, base+".md")

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(jsonPath, append(raw, '\n'), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(mdPath, []byte(renderExperimentMarkdown(report)), 0o644); err != nil {
		return "", "", err
	}
	return jsonPath, mdPath, nil
}

func renderExperimentMarkdown(report *knowledge.NewsletterExperimentReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Newsletter prompt experiment: %s\n\n", report.ID)
	fmt.Fprintf(&builder, "- Generated at: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&builder, "- Digest date: %s\n", report.DigestDate)
	fmt.Fprintf(&builder, "- Prompt version: `%s`\n", report.PromptVersion)
	fmt.Fprintf(&builder, "- Generator model: `%s`\n", report.GeneratorModel)
	fmt.Fprintf(&builder, "- Judge model: `%s`\n", report.JudgeModel)
	fmt.Fprintf(&builder, "- Improver model: `%s`\n", report.ImproverModel)
	fmt.Fprintf(&builder, "- Inputs: %d summaries, %d total insights, %d selected insights, %d themes, %d insight clusters, %d connections\n\n",
		report.Input.SummaryCount,
		report.Input.InsightCount,
		len(report.Input.SelectedInsightIDs),
		report.Input.ThemeCount,
		report.Input.InsightClusterCount,
		report.Input.ConnectionCount,
	)
	fmt.Fprintf(&builder, "## Score trajectory\n\n")
	fmt.Fprintf(&builder, "| Iteration | Overall | Grounding | Synthesis | Voice | Usefulness | Structure | Links | Subject |\n")
	fmt.Fprintf(&builder, "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, run := range report.Runs {
		fmt.Fprintf(&builder, "| %d | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %.1f | %s |\n",
			run.Iteration,
			run.Score,
			run.Judge.Grounding,
			run.Judge.Synthesis,
			run.Judge.EditorialVoice,
			run.Judge.Usefulness,
			run.Judge.Structure,
			run.Judge.SourceLinking,
			escapeMarkdownTable(run.Subject),
		)
	}
	fmt.Fprintf(&builder, "\nBaseline: %.1f\n\nFinal: %.1f\n\nImprovement: %.1f\n\n", report.BaselineScore, report.FinalScore, report.Improvement)

	fmt.Fprintf(&builder, "## Iteration notes\n\n")
	for _, run := range report.Runs {
		fmt.Fprintf(&builder, "### Iteration %d\n\n", run.Iteration)
		fmt.Fprintf(&builder, "- Score: %.1f\n", run.Score)
		if strings.TrimSpace(run.Judge.Rationale) != "" {
			fmt.Fprintf(&builder, "- Judge rationale: %s\n", run.Judge.Rationale)
		}
		if len(run.Judge.Improvements) > 0 {
			fmt.Fprintf(&builder, "- Improvements requested: %s\n", strings.Join(run.Judge.Improvements, "; "))
		}
		if len(run.RevisionForNext.AddendumLines) > 0 {
			fmt.Fprintf(&builder, "- Next prompt revision: %s\n", run.RevisionForNext.Summary)
			for _, line := range run.RevisionForNext.AddendumLines {
				fmt.Fprintf(&builder, "  - %s\n", line)
			}
		}
		fmt.Fprintf(&builder, "\n")
	}

	if len(report.Runs) > 0 {
		final := report.Runs[len(report.Runs)-1]
		fmt.Fprintf(&builder, "## Final experimental addendum\n\n")
		if len(final.PromptAddendum) == 0 {
			fmt.Fprintf(&builder, "No addendum was applied to the final run.\n")
		} else {
			for _, line := range final.PromptAddendum {
				fmt.Fprintf(&builder, "- %s\n", line)
			}
		}
		fmt.Fprintf(&builder, "\nFull generated bodies are stored in the JSON report.\n")
	}
	return builder.String()
}

func printScoreTable(report *knowledge.NewsletterExperimentReport) {
	fmt.Println("iteration overall grounding synthesis voice usefulness structure links subject")
	for _, run := range report.Runs {
		fmt.Printf("%d %.1f %.1f %.1f %.1f %.1f %.1f %.1f %q\n",
			run.Iteration,
			run.Score,
			run.Judge.Grounding,
			run.Judge.Synthesis,
			run.Judge.EditorialVoice,
			run.Judge.Usefulness,
			run.Judge.Structure,
			run.Judge.SourceLinking,
			run.Subject,
		)
	}
	fmt.Printf("baseline=%.1f final=%.1f improvement=%.1f\n", report.BaselineScore, report.FinalScore, report.Improvement)
}

func escapeMarkdownTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

func valueEnv(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
