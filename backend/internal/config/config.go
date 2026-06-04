package config

import (
	"os"
	"strings"
)

const (
	DefaultOwnerID           = "00000000-0000-0000-0000-000000000001"
	DefaultPublicOwnerHandle = "abhijitmohanty"
	defaultYouTubePlaylistID = "PLH_SZ1gwLn4gpQyZICprtx3nKRYGPKE7r"
)

type Config struct {
	Env                          string
	Port                         string
	OwnerID                      string
	PublicOwnerID                string
	PublicOwnerHandle            string
	PublicOwnerEmail             string
	DatabaseURL                  string
	SupabaseDatabaseURL          string
	SupabaseURL                  string
	SupabasePublishableKey       string
	SupabaseStorageKey           string
	SupabaseStorageBucket        string
	ObjectStorageBackend         string
	ObjectStorageRoot            string
	ObjectStorageBucket          string
	AllowedOrigins               []string
	OneCLIBin                    string
	OneCLIGateway                bool
	OneCLIProject                string
	OneCLIXAccessSecretID        string
	OneCLIXRefreshSecretID       string
	XBookmarkLimit               int
	XBookmarkProcessLimit        int
	KnowledgeRunPath             string
	XClientID                    string
	XClientSecret                string
	XRedirectURI                 string
	XOAuthScopes                 []string
	XSessionSecret               string
	XSessionCookieName           string
	XTokenEncryptionKey          string
	XTokenRefreshDirect          bool
	XRequireStoredOAuth          bool
	XExpectedUsername            string
	XReauthorizeCommand          string
	XKeychainTokenSuffix         string
	XTokenRotationPath           string
	YouTubePlaylistID            string
	YouTubeTranscriptTestVideoID string
	OpenAITranslationModel       string
	OpenAISynthesisModel         string
	OpenAIChatModel              string
	OpenAIEmbeddingModel         string
	OpenAIImageModel             string
	PublicBaseURL                string
	DigestEmailTo                string
	DigestEmailFrom              string
	ResendAPIKey                 string
	DigestTimezone               string
	DigestTime                   string
	RefreshTimeout               string
	ProcessWorkerCount           int
	WorkerRefreshInterval        string
	RedisURL                     string
	RedisCacheEnabled            bool
	RedisCacheTTL                string
	RedisRefreshStatusTTL        string
	RedisAskAnswerTTL            string
	CloudflareAPIToken           string
	CloudflareZoneID             string
	CloudflareAPIBaseURL         string
	CloudflareCachePurgeEnabled  bool
	MemoryProfilingEnabled       bool
	MemoryProfilingToken         string
	Neo4jURI                     string
	Neo4jUsername                string
	Neo4jPassword                string
	Neo4jDatabase                string
	GraphSyncBatchSize           int
}

func Load() Config {
	databaseURL := firstEnv("DATABASE_URL", "SUPABASE_DB_URL")
	storageBucket := value("OBJECT_STORAGE_BUCKET", value("SUPABASE_STORAGE_BUCKET", "sources"))
	return Config{
		Env:                          value("APP_ENV", "development"),
		Port:                         value("PORT", "8080"),
		OwnerID:                      value("APP_USER_ID", value("PUBLIC_OWNER_ID", DefaultOwnerID)),
		PublicOwnerID:                value("PUBLIC_OWNER_ID", DefaultOwnerID),
		PublicOwnerHandle:            value("PUBLIC_OWNER_HANDLE", DefaultPublicOwnerHandle),
		PublicOwnerEmail:             os.Getenv("PUBLIC_OWNER_EMAIL"),
		DatabaseURL:                  databaseURL,
		SupabaseDatabaseURL:          databaseURL,
		SupabaseURL:                  os.Getenv("SUPABASE_URL"),
		SupabasePublishableKey:       firstEnv("SUPABASE_PUBLISHABLE_KEY", "NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY"),
		SupabaseStorageKey:           os.Getenv("SUPABASE_SERVICE_ROLE_KEY"),
		SupabaseStorageBucket:        storageBucket,
		ObjectStorageBackend:         value("OBJECT_STORAGE_BACKEND", defaultObjectStorageBackend()),
		ObjectStorageRoot:            os.Getenv("OBJECT_STORAGE_ROOT"),
		ObjectStorageBucket:          storageBucket,
		AllowedOrigins:               csv(value("ALLOWED_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000")),
		OneCLIBin:                    value("ONECLI_BIN", "/Users/abhijitmohanty/.local/bin/onecli"),
		OneCLIGateway:                os.Getenv("ONECLI_GATEWAY") == "true",
		OneCLIProject:                value("ONECLI_PROJECT", "second-brain"),
		OneCLIXAccessSecretID:        os.Getenv("ONECLI_X_ACCESS_SECRET_ID"),
		OneCLIXRefreshSecretID:       os.Getenv("ONECLI_X_REFRESH_SECRET_ID"),
		XBookmarkLimit:               intValue("X_BOOKMARK_LIMIT", 0),
		XBookmarkProcessLimit:        intValue("X_BOOKMARK_PROCESS_LIMIT", 50),
		KnowledgeRunPath:             value("KNOWLEDGE_RUN_PATH", "../data/runtime/latest-knowledge-run.json"),
		XClientID:                    firstEnv("X_CLIENT_ID", "X_CLIENT_ID_PROD"),
		XClientSecret:                firstEnv("X_CLIENT_SECRET", "X_CLIENT_SECRET_PROD"),
		XRedirectURI:                 value("X_REDIRECT_URI", "http://localhost:8080/api/auth/x/callback"),
		XOAuthScopes:                 fields(value("X_OAUTH_SCOPES", "tweet.read users.read bookmark.read offline.access")),
		XSessionSecret:               os.Getenv("X_SESSION_SECRET"),
		XSessionCookieName:           value("X_SESSION_COOKIE_NAME", "second_brain_session"),
		XTokenEncryptionKey:          os.Getenv("X_TOKEN_ENCRYPTION_KEY"),
		XTokenRefreshDirect:          os.Getenv("X_TOKEN_REFRESH_DIRECT") == "true",
		XRequireStoredOAuth:          os.Getenv("X_REQUIRE_STORED_OAUTH") == "true",
		XExpectedUsername:            value("X_EXPECTED_USERNAME", "mohantyabhijit"),
		XReauthorizeCommand:          value("X_REAUTHORIZE_COMMAND", "npm run x:oauth"),
		XKeychainTokenSuffix:         os.Getenv("X_KEYCHAIN_TOKEN_SUFFIX"),
		XTokenRotationPath:           value("X_TOKEN_ROTATION_PATH", "../data/runtime/x-token-rotation.json"),
		YouTubePlaylistID:            value("YOUTUBE_PLAYLIST_ID", defaultYouTubePlaylistID),
		YouTubeTranscriptTestVideoID: os.Getenv("YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID"),
		OpenAITranslationModel:       value("OPENAI_TRANSLATION_MODEL", "gpt-4o-mini"),
		OpenAISynthesisModel:         value("OPENAI_SYNTHESIS_MODEL", "gpt-5.5"),
		OpenAIChatModel:              value("OPENAI_CHAT_MODEL", value("OPENAI_SYNTHESIS_MODEL", "gpt-5.5")),
		OpenAIEmbeddingModel:         value("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		OpenAIImageModel:             value("OPENAI_IMAGE_MODEL", "gpt-image-1"),
		PublicBaseURL:                value("PUBLIC_BASE_URL", defaultPublicBaseURL(value("APP_ENV", "development"))),
		DigestEmailTo:                os.Getenv("DIGEST_EMAIL_TO"),
		DigestEmailFrom:              value("DIGEST_EMAIL_FROM", "Second Brain <digest@second-brain.local>"),
		ResendAPIKey:                 os.Getenv("RESEND_API_KEY"),
		DigestTimezone:               value("DIGEST_TIMEZONE", "Asia/Singapore"),
		DigestTime:                   value("DIGEST_TIME", "18:00"),
		RefreshTimeout:               value("REFRESH_TIMEOUT", "90m"),
		ProcessWorkerCount:           intValue("PROCESS_WORKER_COUNT", 8),
		WorkerRefreshInterval:        value("WORKER_REFRESH_INTERVAL", "2h"),
		RedisURL:                     os.Getenv("REDIS_URL"),
		RedisCacheEnabled:            value("REDIS_CACHE_ENABLED", "false") == "true",
		RedisCacheTTL:                value("REDIS_CACHE_TTL", "720h"),
		RedisRefreshStatusTTL:        value("REDIS_REFRESH_STATUS_TTL", "24h"),
		RedisAskAnswerTTL:            value("REDIS_ASK_ANSWER_TTL", "1h"),
		CloudflareAPIToken:           os.Getenv("CLOUDFLARE_API_TOKEN"),
		CloudflareZoneID:             os.Getenv("CLOUDFLARE_ZONE_ID"),
		CloudflareAPIBaseURL:         value("CLOUDFLARE_API_BASE_URL", "https://api.cloudflare.com/client/v4"),
		CloudflareCachePurgeEnabled:  value("CLOUDFLARE_CACHE_PURGE_ENABLED", "true") == "true",
		MemoryProfilingEnabled:       value("MEMORY_PROFILING_ENABLED", "false") == "true",
		MemoryProfilingToken:         os.Getenv("MEMORY_PROFILE_TOKEN"),
		Neo4jURI:                     os.Getenv("NEO4J_URI"),
		Neo4jUsername:                os.Getenv("NEO4J_USERNAME"),
		Neo4jPassword:                os.Getenv("NEO4J_PASSWORD"),
		Neo4jDatabase:                value("NEO4J_DATABASE", "neo4j"),
		GraphSyncBatchSize:           intValue("GRAPH_SYNC_BATCH_SIZE", 50),
	}
}

func defaultObjectStorageBackend() string {
	if strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ROOT")) != "" {
		return "filesystem"
	}
	return "none"
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
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

func fields(raw string) []string {
	values := strings.Fields(raw)
	if len(values) == 0 {
		return nil
	}
	return values
}
