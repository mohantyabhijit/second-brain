package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestInsightGroupingMigrationDefinesPersistenceSurface(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202605230003_insight_grouping_pipeline.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"create table if not exists public.insights",
		"create table if not exists public.insight_evidence",
		"create table if not exists public.insight_embeddings",
		"create table if not exists public.insight_clusters",
		"create table if not exists public.cluster_memberships",
		"unique (source_capture_id, external_insight_id)",
		"primary key (insight_id, evidence_index)",
		"unique (owner_id, embedding_key, model)",
		"unique (owner_id, cluster_layer, external_cluster_key)",
		"primary key (cluster_id, insight_id)",
		"embedding vector(1536) not null",
		"using ivfflat (embedding vector_cosine_ops)",
		"alter table public.insights enable row level security",
		`create policy "insights_no_browser_access"`,
		`create policy "insight_embeddings_no_browser_access"`,
		`create policy "cluster_memberships_no_browser_access"`,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}

func TestXOAuthTokensMigrationDefinesEncryptedBackendStore(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202605230005_x_oauth_tokens.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"create table if not exists public.x_oauth_tokens",
		"owner_id uuid primary key references public.user_profiles(id) on delete cascade",
		"access_token_ciphertext text not null",
		"refresh_token_ciphertext text not null",
		"access_expires_at timestamptz not null",
		"authenticated_x_username text not null default ''",
		"alter table public.x_oauth_tokens enable row level security",
		`create policy "x_oauth_tokens_no_browser_access"`,
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}

func TestDigestSourceLedgerMigrationDefinesDigestMembership(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202605280001_digest_source_ledger.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"create table if not exists public.digest_source_items",
		"digest_issue_id uuid not null references public.digest_issues(id) on delete cascade",
		"source_item_id uuid references public.source_items(id) on delete set null",
		"source_capture_id uuid references public.source_captures(id) on delete set null",
		"knowledge_synthesis_id uuid references public.knowledge_syntheses(id) on delete set null",
		"unique (digest_issue_id, source_type, external_id, capture_hash)",
		"alter table public.digest_source_items enable row level security",
		`create policy "digest_source_items_no_browser_access"`,
		"on public.digest_source_items (owner_id, source_type, external_id)",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}

func TestReadModelSnapshotMigrationDefinesPrecomputedPayloadStore(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202605310001_read_model_snapshots.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"create table if not exists public.read_model_snapshots",
		"owner_id uuid not null references public.user_profiles(id) on delete cascade",
		"schema_version text not null",
		"run_id text not null",
		"payload jsonb not null",
		"unique (owner_id, run_id)",
		"alter table public.read_model_snapshots enable row level security",
		`create policy "read_model_snapshots_no_browser_access"`,
		"on public.read_model_snapshots (owner_id, schema_version, published_at desc)",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}

func TestSupabaseAuthPublicOwnerMigrationDefinesWorkspaceMapping(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202606010001_supabase_auth_public_owner.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"add column if not exists handle text not null default ''",
		"add column if not exists auth_user_id uuid references auth.users(id) on delete set null",
		"add column if not exists is_public_owner boolean not null default false",
		"handle = 'abhijitmohanty'",
		"display_name = 'Abhijit Mohanty'",
		"is_public_owner = true",
		"public-playlist:PLH_SZ1gwLn4gpQyZICprtx3nKRYGPKE7r",
		"create unique index if not exists user_profiles_handle_uidx",
		"create unique index if not exists user_profiles_auth_user_id_uidx",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}

func TestExternalSupabaseAuthIdentityMigrationDropsLocalAuthForeignKey(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202606040001_external_supabase_auth_identity.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"r.relname = 'user_profiles'",
		"c.contype = 'f'",
		"FOREIGN KEY (auth_user_id) REFERENCES auth.users",
		"alter table public.user_profiles drop constraint",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}

func TestYouTubeTranscriptRequestLedgerMigrationDefinesOneRequestPerVideo(t *testing.T) {
	raw, err := os.ReadFile("../../../../supabase/migrations/202606040002_youtube_transcript_request_ledger.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)

	requiredFragments := []string{
		"create table if not exists public.youtube_transcript_requests",
		"primary key (owner_id, video_id)",
		"alter table public.youtube_transcript_requests enable row level security",
		`create policy "youtube_transcript_requests_no_browser_access"`,
		"from public.source_items",
		"jsonb_array_elements",
		"on conflict (owner_id, video_id) do nothing",
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing fragment %q", fragment)
		}
	}
}
