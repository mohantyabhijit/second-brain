package config

import (
	"os"
	"strings"
)

const defaultYouTubePlaylistID = "PLH_SZ1gwLn4gpQyZICprtx3nKRYGPKE7r"

type Config struct {
	Env                          string
	Port                         string
	OwnerID                      string
	SupabaseDatabaseURL          string
	SupabaseURL                  string
	SupabaseStorageKey           string
	SupabaseStorageBucket        string
	AllowedOrigins               []string
	OneCLIBin                    string
	OneCLIGateway                bool
	OneCLIProject                string
	OneCLIXAccessSecretID        string
	OneCLIXRefreshSecretID       string
	XBookmarkLimit               int
	KnowledgeRunPath             string
	XClientID                    string
	XClientSecret                string
	YouTubePlaylistID            string
	YouTubeTranscriptTestVideoID string
	OpenAITranslationModel       string
	OpenAISynthesisModel         string
	OpenAIEmbeddingModel         string
	DigestEmailTo                string
	DigestEmailFrom              string
	ResendAPIKey                 string
	DigestTimezone               string
}

func Load() Config {
	return Config{
		Env:                          value("APP_ENV", "development"),
		Port:                         value("PORT", "8080"),
		OwnerID:                      value("APP_USER_ID", "00000000-0000-0000-0000-000000000001"),
		SupabaseDatabaseURL:          os.Getenv("SUPABASE_DB_URL"),
		SupabaseURL:                  os.Getenv("SUPABASE_URL"),
		SupabaseStorageKey:           os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseStorageBucket:        value("SUPABASE_STORAGE_BUCKET", "sources"),
		AllowedOrigins:               csv(value("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),
		OneCLIBin:                    value("ONECLI_BIN", "/Users/abhijitmohanty/.local/bin/onecli"),
		OneCLIGateway:                os.Getenv("ONECLI_GATEWAY") == "true",
		OneCLIProject:                value("ONECLI_PROJECT", "second-brain"),
		OneCLIXAccessSecretID:        os.Getenv("ONECLI_X_ACCESS_SECRET_ID"),
		OneCLIXRefreshSecretID:       os.Getenv("ONECLI_X_REFRESH_SECRET_ID"),
		XBookmarkLimit:               intValue("X_BOOKMARK_LIMIT", 0),
		KnowledgeRunPath:             value("KNOWLEDGE_RUN_PATH", "../data/runtime/latest-knowledge-run.json"),
		XClientID:                    os.Getenv("X_CLIENT_ID"),
		XClientSecret:                os.Getenv("X_CLIENT_SECRET"),
		YouTubePlaylistID:            value("YOUTUBE_PLAYLIST_ID", defaultYouTubePlaylistID),
		YouTubeTranscriptTestVideoID: os.Getenv("YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID"),
		OpenAITranslationModel:       value("OPENAI_TRANSLATION_MODEL", "gpt-4o-mini"),
		OpenAISynthesisModel:         value("OPENAI_SYNTHESIS_MODEL", "gpt-4o-mini"),
		OpenAIEmbeddingModel:         value("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		DigestEmailTo:                os.Getenv("DIGEST_EMAIL_TO"),
		DigestEmailFrom:              value("DIGEST_EMAIL_FROM", "Second Brain <digest@second-brain.local>"),
		ResendAPIKey:                 os.Getenv("RESEND_API_KEY"),
		DigestTimezone:               value("DIGEST_TIMEZONE", "Asia/Singapore"),
	}
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func intValue(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var value int
	for _, r := range raw {
		if r < '0' || r > '9' {
			return fallback
		}
		value = value*10 + int(r-'0')
	}
	return value
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
