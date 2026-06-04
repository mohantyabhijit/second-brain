package config

import (
	"reflect"
	"testing"
)

func TestLoadAppliesDefaultsAndParsesCSV(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://primary")
	t.Setenv("SUPABASE_DB_URL", "postgres://example")
	t.Setenv("SUPABASE_URL", "https://supabase.example")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role")
	t.Setenv("SUPABASE_STORAGE_BUCKET", "source-artifacts")
	t.Setenv("OBJECT_STORAGE_BACKEND", "filesystem")
	t.Setenv("OBJECT_STORAGE_ROOT", "/srv/second-brain/object-storage")
	t.Setenv("OBJECT_STORAGE_BUCKET", "sources")
	t.Setenv("ALLOWED_ORIGINS", " https://app.example, ,http://localhost:3000 ")
	t.Setenv("ONECLI_BIN", "/tmp/onecli")
	t.Setenv("ONECLI_GATEWAY", "true")
	t.Setenv("X_BOOKMARK_LIMIT", "250")
	t.Setenv("X_BOOKMARK_PROCESS_LIMIT", "50")
	t.Setenv("X_CLIENT_ID_PROD", "prod-client-id")
	t.Setenv("X_CLIENT_SECRET_PROD", "prod-client-secret")
	t.Setenv("KNOWLEDGE_RUN_PATH", "/tmp/latest.json")
	t.Setenv("YOUTUBE_PLAYLIST_ID", "playlist-1")
	t.Setenv("YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID", "video-1")
	t.Setenv("SUPADATA_MONTHLY_REQUEST_LIMIT", "90")
	t.Setenv("OPENAI_TRANSLATION_MODEL", "translation-model")
	t.Setenv("OPENAI_SYNTHESIS_MODEL", "synthesis-model")
	t.Setenv("OPENAI_IMAGE_MODEL", "image-model")
	t.Setenv("PUBLIC_BASE_URL", "https://example.com/second-brain")

	cfg := Load()

	if cfg.Env != "test" || cfg.Port != "9090" || cfg.OneCLIBin != "/tmp/onecli" || !cfg.OneCLIGateway {
		t.Fatalf("unexpected basic config: %#v", cfg)
	}
	if cfg.DatabaseURL != "postgres://primary" {
		t.Fatalf("unexpected database config: %#v", cfg)
	}
	if cfg.SupabaseURL != "https://supabase.example" {
		t.Fatalf("unexpected Supabase config: %#v", cfg)
	}
	if cfg.ObjectStorageBucket != "sources" || cfg.ObjectStorageBackend != "filesystem" || cfg.ObjectStorageRoot != "/srv/second-brain/object-storage" {
		t.Fatalf("unexpected object storage config: %#v", cfg)
	}
	if cfg.KnowledgeRunPath != "/tmp/latest.json" {
		t.Fatalf("unexpected storage/run config: %#v", cfg)
	}
	if cfg.XBookmarkLimit != 250 {
		t.Fatalf("unexpected X bookmark limit: %d", cfg.XBookmarkLimit)
	}
	if cfg.XBookmarkProcessLimit != 50 {
		t.Fatalf("unexpected X bookmark process limit: %d", cfg.XBookmarkProcessLimit)
	}
	if cfg.XClientID != "prod-client-id" || cfg.XClientSecret != "prod-client-secret" {
		t.Fatalf("expected production X client fallback, got %#v", cfg)
	}
	if cfg.YouTubePlaylistID != "playlist-1" || cfg.YouTubeTranscriptTestVideoID != "video-1" {
		t.Fatalf("unexpected YouTube config: %#v", cfg)
	}
	if cfg.SupadataMonthlyRequestLimit != 90 {
		t.Fatalf("unexpected Supadata monthly request limit: %d", cfg.SupadataMonthlyRequestLimit)
	}
	if cfg.OpenAITranslationModel != "translation-model" || cfg.OpenAISynthesisModel != "synthesis-model" || cfg.OpenAIImageModel != "image-model" {
		t.Fatalf("unexpected OpenAI config: %#v", cfg)
	}
	if cfg.PublicBaseURL != "https://example.com/second-brain" {
		t.Fatalf("unexpected public base URL: %#v", cfg)
	}
	if !reflect.DeepEqual(cfg.AllowedOrigins, []string{"https://app.example", "http://localhost:3000"}) {
		t.Fatalf("unexpected allowed origins: %#v", cfg.AllowedOrigins)
	}
}

func TestLoadFallsBackForBlankValues(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("OBJECT_STORAGE_BUCKET", "")
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("ONECLI_BIN", "")
	t.Setenv("X_BOOKMARK_LIMIT", "")
	t.Setenv("X_BOOKMARK_PROCESS_LIMIT", "")
	t.Setenv("KNOWLEDGE_RUN_PATH", "")
	t.Setenv("YOUTUBE_PLAYLIST_ID", "")
	t.Setenv("SUPADATA_MONTHLY_REQUEST_LIMIT", "")
	t.Setenv("OPENAI_TRANSLATION_MODEL", "")
	t.Setenv("OPENAI_SYNTHESIS_MODEL", "")
	t.Setenv("OPENAI_IMAGE_MODEL", "")
	t.Setenv("PUBLIC_BASE_URL", "")

	cfg := Load()

	if cfg.Env != "development" || cfg.Port != "8080" {
		t.Fatalf("expected env/port defaults, got %#v", cfg)
	}
	if cfg.DatabaseURL != "" || cfg.ObjectStorageBucket != "sources" || cfg.ObjectStorageBackend != "none" || cfg.ObjectStorageRoot != "" {
		t.Fatalf("unexpected default database/object storage config: %#v", cfg)
	}
	if cfg.XBookmarkLimit != 0 || cfg.XBookmarkProcessLimit != 50 {
		t.Fatalf("expected all-bookmarks fetch and 50-bookmark process defaults, got fetch=%d process=%d", cfg.XBookmarkLimit, cfg.XBookmarkProcessLimit)
	}
	if !reflect.DeepEqual(cfg.AllowedOrigins, []string{"http://localhost:3000", "http://127.0.0.1:3000"}) {
		t.Fatalf("unexpected default allowed origins: %#v", cfg.AllowedOrigins)
	}
	if cfg.YouTubePlaylistID == "" || cfg.OpenAITranslationModel != "gpt-4o-mini" || cfg.OpenAISynthesisModel != "gpt-5.4-mini" || cfg.OpenAIImageModel != "gpt-image-1" {
		t.Fatalf("expected model and playlist defaults, got %#v", cfg)
	}
	if cfg.SupadataMonthlyRequestLimit != 100 {
		t.Fatalf("expected free-tier Supadata request limit, got %d", cfg.SupadataMonthlyRequestLimit)
	}
	if cfg.PublicBaseURL != "http://localhost:8080" {
		t.Fatalf("expected local public base URL default, got %#v", cfg)
	}
}

func TestLoadIgnoresLegacySupabaseDataAndStorageVariables(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SUPABASE_DB_URL", "postgres://legacy-supabase")
	t.Setenv("SUPABASE_URL", "https://supabase.example")
	t.Setenv("SUPABASE_PUBLISHABLE_KEY", "publishable-key")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "legacy-storage-key")
	t.Setenv("SUPABASE_STORAGE_BUCKET", "legacy-bucket")
	t.Setenv("OBJECT_STORAGE_BACKEND", "")
	t.Setenv("OBJECT_STORAGE_ROOT", "")
	t.Setenv("OBJECT_STORAGE_BUCKET", "")

	cfg := Load()

	if cfg.DatabaseURL != "" || cfg.ObjectStorageBackend != "none" || cfg.ObjectStorageBucket != "sources" {
		t.Fatalf("expected legacy Supabase data/storage variables to be ignored, got %#v", cfg)
	}
	if cfg.SupabaseURL != "https://supabase.example" || cfg.SupabasePublishableKey != "publishable-key" {
		t.Fatalf("expected Supabase Auth variables to remain configured, got %#v", cfg)
	}
}
