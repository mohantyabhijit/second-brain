package knowledge

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestFeedbackValidationRejectsAmbiguousOrUnboundedMetadata(t *testing.T) {
	tests := []struct {
		name  string
		event FeedbackEvent
	}{
		{"missing target type", FeedbackEvent{TargetID: "1"}},
		{"missing target id", FeedbackEvent{TargetType: "insight"}},
		{"oversized target type", FeedbackEvent{TargetType: strings.Repeat("t", 65), TargetID: "1"}},
		{"oversized target id", FeedbackEvent{TargetType: "insight", TargetID: strings.Repeat("i", 257)}},
		{"oversized note", FeedbackEvent{TargetType: "insight", TargetID: "1", Note: strings.Repeat("n", 2001)}},
		{"relative source URL", FeedbackEvent{TargetType: "insight", TargetID: "1", SourceURL: "/private"}},
		{"javascript source URL", FeedbackEvent{TargetType: "insight", TargetID: "1", SourceURL: "javascript:alert(1)"}},
		{"file source URL", FeedbackEvent{TargetType: "insight", TargetID: "1", SourceURL: "file:///etc/passwd"}},
		{"URL without host", FeedbackEvent{TargetType: "insight", TargetID: "1", SourceURL: "https:///missing"}},
		{"oversized source URL", FeedbackEvent{TargetType: "insight", TargetID: "1", SourceURL: "https://example.test/" + strings.Repeat("x", 2048)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateFeedbackEvent(test.event); err == nil {
				t.Fatalf("expected %#v to be rejected", test.event)
			}
		})
	}
}

func TestFeedbackValidationAcceptsSupportedPublicURLs(t *testing.T) {
	for _, sourceURL := range []string{"", "https://example.test/source", "http://localhost:3000/item"} {
		t.Run(sourceURL, func(t *testing.T) {
			event := FeedbackEvent{TargetType: "insight", TargetID: "1", SourceURL: sourceURL}
			if err := validateFeedbackEvent(event); err != nil {
				t.Fatalf("expected valid feedback metadata, got %v", err)
			}
		})
	}
}

func TestSaveFeedbackRejectsUnsupportedSignals(t *testing.T) {
	service := NewService(config.Config{}, cacheStore{}, http.DefaultClient)
	for _, signal := range []string{"", "admin", "delete", "useful;drop table"} {
		t.Run(signal, func(t *testing.T) {
			err := service.SaveFeedback(context.Background(), FeedbackEvent{TargetType: "insight", TargetID: "1", Signal: signal})
			if err == nil || !strings.Contains(err.Error(), "unsupported feedback signal") {
				t.Fatalf("expected unsupported signal error, got %v", err)
			}
		})
	}
}

func TestShareTweetValidatesTargetBeforeProviderSideEffect(t *testing.T) {
	providerCalls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		providerCalls++
		return nil, nil
	})}
	service := NewService(config.Config{OneCLIGateway: true}, cacheStore{}, client)
	_, err := service.ShareTweet(context.Background(), TweetShareRequest{Text: "publish me"})
	if err == nil || !strings.Contains(err.Error(), "targetType and targetId") {
		t.Fatalf("expected target validation error, got %v", err)
	}
	if providerCalls != 0 {
		t.Fatalf("provider called %d times before validation", providerCalls)
	}
}

func TestTruncateTweetTextNormalizesAndPreservesRuneLimit(t *testing.T) {
	if got := truncateTweetText("  hello   world \n "); got != "hello world" {
		t.Fatalf("normalized tweet = %q", got)
	}
	long := strings.Repeat("界", 281)
	got := truncateTweetText(long)
	if runes := len([]rune(got)); runes != 280 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated tweet has %d runes and suffix %q", runes, got[len(got)-3:])
	}
}
