package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
)

func TestWriteExperimentReportCreatesPrivateArtifacts(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "reports")
	jsonPath, markdownPath, err := writeExperimentReport(outputDir, &knowledge.NewsletterExperimentReport{ID: "experiment-1"})
	if err != nil {
		t.Fatalf("write experiment report: %v", err)
	}
	for _, path := range []string{jsonPath, markdownPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", path, got)
		}
	}
	info, err := os.Stat(outputDir)
	if err != nil {
		t.Fatalf("stat output directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("output directory mode = %o, want 700", got)
	}
}

func TestExperimentConfigurationParsersFailSafely(t *testing.T) {
	t.Run("markdown table escaping", func(t *testing.T) {
		if got := escapeMarkdownTable(" first | second\nthird "); got != `first \| second third` {
			t.Fatalf("escaped table value = %q", got)
		}
	})
	t.Run("value uses configured value", func(t *testing.T) {
		t.Setenv("TEST_VALUE", " configured ")
		if got := valueEnv("TEST_VALUE", "fallback"); got != "configured" {
			t.Fatalf("valueEnv = %q", got)
		}
	})
	t.Run("value falls back when blank", func(t *testing.T) {
		t.Setenv("TEST_VALUE", "  ")
		if got := valueEnv("TEST_VALUE", "fallback"); got != "fallback" {
			t.Fatalf("valueEnv = %q", got)
		}
	})
	for _, test := range []struct {
		name string
		raw  string
		want int
	}{
		{"integer", "12", 12},
		{"negative integer", "-2", -2},
		{"blank integer", "", 7},
		{"invalid integer", "many", 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TEST_INT", test.raw)
			if got := intEnv("TEST_INT", 7); got != test.want {
				t.Fatalf("intEnv(%q) = %d, want %d", test.raw, got, test.want)
			}
		})
	}
	for _, test := range []struct {
		name string
		raw  string
		want time.Duration
	}{
		{"duration", "45s", 45 * time.Second},
		{"zero duration is explicit", "0s", 0},
		{"blank duration", "", time.Minute},
		{"invalid duration", "later", time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("TEST_DURATION", test.raw)
			if got := durationEnv("TEST_DURATION", time.Minute); got != test.want {
				t.Fatalf("durationEnv(%q) = %s, want %s", test.raw, got, test.want)
			}
		})
	}
}
