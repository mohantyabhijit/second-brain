package config

import (
	"os"
	"strings"
)

const defaultYouTubePlaylistID = "PLH_SZ1gwLn4gpQyZICprtx3nKRYGPKE7r"

type Config struct {
	Env                          string
	Port                         string
	SupabaseDatabaseURL          string
	AllowedOrigins               []string
	OneCLIBin                    string
	OneCLIGateway                bool
	YouTubePlaylistID            string
	YouTubeTranscriptTestVideoID string
	OpenAITranslationModel       string
}

func Load() Config {
	return Config{
		Env:                          value("APP_ENV", "development"),
		Port:                         value("PORT", "8080"),
		SupabaseDatabaseURL:          os.Getenv("SUPABASE_DB_URL"),
		AllowedOrigins:               csv(value("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),
		OneCLIBin:                    value("ONECLI_BIN", "/Users/abhijitmohanty/.local/bin/onecli"),
		OneCLIGateway:                os.Getenv("ONECLI_GATEWAY") == "true",
		YouTubePlaylistID:            value("YOUTUBE_PLAYLIST_ID", defaultYouTubePlaylistID),
		YouTubeTranscriptTestVideoID: os.Getenv("YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID"),
		OpenAITranslationModel:       value("OPENAI_TRANSLATION_MODEL", "gpt-4o-mini"),
	}
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func csv(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}
