package config

import (
	"reflect"
	"testing"
)

func TestLoadAppliesDefaultsAndParsesCSV(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("PORT", "9090")
	t.Setenv("SUPABASE_DB_URL", "postgres://example")
	t.Setenv("SUPABASE_URL", "https://supabase.example")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role")
	t.Setenv("SUPABASE_STORAGE_BUCKET", "source-artifacts")
	t.Setenv("ALLOWED_ORIGINS", " https://app.example, ,http://localhost:3000 ")
	t.Setenv("ONECLI_BIN", "/tmp/onecli")
	t.Setenv("ONECLI_GATEWAY", "true")
	t.Setenv("X_BOOKMARK_LIMIT", "250")
	t.Setenv("X_CLIENT_ID_PROD", "prod-client-id")
	t.Setenv("X_CLIENT_SECRET_PROD", "prod-client-secret")
	t.Setenv("KNOWLEDGE_RUN_PATH", "/tmp/latest.json")
	t.Setenv("YOUTUBE_PLAYLIST_ID", "playlist-1")
	t.Setenv("YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID", "video-1")
	t.Setenv("OPENAI_TRANSLATION_MODEL", "translation-model")
	t.Setenv("OPENAI_SYNTHESIS_MODEL", "synthesis-model")

	cfg := Load()

	if cfg.Env != "test" || cfg.Port != "9090" || cfg.OneCLIBin != "/tmp/onecli" || !cfg.OneCLIGateway {
		t.Fatalf("unexpected basic config: %#v", cfg)
	}
	if cfg.SupabaseDatabaseURL != "postgres://example" || cfg.SupabaseURL != "https://supabase.example" || cfg.SupabaseStorageKey != "service-role" {
		t.Fatalf("unexpected Supabase config: %#v", cfg)
	}
	if cfg.SupabaseStorageBucket != "source-artifacts" || cfg.KnowledgeRunPath != "/tmp/latest.json" {
		t.Fatalf("unexpected storage/run config: %#v", cfg)
	}
	if cfg.XBookmarkLimit != 250 {
		t.Fatalf("unexpected X bookmark limit: %d", cfg.XBookmarkLimit)
	}
	if cfg.XClientID != "prod-client-id" || cfg.XClientSecret != "prod-client-secret" {
		t.Fatalf("expected production X client fallback, got %#v", cfg)
	}
	if cfg.YouTubePlaylistID != "playlist-1" || cfg.YouTubeTranscriptTestVideoID != "video-1" {
		t.Fatalf("unexpected YouTube config: %#v", cfg)
	}
	if cfg.OpenAITranslationModel != "translation-model" || cfg.OpenAISynthesisModel != "synthesis-model" {
		t.Fatalf("unexpected OpenAI config: %#v", cfg)
	}
	if !reflect.DeepEqual(cfg.AllowedOrigins, []string{"https://app.example", "http://localhost:3000"}) {
		t.Fatalf("unexpected allowed origins: %#v", cfg.AllowedOrigins)
	}
}

func TestLoadFallsBackForBlankValues(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("PORT", "")
	t.Setenv("SUPABASE_STORAGE_BUCKET", "")
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("ONECLI_BIN", "")
	t.Setenv("X_BOOKMARK_LIMIT", "")
	t.Setenv("KNOWLEDGE_RUN_PATH", "")
	t.Setenv("YOUTUBE_PLAYLIST_ID", "")
	t.Setenv("OPENAI_TRANSLATION_MODEL", "")
	t.Setenv("OPENAI_SYNTHESIS_MODEL", "")

	cfg := Load()

	if cfg.Env != "development" || cfg.Port != "8080" {
		t.Fatalf("expected env/port defaults, got %#v", cfg)
	}
	if cfg.SupabaseStorageBucket != "sources" {
		t.Fatalf("expected default storage bucket, got %q", cfg.SupabaseStorageBucket)
	}
	if cfg.XBookmarkLimit != 0 {
		t.Fatalf("expected all-bookmarks default limit, got %d", cfg.XBookmarkLimit)
	}
	if !reflect.DeepEqual(cfg.AllowedOrigins, []string{"http://localhost:3000", "http://127.0.0.1:3000"}) {
		t.Fatalf("unexpected default allowed origins: %#v", cfg.AllowedOrigins)
	}
	if cfg.YouTubePlaylistID == "" || cfg.OpenAITranslationModel != "gpt-4o-mini" || cfg.OpenAISynthesisModel != "gpt-4o-mini" {
		t.Fatalf("expected model and playlist defaults, got %#v", cfg)
	}
}
