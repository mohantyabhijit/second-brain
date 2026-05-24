package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

type playlistResponse struct {
	Items []struct {
		Snippet *struct {
			Title        string `json:"title"`
			Description  string `json:"description"`
			ChannelTitle string `json:"channelTitle"`
			PublishedAt  string `json:"publishedAt"`
			ResourceID   *struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
	} `json:"items"`
}

type videoDetailsResponse struct {
	Items []struct {
		ID             string `json:"id"`
		ContentDetails *struct {
			Duration string `json:"duration"`
		} `json:"contentDetails"`
	} `json:"items"`
}

type supadataTranscriptResponse struct {
	Content        any      `json:"content"`
	Lang           string   `json:"lang"`
	AvailableLangs []string `json:"availableLangs"`
	JobID          string   `json:"jobId"`
	Status         string   `json:"status"`
	Error          string   `json:"error"`
	Message        string   `json:"message"`
}

var youtubeDurationPattern = regexp.MustCompile(`^P(?:\d+Y)?(?:\d+M)?(?:\d+W)?(?:\d+D)?T?(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)

type openAIResponse struct {
	OutputText string `json:"output_text"`
	Output     []struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type transcriptAttempt struct {
	label string
	lang  string
	mode  string
}

func (s *Service) fetchYouTubeInboxItems(ctx context.Context, playlistID string, transcriptVideoID string) ([]YouTubeItem, error) {
	items, err := s.fetchPlaylistItems(ctx, playlistID, 5)
	if err != nil {
		return nil, err
	}

	for index, item := range items {
		if transcriptVideoID != "" && item.VideoID != transcriptVideoID {
			continue
		}
		transcript := s.fetchSupadataTranscript(ctx, item.VideoID)
		items[index] = mergeTranscript(item, transcript)
	}
	return items, nil
}

func (s *Service) fetchPlaylistItems(ctx context.Context, playlistID string, limit int) ([]YouTubeItem, error) {
	if os.Getenv("YOUTUBE_API_KEY") == "" && os.Getenv("YOUTUBE_ACCESS_TOKEN") == "" && !s.cfg.OneCLIGateway {
		return nil, fmt.Errorf(credentialHint("YOUTUBE_API_KEY or YOUTUBE_ACCESS_TOKEN"))
	}

	requestURL := "https://www.googleapis.com/youtube/v3/playlistItems"
	requestURL = appendQueryValue(requestURL, "part", "snippet")
	requestURL = appendQueryValue(requestURL, "playlistId", playlistID)
	requestURL = appendQueryValue(requestURL, "maxResults", fmt.Sprintf("%d", limit))
	requestURL = appendQueryValue(requestURL, "key", os.Getenv("YOUTUBE_API_KEY"))

	headers := authHeader("YOUTUBE_ACCESS_TOKEN", "Bearer {value}")
	var payload playlistResponse
	if err := s.requestJSON(ctx, http.MethodGet, requestURL, headers, nil, &payload); err != nil {
		return nil, fmt.Errorf("YouTube playlist validation failed: %w", err)
	}

	items := []YouTubeItem{}
	for _, item := range payload.Items {
		if item.Snippet == nil || item.Snippet.ResourceID == nil || item.Snippet.ResourceID.VideoID == "" {
			continue
		}
		items = append(items, YouTubeItem{
			VideoID:          item.Snippet.ResourceID.VideoID,
			Title:            fallback(item.Snippet.Title, "Untitled YouTube video"),
			Description:      strings.TrimSpace(item.Snippet.Description),
			ChannelTitle:     item.Snippet.ChannelTitle,
			PublishedAt:      item.Snippet.PublishedAt,
			SourceURL:        "https://www.youtube.com/watch?v=" + item.Snippet.ResourceID.VideoID,
			TranscriptStatus: "untested",
		})
	}
	items = s.attachVideoDurations(ctx, items)
	return items, nil
}

func (s *Service) attachVideoDurations(ctx context.Context, items []YouTubeItem) []YouTubeItem {
	if len(items) == 0 {
		return items
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.VideoID)
	}
	requestURL := "https://www.googleapis.com/youtube/v3/videos"
	requestURL = appendQueryValue(requestURL, "part", "contentDetails")
	requestURL = appendQueryValue(requestURL, "id", strings.Join(ids, ","))
	requestURL = appendQueryValue(requestURL, "key", os.Getenv("YOUTUBE_API_KEY"))

	headers := authHeader("YOUTUBE_ACCESS_TOKEN", "Bearer {value}")
	var payload videoDetailsResponse
	if err := s.requestJSON(ctx, http.MethodGet, requestURL, headers, nil, &payload); err != nil {
		if s.logger != nil {
			s.logger.Warn("youtube duration fetch skipped", "error", err)
		}
		return items
	}
	durations := map[string]int{}
	for _, item := range payload.Items {
		if item.ContentDetails == nil {
			continue
		}
		if seconds := parseYouTubeDuration(item.ContentDetails.Duration); seconds > 0 {
			durations[item.ID] = seconds
		}
	}
	for index := range items {
		items[index].DurationSeconds = durations[items[index].VideoID]
	}
	return items
}

func (s *Service) fetchSupadataTranscript(ctx context.Context, videoID string) YouTubeItem {
	if os.Getenv("SUPADATA_API_KEY") == "" && !s.cfg.OneCLIGateway {
		return YouTubeItem{TranscriptStatus: "blocked", TranscriptError: credentialHint("SUPADATA_API_KEY")}
	}

	attempts := []transcriptAttempt{
		{label: "english native", lang: "en", mode: "native"},
		{label: "native auto-language", mode: "native"},
		{label: "hindi native", lang: "hi", mode: "native"},
		{label: "default transcript"},
	}

	var missingResults []string
	for _, attempt := range attempts {
		transcript := s.fetchSupadataTranscriptAttempt(ctx, videoID, attempt)
		if transcript.TranscriptStatus == "available" || transcript.TranscriptStatus == "blocked" {
			return transcript
		}
		missingResults = append(missingResults, fmt.Sprintf("%s: %s", attempt.label, transcript.TranscriptError))
	}

	return YouTubeItem{
		TranscriptStatus: "missing",
		TranscriptError:  "Transcript unavailable after retrying Supadata variants. " + strings.Join(missingResults, " | "),
	}
}

func (s *Service) fetchSupadataTranscriptAttempt(ctx context.Context, videoID string, attempt transcriptAttempt) YouTubeItem {
	requestURL := "https://api.supadata.ai/v1/transcript"
	requestURL = appendQueryValue(requestURL, "url", "https://www.youtube.com/watch?v="+videoID)
	requestURL = appendQueryValue(requestURL, "text", "false")
	requestURL = appendQueryValue(requestURL, "lang", attempt.lang)
	requestURL = appendQueryValue(requestURL, "mode", attempt.mode)

	headers := apiKeyHeader("SUPADATA_API_KEY", "x-api-key")
	var payload supadataTranscriptResponse
	if err := s.requestJSON(ctx, http.MethodGet, requestURL, headers, nil, &payload); err != nil {
		return YouTubeItem{TranscriptStatus: "blocked", TranscriptError: fmt.Sprintf("Supadata transcript extraction failed: %v", err)}
	}
	if payload.JobID != "" {
		return YouTubeItem{TranscriptStatus: "blocked", TranscriptError: "Supadata returned async job " + payload.JobID + "; Knowledge inbox only accepts immediate native transcripts."}
	}

	text := transcriptText(payload.Content)
	if text == "" {
		return YouTubeItem{TranscriptStatus: "missing", TranscriptError: fallback(payload.Message, fallback(payload.Error, "Supadata returned no transcript text."))}
	}
	timedText := transcriptTimedText(payload.Content)
	timeMarkers := extractTimeMarkers(timedText, 8)

	if payload.Lang != "" && payload.Lang != "en" {
		translated, err := s.translateTranscriptPreviewToEnglish(ctx, text, payload.Lang)
		if err != nil {
			return YouTubeItem{
				TranscriptStatus:            "available",
				TranscriptPreview:           truncate(text, 1200),
				TranscriptOriginalPreview:   truncate(text, 1200),
				TranscriptText:              text,
				TranscriptOriginalText:      text,
				TranscriptTimedText:         timedText,
				ImportantTimeMarkers:        timeMarkers,
				TranscriptLang:              payload.Lang,
				TranscriptSourceLang:        payload.Lang,
				TranscriptAvailableLangs:    payload.AvailableLangs,
				TranscriptTranslationStatus: "blocked",
				TranscriptError:             err.Error(),
			}
		}
		return YouTubeItem{
			TranscriptStatus:            "available",
			TranscriptPreview:           truncate(translated, 1200),
			TranscriptOriginalPreview:   truncate(text, 1200),
			TranscriptText:              translated,
			TranscriptOriginalText:      text,
			TranscriptTimedText:         "",
			ImportantTimeMarkers:        timeMarkers,
			TranscriptLang:              "en",
			TranscriptSourceLang:        payload.Lang,
			TranscriptAvailableLangs:    payload.AvailableLangs,
			TranscriptTranslationStatus: "translated",
		}
	}

	return YouTubeItem{
		TranscriptStatus:            "available",
		TranscriptPreview:           truncate(text, 1200),
		TranscriptText:              text,
		TranscriptTimedText:         timedText,
		ImportantTimeMarkers:        timeMarkers,
		TranscriptLang:              payload.Lang,
		TranscriptSourceLang:        payload.Lang,
		TranscriptAvailableLangs:    payload.AvailableLangs,
		TranscriptTranslationStatus: "none",
	}
}

func (s *Service) translateTranscriptPreviewToEnglish(ctx context.Context, text string, sourceLang string) (string, error) {
	token := os.Getenv("OPENAI_API_KEY")
	if token == "" && !s.cfg.OneCLIGateway {
		return "", fmt.Errorf(credentialHint("OPENAI_API_KEY"))
	}

	requestBody := map[string]any{
		"model": s.cfg.OpenAITranslationModel,
		"input": strings.Join([]string{
			"Translate this transcript excerpt into natural English.",
			"Preserve concrete claims, book titles, names, numbers, and the speaker's meaning.",
			"Do not summarize, add commentary, or omit details.",
			"Source language: " + sourceLang + ".",
			"",
			truncate(text, 5000),
		}, "\n"),
		"max_output_tokens": 2500,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")
	var payload openAIResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/responses", headers, bytes.NewReader(raw), &payload); err != nil {
		return "", fmt.Errorf("OpenAI translation failed: %w", err)
	}
	if payload.Error != nil && payload.Error.Message != "" {
		return "", fmt.Errorf("OpenAI translation failed: %s", payload.Error.Message)
	}
	if payload.OutputText != "" {
		return strings.TrimSpace(payload.OutputText), nil
	}

	parts := []string{}
	for _, output := range payload.Output {
		for _, content := range output.Content {
			if strings.TrimSpace(content.Text) != "" {
				parts = append(parts, strings.TrimSpace(content.Text))
			}
		}
	}
	translated := strings.TrimSpace(strings.Join(parts, "\n"))
	if translated == "" {
		return "", fmt.Errorf("OpenAI translation returned no text")
	}
	return translated, nil
}

func mergeTranscript(item YouTubeItem, transcript YouTubeItem) YouTubeItem {
	item.TranscriptStatus = transcript.TranscriptStatus
	item.TranscriptLang = transcript.TranscriptLang
	item.TranscriptSourceLang = transcript.TranscriptSourceLang
	item.TranscriptAvailableLangs = transcript.TranscriptAvailableLangs
	item.TranscriptTranslationStatus = transcript.TranscriptTranslationStatus
	item.TranscriptPreview = transcript.TranscriptPreview
	item.TranscriptOriginalPreview = transcript.TranscriptOriginalPreview
	item.TranscriptText = transcript.TranscriptText
	item.TranscriptOriginalText = transcript.TranscriptOriginalText
	item.TranscriptTimedText = transcript.TranscriptTimedText
	item.ImportantTimeMarkers = transcript.ImportantTimeMarkers
	item.TranscriptError = transcript.TranscriptError
	if len(item.ImportantTimeMarkers) == 0 {
		item.ImportantTimeMarkers = extractTimeMarkers(item.Description, 8)
	}
	if len(item.ImportantTimeMarkers) == 0 {
		text := fallback(
			fallback(item.TranscriptText, item.TranscriptOriginalText),
			fallback(item.Description, fallback(item.TranscriptPreview, item.TranscriptOriginalPreview)),
		)
		item.ImportantTimeMarkers = estimatedTranscriptMarkers(text, item.DurationSeconds, 3)
	}
	return item
}

func parseYouTubeDuration(value string) int {
	matches := youtubeDurationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 4 {
		return 0
	}
	total := 0
	multipliers := []int{3600, 60, 1}
	for index, match := range matches[1:] {
		if match == "" {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(match, "%d", &number); err != nil {
			return 0
		}
		total += number * multipliers[index]
	}
	return total
}

func transcriptText(content any) string {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			chunk, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := chunk["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func transcriptTimedText(content any) string {
	items, ok := content.([]any)
	if !ok {
		return ""
	}
	type segment struct {
		seconds int
		text    string
	}
	segments := []segment{}
	for _, item := range items {
		chunk, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text, _ := chunk["text"].(string)
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}
		seconds, ok := transcriptSegmentSeconds(chunk)
		if !ok {
			continue
		}
		segments = append(segments, segment{seconds: seconds, text: text})
	}
	if len(segments) == 0 {
		return ""
	}
	sort.SliceStable(segments, func(i int, j int) bool {
		return segments[i].seconds < segments[j].seconds
	})
	lines := make([]string, 0, len(segments))
	for _, segment := range segments {
		lines = append(lines, "["+formatTimeMarkerTimestamp(segment.seconds)+"] "+segment.text)
	}
	return strings.Join(lines, "\n")
}

func transcriptSegmentSeconds(chunk map[string]any) (int, bool) {
	for _, key := range []string{"start", "startTime", "offset", "offsetSec", "offsetSeconds"} {
		if seconds, ok := numericSeconds(chunk[key], false); ok {
			return seconds, true
		}
	}
	for _, key := range []string{"startMs", "offsetMs"} {
		if seconds, ok := numericSeconds(chunk[key], true); ok {
			return seconds, true
		}
	}
	return 0, false
}

func numericSeconds(value any, milliseconds bool) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if typed < 0 {
			return 0, false
		}
		if milliseconds || typed > 100000 {
			typed = typed / 1000
		}
		return int(typed), true
	case int:
		if typed < 0 {
			return 0, false
		}
		if milliseconds || typed > 100000 {
			typed = typed / 1000
		}
		return typed, true
	case string:
		return parseTimestampSeconds(typed)
	default:
		return 0, false
	}
}
