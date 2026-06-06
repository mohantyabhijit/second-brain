package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"go.opentelemetry.io/otel/trace"
)

func TestFinalizeNewsletterExperimentKeepsPartialScores(t *testing.T) {
	service := NewService(config.Config{}, cacheStore{}, nil)
	report := &NewsletterExperimentReport{
		Runs: []NewsletterExperimentRun{
			{Score: 82, PromptAddendum: []string{"baseline"}},
			{Score: 88, PromptAddendum: []string{"champion"}},
			{Score: 84, PromptAddendum: []string{"regression"}},
		},
	}

	service.finalizeNewsletterExperiment(context.Background(), trace.SpanFromContext(context.Background()), report)

	if report.BaselineScore != 82 || report.FinalScore != 84 || report.Improvement != 2 {
		t.Fatalf("unexpected partial experiment summary: %#v", report)
	}
	if report.BestScore != 88 || report.BestIteration != 1 || report.ChampionImprovement != 6 || len(report.ChampionAddendum) != 1 || report.ChampionAddendum[0] != "champion" {
		t.Fatalf("unexpected champion summary: %#v", report)
	}
}

func TestValidateJudgeScoresRejectsMixedScales(t *testing.T) {
	judge := NewsletterJudgeScores{
		Overall:        90,
		Grounding:      20,
		Synthesis:      20,
		EditorialVoice: 15,
		Usefulness:     15,
		Structure:      10,
		SourceLinking:  10,
	}
	if validateJudgeScores(judge) == nil {
		t.Fatal("expected inconsistent judge scores to be rejected")
	}
}

func TestIsTransientProviderError(t *testing.T) {
	if !isTransientProviderError(errors.New("api.openai.com 503 Service Unavailable: connection timeout")) {
		t.Fatal("expected 503 timeout to be retryable")
	}
	if isTransientProviderError(errors.New("api.openai.com 400 Bad Request")) {
		t.Fatal("expected 400 response not to be retryable")
	}
}
