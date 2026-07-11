package knowledge

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var sentenceBoundary = regexp.MustCompile(`([.!?])\s+`)

var strongSignals = []string{"research", "agent", "workflow", "build", "evidence", "benchmark", "model", "product"}
var weakSignals = []string{"giveaway", "discount", "sale", "promo"}

type rawSummaryInput struct {
	ID        string
	Source    string
	Title     string
	SourceURL string
	Body      string
	Quote     string
	Notes     []string
}

func extractiveSummary(input rawSummaryInput) Summary {
	sentences := sentenceSplit(input.Body)
	first := truncate(input.Body, 220)
	if len(sentences) > 0 {
		first = sentences[0]
	}
	second := ""
	for _, sentence := range sentences {
		if sentence != first && utf8.RuneCountInString(sentence) > 50 {
			second = sentence
			break
		}
	}
	summaryText := strings.TrimSpace(strings.Join(compact([]string{first, second}), " "))
	if summaryText == "" {
		summaryText = "No summary could be produced because the source text was empty."
	}

	notes := []string{"Extractive fallback: no unsupported claims added beyond available source text."}
	notes = append(notes, input.Notes...)
	confidence := "low"
	if utf8.RuneCountInString(input.Body) > 120 {
		confidence = "medium"
	}

	return Summary{
		ID:         input.ID,
		Source:     input.Source,
		Title:      input.Title,
		SourceURL:  input.SourceURL,
		Decision:   decisionFor(input.Body),
		Summary:    summaryText,
		Quote:      input.Quote,
		Confidence: confidence,
		Notes:      notes,
	}
}

func sentenceSplit(text string) []string {
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" {
		return nil
	}
	withBreaks := sentenceBoundary.ReplaceAllString(normalized, "$1\n")
	return compact(strings.Split(withBreaks, "\n"))
}

func decisionFor(text string) Decision {
	lower := strings.ToLower(text)
	for _, word := range weakSignals {
		if strings.Contains(lower, word) {
			return DecisionSkip
		}
	}
	for _, word := range strongSignals {
		if strings.Contains(lower, word) {
			return DecisionReadNow
		}
	}
	return DecisionLater
}

func compact(values []string) []string {
	compacted := values[:0]
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			compacted = append(compacted, trimmed)
		}
	}
	return compacted
}

func fallback(value string, fallbackValue string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallbackValue
}

func truncate(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
