package prompts

import (
	"strings"
	"testing"
)

func TestDigestNewsletterPromptIncludesPromotedExperimentLearning(t *testing.T) {
	prompt := strings.Join(DigestNewsletterLines("2026-05-24"), "\n")
	for _, expected := range []string{
		DigestPromptVersion,
		"PROMOTED LEARNING FROM NEWSLETTER EVAL",
		"Keep numerical claims precise",
		"Keep the central thesis visible",
		"Treat themes, clusters, and connections as connective tissue",
		"source links close to the exact claim",
		"one practical Abhijit-specific next move",
		"paragraph-local reasoning",
		"fuse them into one synthesized paragraph",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected digest prompt to include %q, got %s", expected, prompt)
		}
	}
}

func TestNewsletterExperimentJudgeRequiresConsistentHundredPointScores(t *testing.T) {
	prompt := NewsletterExperimentJudge("{}", "{}")
	for _, expected := range []string{
		"Never use a 0-10 scale",
		"arithmetic mean",
		"Scores, rationale, strengths, and improvements must agree",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("expected judge prompt to include %q, got %s", expected, prompt)
		}
	}
}
