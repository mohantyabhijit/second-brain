package main

import (
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestDigestCronSpecUsesConfiguredSingaporeTime(t *testing.T) {
	spec := digestCronSpec(config.Config{
		DigestTimezone: "Asia/Singapore",
		DigestTime:     "18:00",
	})

	if spec != "CRON_TZ=Asia/Singapore 0 18 * * *" {
		t.Fatalf("expected 6 PM SGT cron spec, got %q", spec)
	}
}

func TestDigestCronSpecFallsBackToSingaporeForInvalidTimezone(t *testing.T) {
	spec := digestCronSpec(config.Config{
		DigestTimezone: "bad-zone",
		DigestTime:     "07:30",
	})

	if spec != "CRON_TZ=Asia/Singapore 30 7 * * *" {
		t.Fatalf("expected fallback timezone cron spec, got %q", spec)
	}
}

func TestParseClockFallsBackForInvalidInput(t *testing.T) {
	hour, minute := parseClock("nope", 18, 0)
	if hour != 18 || minute != 0 {
		t.Fatalf("expected fallback clock, got %02d:%02d", hour, minute)
	}
}

func TestShouldPublishAfterRefreshRequiresSuccessfulRefresh(t *testing.T) {
	if shouldPublishAfterRefresh(refreshOutcome{ok: false, skippedReason: "refresh_failed"}) {
		t.Fatal("expected failed refresh to skip read model publish")
	}
	if !shouldPublishAfterRefresh(refreshOutcome{ok: true, skippedReason: "no_new_source_materials"}) {
		t.Fatal("expected successful refresh to publish read models")
	}
}
