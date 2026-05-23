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
