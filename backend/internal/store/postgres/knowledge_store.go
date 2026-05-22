package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) ReadLatest(ctx context.Context) (*knowledge.Result, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select payload
		from knowledge_runs
		order by generated_at desc
		limit 1
	`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var result knowledge.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) SaveLatest(ctx context.Context, result knowledge.Result) error {
	return s.SaveRun(ctx, result, nil)
}

func (s *Store) ReadCachedSyntheses(ctx context.Context, keys []knowledge.SynthesisCacheKey) (map[string]knowledge.SynthesisRecord, error) {
	cached := map[string]knowledge.SynthesisRecord{}
	for _, key := range keys {
		var summaryRaw, insightsRaw, actionsRaw []byte
		var generatedAt time.Time
		err := s.pool.QueryRow(ctx, `
			select ks.summary, ks.insights, ks.action_items, ks.generated_at
			from knowledge_syntheses ks
			join source_items si on si.id = ks.source_item_id
			where si.source_type = $1
			  and si.external_id = $2
			  and ks.capture_hash = $3
			  and ks.prompt_version = $4
			  and ks.model = $5
			limit 1
		`, string(key.SourceType), key.ExternalID, key.CaptureHash, key.PromptVersion, key.Model).Scan(&summaryRaw, &insightsRaw, &actionsRaw, &generatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return cached, err
		}
		var summary knowledge.Summary
		if err := json.Unmarshal(summaryRaw, &summary); err != nil {
			return cached, err
		}
		var insights []knowledge.Insight
		if err := json.Unmarshal(insightsRaw, &insights); err != nil {
			return cached, err
		}
		var actions []knowledge.ActionItem
		if err := json.Unmarshal(actionsRaw, &actions); err != nil {
			return cached, err
		}
		cached[key.String()] = knowledge.SynthesisRecord{
			SourceType:    key.SourceType,
			ExternalID:    key.ExternalID,
			CaptureHash:   key.CaptureHash,
			PromptVersion: key.PromptVersion,
			Model:         key.Model,
			Summary:       summary,
			Insights:      insights,
			ActionItems:   actions,
			GeneratedAt:   generatedAt,
		}
	}
	return cached, nil
}

func (s *Store) SaveRun(ctx context.Context, result knowledge.Result, sources []knowledge.ProcessedSource) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, source := range sources {
		sourceItemID, err := upsertSourceItem(ctx, tx, source)
		if err != nil {
			return err
		}
		if source.Artifact.Path != "" {
			if err := upsertSourceObject(ctx, tx, sourceItemID, source.Artifact); err != nil {
				return err
			}
		}
		if !source.Cached {
			if err := upsertSynthesis(ctx, tx, sourceItemID, source.Synthesis); err != nil {
				return err
			}
		}
	}

	_, err = tx.Exec(ctx, `
		insert into knowledge_runs (generated_at, payload)
		values ($1, $2)
	`, result.GeneratedAt, raw)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func upsertSourceItem(ctx context.Context, tx pgx.Tx, source knowledge.ProcessedSource) (string, error) {
	var id string
	publishedAt := parseOptionalTime(source.PublishedAt)
	err := tx.QueryRow(ctx, `
		insert into source_items (
			source_type,
			external_id,
			source_url,
			title,
			author_name,
			username,
			published_at,
			capture_hash,
			processing_state,
			last_seen_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, 'processed', now())
		on conflict (source_type, external_id) do update set
			source_url = excluded.source_url,
			title = excluded.title,
			author_name = excluded.author_name,
			username = excluded.username,
			published_at = excluded.published_at,
			capture_hash = excluded.capture_hash,
			processing_state = excluded.processing_state,
			last_seen_at = now()
		returning id
	`, string(source.SourceType), source.ExternalID, source.SourceURL, source.Title, source.AuthorName, source.Username, publishedAt, source.CaptureHash).Scan(&id)
	return id, err
}

func upsertSourceObject(ctx context.Context, tx pgx.Tx, sourceItemID string, artifact knowledge.SourceArtifact) error {
	_, err := tx.Exec(ctx, `
		insert into source_objects (
			source_item_id,
			kind,
			bucket,
			path,
			checksum,
			content_type,
			byte_size
		)
		values ($1, $2, $3, $4, $5, $6, $7)
		on conflict (source_item_id, kind, checksum) do update set
			bucket = excluded.bucket,
			path = excluded.path,
			content_type = excluded.content_type,
			byte_size = excluded.byte_size,
			captured_at = now()
	`, sourceItemID, artifact.Kind, artifact.Bucket, artifact.Path, artifact.Checksum, artifact.ContentType, artifact.ByteSize)
	return err
}

func upsertSynthesis(ctx context.Context, tx pgx.Tx, sourceItemID string, synthesis knowledge.SynthesisRecord) error {
	summaryRaw, err := json.Marshal(synthesis.Summary)
	if err != nil {
		return err
	}
	insightsRaw, err := json.Marshal(synthesis.Insights)
	if err != nil {
		return err
	}
	actionsRaw, err := json.Marshal(synthesis.ActionItems)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into knowledge_syntheses (
			source_item_id,
			capture_hash,
			prompt_version,
			model,
			summary,
			insights,
			action_items,
			generated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (source_item_id, capture_hash, prompt_version, model) do update set
			summary = excluded.summary,
			insights = excluded.insights,
			action_items = excluded.action_items,
			generated_at = excluded.generated_at
	`, sourceItemID, synthesis.CaptureHash, synthesis.PromptVersion, synthesis.Model, summaryRaw, insightsRaw, actionsRaw, synthesis.GeneratedAt)
	return err
}

func parseOptionalTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return &parsed
		}
	}
	return nil
}
