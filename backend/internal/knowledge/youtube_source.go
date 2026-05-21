package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type playlistResponse struct {
	Items []struct {
		Snippet *struct {
			Title        string `json:"title"`
			ChannelTitle string `json:"channelTitle"`
			PublishedAt  string `json:"publishedAt"`
			ResourceID   *struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
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
			ChannelTitle:     item.Snippet.ChannelTitle,
			PublishedAt:      item.Snippet.PublishedAt,
			SourceURL:        "https://www.youtube.com/watch?v=" + item.Snippet.ResourceID.VideoID,
			TranscriptStatus: "untested",
		})
	}
	return items, nil
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
	requestURL = appendQueryValue(requestURL, "text", "true")
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

	if payload.Lang != "" && payload.Lang != "en" {
		translated, err := s.translateTranscriptPreviewToEnglish(ctx, text, payload.Lang)
		if err != nil {
			return YouTubeItem{
				TranscriptStatus:            "available",
				TranscriptPreview:           truncate(text, 1200),
				TranscriptOriginalPreview:   truncate(text, 1200),
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
			TranscriptLang:              "en",
			TranscriptSourceLang:        payload.Lang,
			TranscriptAvailableLangs:    payload.AvailableLangs,
			TranscriptTranslationStatus: "translated",
		}
	}

	return YouTubeItem{
		TranscriptStatus:            "available",
		TranscriptPreview:           truncate(text, 1200),
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
	item.TranscriptError = transcript.TranscriptError
	return item
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
