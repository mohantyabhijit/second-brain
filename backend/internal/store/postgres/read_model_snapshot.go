package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SaveReadModelSnapshot(ctx context.Context, ownerID string, state knowledge.AppState) error {
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into public.read_model_snapshots (
			owner_id,
			schema_version,
			run_id,
			etag,
			generated_at,
			published_at,
			payload
		)
		values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (owner_id, run_id) do update set
			schema_version = excluded.schema_version,
			etag = excluded.etag,
			generated_at = excluded.generated_at,
			published_at = excluded.published_at,
			payload = excluded.payload,
			updated_at = now()
	`, ownerID, state.Manifest.SchemaVersion, state.Manifest.RunID, state.Manifest.ETag, state.Manifest.GeneratedAt, state.Manifest.PublishedAt, string(raw))
	return err
}

func (s *Store) ReadLatestReadModelSnapshot(ctx context.Context, ownerID string) (*knowledge.AppState, error) {
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select payload
		from public.read_model_snapshots
		where owner_id = $1
		  and schema_version = $2
		order by published_at desc, updated_at desc
		limit 1
	`, ownerID, knowledge.AppStateSchemaVersion).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, knowledge.ErrReadModelCacheMiss
	}
	if err != nil {
		return nil, err
	}
	var state knowledge.AppState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
