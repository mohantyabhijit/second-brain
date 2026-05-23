package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type xCreateTweetResponse struct {
	Data *struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

func (s *Service) ShareTweet(ctx context.Context, input TweetShareRequest) (TweetShareResult, error) {
	text := truncateTweetText(input.Text)
	if text == "" {
		return TweetShareResult{}, fmt.Errorf("tweet text is required")
	}
	accessToken, err := s.refreshXAccessToken(ctx)
	if err != nil {
		return TweetShareResult{}, err
	}
	headers := xAccessHeaders(accessToken)
	headers.Set("Content-Type", "application/json")
	raw, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return TweetShareResult{}, err
	}
	var response xCreateTweetResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.x.com/2/tweets", headers, bytes.NewReader(raw), &response); err != nil {
		return TweetShareResult{}, fmt.Errorf("create X post: %w", err)
	}
	if response.Data == nil || strings.TrimSpace(response.Data.ID) == "" {
		return TweetShareResult{}, fmt.Errorf("create X post returned no post id")
	}
	feedback := FeedbackEvent{
		TargetType: input.TargetType,
		TargetID:   input.TargetID,
		Signal:     "tweeted",
		Note:       "tweet_id=" + response.Data.ID,
		SourceURL:  input.SourceURL,
	}
	if err := s.SaveFeedback(ctx, feedback); err != nil {
		s.logger.Warn("save tweet feedback failed", "error", err)
	}
	return TweetShareResult{ID: response.Data.ID, Text: response.Data.Text}, nil
}

func truncateTweetText(value string) string {
	text := strings.Join(strings.Fields(value), " ")
	if len([]rune(text)) <= 280 {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:277])) + "..."
}
