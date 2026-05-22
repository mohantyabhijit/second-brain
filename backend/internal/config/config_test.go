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
	if !reflect.DeepEqual(cfg.AllowedOrigins, []string{"http://localhost:3000", "http://127.0.0.1:3000"}) {
		t.Fatalf("unexpected default allowed origins: %#v", cfg.AllowedOrigins)
	}
	if cfg.YouTubePlaylistID == "" || cfg.OpenAITranslationModel != "gpt-4o-mini" || cfg.OpenAISynthesisModel != "gpt-4o-mini" {
		t.Fatalf("expected model and playlist defaults, got %#v", cfg)
	}
}
