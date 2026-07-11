package main

import (
	"os"
	"path/filepath"
	"testing"

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
