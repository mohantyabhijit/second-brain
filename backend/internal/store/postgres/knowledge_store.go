package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool       *pgxpool.Pool
	progressMu sync.RWMutex
	progress   func(done int, total int)
}

const latestRunOrderClause = `
		case
			when (jsonb_typeof(payload->'xBookmarks') = 'array' and jsonb_array_length(payload->'xBookmarks') > 0)
				or (jsonb_typeof(payload->'youtubeItems') = 'array' and jsonb_array_length(payload->'youtubeItems') > 0)
				or (jsonb_typeof(payload->'insights') = 'array' and jsonb_array_length(payload->'insights') > 0)
				or (jsonb_typeof(payload->'summaries') = 'array' and jsonb_array_length(payload->'summaries') > 0)
			then 0
			else 1
		end,
		generated_at desc
`

func New(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	// Supabase pooler deployments are safer with simple query mode because prepared
	// statement caches can interact badly with pooled backend connections.
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, config)
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

func (s *Store) SetRefreshProgressReporter(progress func(done int, total int)) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	s.progress = progress
}

func (s *Store) reportRefreshProgress(done int, total int) {
	s.progressMu.RLock()
	progress := s.progress
	s.progressMu.RUnlock()
	if progress != nil {
		progress(done, total)
	}
}

func (s *Store) ReadLatest(ctx context.Context) (*knowledge.Result, error) {
	return s.ReadLatestForOwner(ctx, config.DefaultOwnerID)
}

func (s *Store) ReadLatestForOwner(ctx context.Context, ownerID string) (*knowledge.Result, error) {
	ownerID = defaultOwnerID(ownerID)
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select payload
		from knowledge_runs
		where owner_id = $1
		order by `+latestRunOrderClause+`
		limit 1
	`, ownerID).Scan(&raw)
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
	if digest, err := s.readLatestDigestForOwner(ctx, ownerID); err == nil && digest != nil {
		result.Digest = digest
	}
	return &result, nil
}

func (s *Store) ReadLatestView(ctx context.Context, view string, limit int) (*knowledge.Result, error) {
	return s.ReadLatestViewForOwner(ctx, config.DefaultOwnerID, view, limit)
}

func (s *Store) ReadLatestViewForOwner(ctx context.Context, ownerID string, view string, limit int) (*knowledge.Result, error) {
	ownerID = defaultOwnerID(ownerID)
	field, limit, orderClause := latestViewField(view, limit)
	var raw []byte
	var err error
	if field == "" {
		err = s.pool.QueryRow(ctx, `
			with latest as (
				select payload
				from knowledge_runs
				where owner_id = $1
				order by `+latestRunOrderClause+`
				limit 1
			)
			select jsonb_build_object(
				'generatedAt', payload->'generatedAt',
				'sourceStatus', payload->'sourceStatus',
				'sourceCounts', source_counts,
				'validation', coalesce(payload->'validation', '[]'::jsonb),
				'blockers', coalesce(payload->'blockers', '[]'::jsonb)
			)
			from latest,
			lateral (
				select jsonb_build_object(
					'xBookmarks', jsonb_array_length(coalesce(payload->'xBookmarks', '[]'::jsonb)),
					'youtubeItems', jsonb_array_length(coalesce(payload->'youtubeItems', '[]'::jsonb))
				) source_counts
			) counts
		`, ownerID).Scan(&raw)
	} else {
		err = s.pool.QueryRow(ctx, fmt.Sprintf(`
			with latest as (
				select payload
				from knowledge_runs
				where owner_id = $1
				order by `+latestRunOrderClause+`
				limit 1
			)
			select jsonb_build_object(
				'generatedAt', payload->'generatedAt',
				'sourceStatus', payload->'sourceStatus',
				'sourceCounts', source_counts,
				'validation', coalesce(payload->'validation', '[]'::jsonb),
				'blockers', coalesce(payload->'blockers', '[]'::jsonb),
				'%[1]s', coalesce((
					select jsonb_agg(item.value order by %[2]s)
					from (
						select value, ordinality
						from jsonb_array_elements(coalesce(payload->'%[1]s', '[]'::jsonb)) with ordinality as item(value, ordinality)
						order by %[2]s
						limit $2
					) item
				), '[]'::jsonb)
			)
			from latest,
			lateral (
				select jsonb_build_object(
					'xBookmarks', jsonb_array_length(coalesce(payload->'xBookmarks', '[]'::jsonb)),
					'youtubeItems', jsonb_array_length(coalesce(payload->'youtubeItems', '[]'::jsonb))
				) source_counts
			) counts
		`, field, orderClause), ownerID, limit).Scan(&raw)
	}
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

func latestViewField(view string, limit int) (string, int, string) {
	normalizedView := strings.TrimSpace(view)
	limit = knowledge.NormalizeAppStateViewLimit(normalizedView, limit)
	switch normalizedView {
	case "insights":
		return "insights", limit, "item.ordinality"
	case "daily-newsletter":
		if limit > 8 {
			limit = 8
		}
		return "summaries", limit, "item.ordinality"
	case "original-x-posts", "original-x-bookmarks":
		return "xBookmarks", limit, "coalesce(item.value->>'createdAt', '') desc, item.ordinality"
	case "original-youtube-posts", "original-youtube-videos":
		return "youtubeItems", limit, "coalesce(item.value->>'publishedAt', '') desc, item.ordinality"
	default:
		return "", limit, "item.ordinality"
	}
}

func (s *Store) readLatestDigestForOwner(ctx context.Context, ownerID string) (*knowledge.DigestIssue, error) {
	ownerID = defaultOwnerID(ownerID)
	var digest knowledge.DigestIssue
	err := s.pool.QueryRow(ctx, `
		select
			id::text,
			owner_id::text,
			digest_date,
			scheduled_for,
			idempotency_key,
			subject,
			body_markdown,
			coalesce(illustration_prompt, ''),
			coalesce(illustration_alt, ''),
			coalesce(illustration_mime_type, ''),
			coalesce(illustration_base64, '') <> '',
			coalesce(illustration_model, ''),
			status
		from digest_issues
		where owner_id = $1
		order by updated_at desc, created_at desc
		limit 1
	`, ownerID).Scan(&digest.ID, &digest.OwnerID, &digest.DigestDate, &digest.ScheduledFor, &digest.IdempotencyKey, &digest.Subject, &digest.BodyMarkdown, &digest.IllustrationPrompt, &digest.IllustrationAlt, &digest.IllustrationMimeType, &digest.IllustrationAvailable, &digest.IllustrationModel, &digest.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &digest, nil
}

func (s *Store) ReadDigests(ctx context.Context, limit int) ([]knowledge.DigestIssue, error) {
	return s.ReadDigestsForOwner(ctx, config.DefaultOwnerID, limit)
}

func (s *Store) ReadDigestsForOwner(ctx context.Context, ownerID string, limit int) ([]knowledge.DigestIssue, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	ownerID = defaultOwnerID(ownerID)
	rows, err := s.pool.Query(ctx, `
		select
			id::text,
			owner_id::text,
			digest_date,
			scheduled_for,
			idempotency_key,
			subject,
			body_markdown,
			coalesce(illustration_prompt, ''),
			coalesce(illustration_alt, ''),
			coalesce(illustration_mime_type, ''),
			coalesce(illustration_base64, '') <> '',
			coalesce(illustration_model, ''),
			status
		from digest_issues
		where owner_id = $1
		order by scheduled_for desc, updated_at desc, created_at desc
		limit $2
	`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	digests := []knowledge.DigestIssue{}
	for rows.Next() {
		var digest knowledge.DigestIssue
		if err := rows.Scan(&digest.ID, &digest.OwnerID, &digest.DigestDate, &digest.ScheduledFor, &digest.IdempotencyKey, &digest.Subject, &digest.BodyMarkdown, &digest.IllustrationPrompt, &digest.IllustrationAlt, &digest.IllustrationMimeType, &digest.IllustrationAvailable, &digest.IllustrationModel, &digest.Status); err != nil {
			return nil, err
		}
		digests = append(digests, digest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return digests, nil
}

func (s *Store) ReadDigestIllustration(ctx context.Context, ownerID string, digestID string) (*knowledge.DigestIllustration, error) {
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	var illustration knowledge.DigestIllustration
	err := s.pool.QueryRow(ctx, `
		select
			id::text,
			coalesce(illustration_alt, ''),
			coalesce(illustration_mime_type, ''),
			coalesce(illustration_base64, '')
		from digest_issues
		where owner_id = $1
		  and id = $2
		  and coalesce(illustration_base64, '') <> ''
		limit 1
	`, ownerID, digestID).Scan(&illustration.ID, &illustration.Alt, &illustration.MimeType, &illustration.Base64)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &illustration, nil
}

func (s *Store) ReadNewDigestSources(ctx context.Context, ownerID string, promptVersion string, model string) ([]knowledge.DigestSourceRef, error) {
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	rows, err := s.pool.Query(ctx, `
		with latest_digest as (
			select
				coalesce(max(dd.attempted_at), di.scheduled_for, di.created_at) as cutoff
			from digest_issues di
			left join digest_deliveries dd on dd.digest_issue_id = di.id
			where di.owner_id = $1
			group by di.id, di.scheduled_for, di.created_at, di.updated_at
			order by coalesce(max(dd.attempted_at), di.scheduled_for, di.created_at) desc, di.updated_at desc
			limit 1
		),
		latest_synthesis as (
			select distinct on (ks.source_item_id)
				ks.id,
				ks.source_item_id,
				ks.source_capture_id,
				ks.capture_hash,
				ks.generated_at
			from knowledge_syntheses ks
			where ks.owner_id = $1
			  and ks.prompt_version = $2
			  and ks.model = $3
			order by ks.source_item_id, ks.generated_at desc
		)
		select
			si.id::text,
			coalesce(coalesce(sc.id, ls.source_capture_id)::text, ''),
			ls.id::text,
			si.source_type,
			si.external_id,
			si.source_url,
			si.title,
			coalesce(sc.capture_hash, ls.capture_hash, si.latest_capture_hash, si.capture_hash),
			si.first_seen_at,
			sc.captured_at,
			ls.generated_at
		from source_items si
		join latest_synthesis ls on ls.source_item_id = si.id
		left join source_captures sc on sc.id = ls.source_capture_id
		left join latest_digest ld on true
		where si.owner_id = $1
		  and (ld.cutoff is null or si.first_seen_at > ld.cutoff)
		  and not exists (
			select 1
			from digest_source_items dsi
			join digest_issues di on di.id = dsi.digest_issue_id
			where di.owner_id = $1
			  and dsi.source_item_id = si.id
		  )
		order by si.first_seen_at asc, si.source_type, si.external_id
	`, ownerID, promptVersion, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []knowledge.DigestSourceRef{}
	for rows.Next() {
		var ref knowledge.DigestSourceRef
		var firstSeenAt time.Time
		var capturedAt *time.Time
		var synthesizedAt time.Time
		if err := rows.Scan(
			&ref.SourceItemID,
			&ref.SourceCaptureID,
			&ref.KnowledgeSynthesisID,
			&ref.Source,
			&ref.ExternalID,
			&ref.SourceURL,
			&ref.Title,
			&ref.CaptureHash,
			&firstSeenAt,
			&capturedAt,
			&synthesizedAt,
		); err != nil {
			return nil, err
		}
		firstSeenAt = firstSeenAt.UTC()
		synthesizedAt = synthesizedAt.UTC()
		ref.FirstSeenAt = &firstSeenAt
		if capturedAt != nil {
			value := capturedAt.UTC()
			ref.CapturedAt = &value
		}
		ref.SynthesizedAt = &synthesizedAt
		ref.DigestRole = "input"
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func (s *Store) SaveLatest(ctx context.Context, result knowledge.Result) error {
	return s.SaveRun(ctx, result, nil)
}

func (s *Store) ReadCachedSyntheses(ctx context.Context, keys []knowledge.SynthesisCacheKey) (map[string]knowledge.SynthesisRecord, error) {
	return s.ReadCachedSynthesesForOwner(ctx, config.DefaultOwnerID, keys)
}

func (s *Store) ReadCachedSynthesesForOwner(ctx context.Context, ownerID string, keys []knowledge.SynthesisCacheKey) (map[string]knowledge.SynthesisRecord, error) {
	cached := map[string]knowledge.SynthesisRecord{}
	ownerID = defaultOwnerID(ownerID)
	for _, key := range keys {
		var summaryRaw, insightsRaw, actionsRaw []byte
		var generatedAt time.Time
		err := s.pool.QueryRow(ctx, `
			select ks.summary, ks.insights, ks.action_items, ks.generated_at
			from knowledge_syntheses ks
			join source_items si on si.id = ks.source_item_id
			left join source_captures sc on sc.id = ks.source_capture_id
			where si.source_type = $1
			  and si.external_id = $2
			  and ($3 = '' or coalesce(sc.capture_hash, ks.capture_hash) = $3)
			  and ks.prompt_version = $4
			  and ks.model = $5
			  and ks.owner_id = $6
			  and si.owner_id = $6
			order by ks.generated_at desc
			limit 1
		`, string(key.SourceType), key.ExternalID, key.CaptureHash, key.PromptVersion, key.Model, ownerID).Scan(&summaryRaw, &insightsRaw, &actionsRaw, &generatedAt)
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
			CaptureHash:   summary.CaptureHash,
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

func (s *Store) ReadSourceMaterialStates(ctx context.Context, ownerID string, keys []knowledge.SourceMaterialKey) (map[string]knowledge.SourceMaterialState, error) {
	states := map[string]knowledge.SourceMaterialState{}
	if len(keys) == 0 {
		return states, nil
	}
	if strings.TrimSpace(ownerID) == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	sourceTypes := make([]string, 0, len(keys))
	externalIDs := make([]string, 0, len(keys))
	promptVersions := make([]string, 0, len(keys))
	models := make([]string, 0, len(keys))
	for _, key := range keys {
		sourceTypes = append(sourceTypes, string(key.SourceType))
		externalIDs = append(externalIDs, key.ExternalID)
		promptVersions = append(promptVersions, key.PromptVersion)
		models = append(models, key.Model)
	}
	rows, err := s.pool.Query(ctx, `
		with requested as (
			select source_type, external_id, prompt_version, model
			from unnest($1::text[], $2::text[], $3::text[], $4::text[]) as request(source_type, external_id, prompt_version, model)
		),
		latest as (
			select distinct on (si.source_type, si.external_id, requested.prompt_version, requested.model)
				si.source_type,
				si.external_id,
				coalesce(ks.capture_hash, si.latest_capture_hash, si.capture_hash, '') as latest_capture_hash,
				requested.prompt_version,
				requested.model,
				coalesce(si.content_type, '') as content_type,
				coalesce(sc.metadata->>'artifactKind', source_object.kind, '') as artifact_kind,
				si.last_seen_at
			from requested
			join source_items si
			  on si.source_type = requested.source_type
			 and si.external_id = requested.external_id
			 and si.owner_id = $5
			join knowledge_syntheses ks
			  on ks.source_item_id = si.id
			 and ks.prompt_version = requested.prompt_version
			 and ks.model = requested.model
			left join source_captures sc on sc.id = ks.source_capture_id
			left join lateral (
				select so.kind
				from source_objects so
				where so.source_capture_id = ks.source_capture_id
				order by
					case when so.kind in ('transcript', 'article', 'tweet') then 0 else 1 end,
					so.captured_at desc
				limit 1
			) source_object on true
			order by si.source_type, si.external_id, requested.prompt_version, requested.model, ks.generated_at desc
		)
		select source_type, external_id, latest_capture_hash, prompt_version, model, content_type, artifact_kind, last_seen_at
		from latest
	`, sourceTypes, externalIDs, promptVersions, models, ownerID)
	if err != nil {
		return states, err
	}
	defer rows.Close()
	for rows.Next() {
		var state knowledge.SourceMaterialState
		if err := rows.Scan(&state.SourceType, &state.ExternalID, &state.LatestCaptureHash, &state.PromptVersion, &state.Model, &state.ContentType, &state.ArtifactKind, &state.LastSeenAt); err != nil {
			return states, err
		}
		state.Processed = true
		if state.ArtifactKind == "" {
			switch state.SourceType {
			case knowledge.SourceTypeX:
				state.ArtifactKind = state.ContentType
			case knowledge.SourceTypeYouTube:
				state.ArtifactKind = "metadata"
			default:
				state.ArtifactKind = "source"
			}
		}
		states[state.Key().String()] = state
	}
	if err := rows.Err(); err != nil {
		return states, err
	}
	return states, nil
}

func (s *Store) SaveRun(ctx context.Context, result knowledge.Result, sources []knowledge.ProcessedSource) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}

	sourceIDs := map[string]string{}
	allInsightIDs := map[string]string{}
	totalSources := len(sources)
	for index, source := range sources {
		sourceItemID, insightIDs, err := s.saveSourceCore(ctx, source)
		if err != nil {
			return err
		}
		sourceIDs[sourceKey(source.SourceType, source.ExternalID)] = sourceItemID
		for externalID, insightID := range insightIDs {
			allInsightIDs[externalID] = insightID
		}
		s.reportRefreshProgress(index+1, totalSources)
	}

	tx, err := s.beginWriteTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var runID string
	ownerID := ownerIDFromSources(sources)
	err = tx.QueryRow(ctx, `
		insert into knowledge_runs (owner_id, generated_at, payload)
		values ($1, $2, $3)
		returning id
	`, ownerID, result.GeneratedAt, string(raw)).Scan(&runID)
	if err != nil {
		return err
	}
	if err := saveThemes(ctx, tx, ownerID, runID, result.Themes, sourceIDs); err != nil {
		return err
	}
	if err := saveInsightClusters(ctx, tx, ownerID, result.InsightClusters, allInsightIDs); err != nil {
		return err
	}
	if err := saveConnections(ctx, tx, ownerID, runID, result.Connections, sourceIDs); err != nil {
		return err
	}
	if result.Digest != nil {
		if _, err := saveDigestTx(ctx, tx, ownerID, runID, *result.Digest); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) beginWriteTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		set local lock_timeout = '10s';
		set local statement_timeout = '45s';
		set local idle_in_transaction_session_timeout = '60s'
	`); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (s *Store) saveSourceCore(ctx context.Context, source knowledge.ProcessedSource) (string, map[string]string, error) {
	tx, err := s.beginWriteTx(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	sourceItemID, err := upsertSourceItem(ctx, tx, source)
	if err != nil {
		return "", nil, fmt.Errorf("upsert source item %s:%s: %w", source.SourceType, source.ExternalID, err)
	}
	sourceCaptureID, err := upsertSourceCapture(ctx, tx, sourceItemID, source)
	if err != nil {
		return "", nil, fmt.Errorf("upsert source capture %s:%s: %w", source.SourceType, source.ExternalID, err)
	}
	if source.Cached {
		if err := tx.Commit(ctx); err != nil {
			return "", nil, err
		}
		return sourceItemID, nil, nil
	}

	var summaryObjectID string
	if source.Artifact.Path != "" {
		if _, err := upsertSourceObject(ctx, tx, sourceItemID, sourceCaptureID, source, source.Artifact); err != nil {
			return "", nil, fmt.Errorf("upsert source object %s:%s:%s: %w", source.SourceType, source.ExternalID, source.Artifact.Kind, err)
		}
	}
	if source.SummaryArtifact.Path != "" {
		summaryObjectID, err = upsertSourceObject(ctx, tx, sourceItemID, sourceCaptureID, source, source.SummaryArtifact)
		if err != nil {
			return "", nil, fmt.Errorf("upsert summary object %s:%s: %w", source.SourceType, source.ExternalID, err)
		}
	}
	chunkIDs, err := upsertSourceChunks(ctx, tx, sourceItemID, sourceCaptureID, source)
	if err != nil {
		return "", nil, fmt.Errorf("upsert source chunks %s:%s: %w", source.SourceType, source.ExternalID, err)
	}
	synthesisID, err := upsertSynthesis(ctx, tx, sourceItemID, sourceCaptureID, source, summaryObjectID)
	if err != nil {
		return "", nil, fmt.Errorf("upsert synthesis %s:%s: %w", source.SourceType, source.ExternalID, err)
	}
	insightIDs, err := upsertInsights(ctx, tx, sourceItemID, sourceCaptureID, synthesisID, chunkIDs, source)
	if err != nil {
		return "", nil, fmt.Errorf("upsert insights %s:%s: %w", source.SourceType, source.ExternalID, err)
	}
	if err := enqueueGraphSync(ctx, tx, sourceItemID, sourceCaptureID, source); err != nil {
		return "", nil, fmt.Errorf("enqueue graph sync %s:%s: %w", source.SourceType, source.ExternalID, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}

	s.saveSourceEmbeddingsBestEffort(ctx, sourceItemID, sourceCaptureID, chunkIDs, insightIDs, source)
	return sourceItemID, insightIDs, nil
}

func (s *Store) saveSourceEmbeddingsBestEffort(ctx context.Context, sourceItemID string, sourceCaptureID string, chunkIDs map[int]string, insightIDs map[string]string, source knowledge.ProcessedSource) {
	if len(source.Embeddings) == 0 {
		return
	}
	embeddingCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := s.beginWriteTx(embeddingCtx)
	if err != nil {
		return
	}
	defer tx.Rollback(embeddingCtx)
	if err := upsertEmbeddings(embeddingCtx, tx, sourceItemID, sourceCaptureID, chunkIDs, source); err != nil {
		return
	}
	if err := upsertInsightEmbeddings(embeddingCtx, tx, insightIDs, source); err != nil {
		return
	}
	_ = tx.Commit(embeddingCtx)
}

func upsertSourceItem(ctx context.Context, tx pgx.Tx, source knowledge.ProcessedSource) (string, error) {
	var id string
	publishedAt := parseOptionalTime(source.PublishedAt)
	ownerID := ownerIDForSource(source)
	err := tx.QueryRow(ctx, `
		insert into source_items (
			owner_id,
			source_type,
			content_type,
			external_id,
			source_url,
			title,
			author_name,
			username,
			published_at,
			capture_hash,
			latest_capture_hash,
			processing_state,
			last_seen_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'processed', now())
		on conflict (owner_id, source_type, external_id) do update set
			content_type = excluded.content_type,
			source_url = excluded.source_url,
			title = excluded.title,
			author_name = excluded.author_name,
			username = excluded.username,
			published_at = excluded.published_at,
			capture_hash = excluded.capture_hash,
			latest_capture_hash = excluded.latest_capture_hash,
			processing_state = excluded.processing_state,
			last_seen_at = now()
		returning id
	`, ownerID, string(source.SourceType), contentTypeForSource(source), source.ExternalID, safeUTF8(source.SourceURL), safeUTF8(source.Title), safeUTF8(source.AuthorName), safeUTF8(source.Username), publishedAt, source.CaptureHash, source.CaptureHash).Scan(&id)
	return id, err
}

func upsertSourceCapture(ctx context.Context, tx pgx.Tx, sourceItemID string, source knowledge.ProcessedSource) (string, error) {
	ownerID := ownerIDForSource(source)
	metadata, err := json.Marshal(map[string]any{
		"sourceType":   source.SourceType,
		"contentType":  contentTypeForSource(source),
		"artifactKind": sourceArtifactKind(source),
		"externalId":   source.ExternalID,
		"sourceUrl":    safeUTF8(source.SourceURL),
		"title":        safeUTF8(source.Title),
		"authorName":   safeUTF8(source.AuthorName),
		"username":     safeUTF8(source.Username),
	})
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		insert into source_captures (
			owner_id,
			source_item_id,
			capture_hash,
			metadata
		)
		values ($1, $2, $3, $4)
		on conflict (source_item_id, capture_hash) do update set
			metadata = source_captures.metadata
		returning id
	`, ownerID, sourceItemID, source.CaptureHash, string(metadata)).Scan(&id)
	return id, err
}

func upsertSourceObject(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, source knowledge.ProcessedSource, artifact knowledge.SourceArtifact) (string, error) {
	ownerID := ownerIDForSource(source)
	var id string
	err := tx.QueryRow(ctx, `
		insert into source_objects (
			owner_id,
			source_item_id,
			source_capture_id,
			kind,
			bucket,
			path,
			checksum,
			content_type,
			byte_size
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict (bucket, path) do update set
			owner_id = excluded.owner_id,
			source_item_id = excluded.source_item_id,
			source_capture_id = excluded.source_capture_id,
			kind = excluded.kind,
			checksum = excluded.checksum,
			bucket = excluded.bucket,
			path = excluded.path,
			content_type = excluded.content_type,
			byte_size = excluded.byte_size,
			captured_at = now()
		returning id
	`, ownerID, sourceItemID, sourceCaptureID, artifact.Kind, artifact.Bucket, artifact.Path, artifact.Checksum, artifact.ContentType, artifact.ByteSize).Scan(&id)
	return id, err
}

func upsertSynthesis(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, source knowledge.ProcessedSource, summaryObjectID string) (string, error) {
	synthesis := source.Synthesis
	ownerID := ownerIDForSource(source)
	summaryRaw, err := json.Marshal(synthesis.Summary)
	if err != nil {
		return "", err
	}
	insightsRaw, err := json.Marshal(synthesis.Insights)
	if err != nil {
		return "", err
	}
	actionsRaw, err := json.Marshal(synthesis.ActionItems)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		insert into knowledge_syntheses (
			owner_id,
			source_item_id,
			source_capture_id,
			capture_hash,
			prompt_version,
			model,
			summary,
			insights,
			action_items,
			summary_object_id,
			generated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		on conflict (source_capture_id, prompt_version, model) do update set
			owner_id = excluded.owner_id,
			source_item_id = excluded.source_item_id,
			capture_hash = excluded.capture_hash,
			summary = excluded.summary,
			insights = excluded.insights,
			action_items = excluded.action_items,
			summary_object_id = excluded.summary_object_id,
			generated_at = excluded.generated_at
		returning id
	`, ownerID, sourceItemID, sourceCaptureID, synthesis.CaptureHash, synthesis.PromptVersion, synthesis.Model, safeUTF8(string(summaryRaw)), safeUTF8(string(insightsRaw)), safeUTF8(string(actionsRaw)), nullableUUID(summaryObjectID), synthesis.GeneratedAt).Scan(&id)
	return id, err
}

func upsertInsights(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, synthesisID string, chunkIDs map[int]string, source knowledge.ProcessedSource) (map[string]string, error) {
	ownerID := ownerIDForSource(source)
	insightIDs := map[string]string{}
	for _, insight := range source.Synthesis.Insights {
		var id string
		err := tx.QueryRow(ctx, `
			insert into insights (
				owner_id,
				source_item_id,
				source_capture_id,
				knowledge_synthesis_id,
				external_insight_id,
				title,
				raw_text,
				canonical_text,
				abstract_text,
				practical_text,
				mechanism,
				insight_type,
				domain,
				topics,
				entities,
				confidence,
				explicit_or_inferred,
				importance_score,
				novelty_score,
				actionability_score,
				embedding_text,
				generated_at
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
			on conflict (source_capture_id, external_insight_id) do update set
				owner_id = excluded.owner_id,
				source_item_id = excluded.source_item_id,
				knowledge_synthesis_id = excluded.knowledge_synthesis_id,
				title = excluded.title,
				raw_text = excluded.raw_text,
				canonical_text = excluded.canonical_text,
				abstract_text = excluded.abstract_text,
				practical_text = excluded.practical_text,
				mechanism = excluded.mechanism,
				insight_type = excluded.insight_type,
				domain = excluded.domain,
				topics = excluded.topics,
				entities = excluded.entities,
				confidence = excluded.confidence,
				explicit_or_inferred = excluded.explicit_or_inferred,
				importance_score = excluded.importance_score,
				novelty_score = excluded.novelty_score,
				actionability_score = excluded.actionability_score,
				embedding_text = excluded.embedding_text,
				generated_at = excluded.generated_at,
				updated_at = now()
			returning id
		`, ownerID, sourceItemID, sourceCaptureID, synthesisID, insight.ID, safeUTF8(insight.Title), safeUTF8(fallbackText(insight.RawInsight, insight.Insight)), safeUTF8(fallbackText(insight.CanonicalInsight, insight.Insight)), safeUTF8(insight.AbstractInsight), safeUTF8(insight.PracticalText), safeUTF8(insight.Mechanism), safeUTF8(insight.InsightType), safeUTF8(insight.Domain), insight.Topics, insight.Entities, insight.Confidence, insight.ExplicitOrInferred, insight.ImportanceScore, insight.NoveltyScore, insight.ActionabilityScore, safeUTF8(insight.EmbeddingText), insight.GeneratedAt).Scan(&id)
		if err != nil {
			return insightIDs, err
		}
		insightIDs[insight.ID] = id
		refs := insight.EvidenceRefs
		if len(refs) == 0 && insight.Evidence != "" {
			refs = []knowledge.InsightEvidenceRef{{Quote: insight.Evidence}}
		}
		for index, ref := range refs {
			var chunkID any
			if ref.ChunkIndex != nil {
				if id, ok := chunkIDs[*ref.ChunkIndex]; ok {
					chunkID = id
				}
			}
			if _, err := tx.Exec(ctx, `
				insert into insight_evidence (
					insight_id,
					source_item_id,
					source_capture_id,
					source_chunk_id,
					evidence_index,
					evidence_text
				)
				values ($1, $2, $3, $4, $5, $6)
				on conflict (insight_id, evidence_index) do update set
					source_item_id = excluded.source_item_id,
					source_capture_id = excluded.source_capture_id,
					source_chunk_id = excluded.source_chunk_id,
					evidence_text = excluded.evidence_text
			`, id, sourceItemID, sourceCaptureID, chunkID, index, safeUTF8(ref.Quote)); err != nil {
				return insightIDs, err
			}
		}
	}
	return insightIDs, nil
}

func upsertSourceChunks(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, source knowledge.ProcessedSource) (map[int]string, error) {
	chunkIDs := map[int]string{}
	ownerID := ownerIDForSource(source)
	for _, chunk := range source.Chunks {
		var id string
		err := tx.QueryRow(ctx, `
			insert into source_chunks (
				owner_id,
				source_item_id,
				source_capture_id,
				chunk_index,
				content,
				token_estimate,
				checksum
			)
			values ($1, $2, $3, $4, $5, $6, $7)
			on conflict (source_capture_id, chunk_index, checksum) do update set
				owner_id = excluded.owner_id,
				source_item_id = excluded.source_item_id,
				content = excluded.content,
				token_estimate = excluded.token_estimate
			returning id
		`, ownerID, sourceItemID, sourceCaptureID, chunk.Index, safeUTF8(chunk.Content), chunk.TokenEstimate, chunk.Checksum).Scan(&id)
		if err != nil {
			return chunkIDs, err
		}
		chunkIDs[chunk.Index] = id
	}
	return chunkIDs, nil
}

func upsertEmbeddings(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, chunkIDs map[int]string, source knowledge.ProcessedSource) error {
	ownerID := ownerIDForSource(source)
	for _, embedding := range source.Embeddings {
		if embedding.Type == "insight" || embedding.Type == "entity" {
			continue
		}
		var chunkID any
		if embedding.ChunkIndex != nil {
			if id, ok := chunkIDs[*embedding.ChunkIndex]; ok {
				chunkID = id
			}
		}
		_, err := tx.Exec(ctx, `
			insert into source_embeddings (
				owner_id,
				source_item_id,
				source_capture_id,
				source_chunk_id,
				embedding_type,
				embedding_key,
				label,
				model,
				dimensions,
				embedding
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::vector)
			on conflict (owner_id, embedding_key, model) do update set
				source_item_id = excluded.source_item_id,
				source_capture_id = excluded.source_capture_id,
				source_chunk_id = excluded.source_chunk_id,
				dimensions = excluded.dimensions,
				embedding = excluded.embedding
		`, ownerID, sourceItemID, sourceCaptureID, chunkID, embedding.Type, embeddingKey(source, embedding), embedding.Label, embedding.Model, embedding.Dimensions, embedding.Vector)
		if err != nil {
			return err
		}
	}
	return nil
}

func upsertInsightEmbeddings(ctx context.Context, tx pgx.Tx, insightIDs map[string]string, source knowledge.ProcessedSource) error {
	ownerID := ownerIDForSource(source)
	for _, embedding := range source.Embeddings {
		if embedding.Type != "insight" {
			continue
		}
		insightID, ok := insightIDs[embedding.Label]
		if !ok || embedding.Vector == "" {
			continue
		}
		embeddingKey := string(source.SourceType) + ":" + source.ExternalID + ":" + source.CaptureHash + ":insight:" + embedding.Label
		_, err := tx.Exec(ctx, `
			insert into insight_embeddings (
				owner_id,
				insight_id,
				embedding_key,
				model,
				dimensions,
				embedding
			)
			values ($1, $2, $3, $4, $5, $6::vector)
			on conflict (owner_id, embedding_key, model) do update set
				insight_id = excluded.insight_id,
				dimensions = excluded.dimensions,
				embedding = excluded.embedding
		`, ownerID, insightID, embeddingKey, embedding.Model, embedding.Dimensions, embedding.Vector)
		if err != nil {
			return err
		}
	}
	return nil
}

func embeddingKey(source knowledge.ProcessedSource, embedding knowledge.EmbeddingRecord) string {
	key := string(source.SourceType) + ":" + source.ExternalID + ":" + source.CaptureHash + ":" + embedding.Type + ":" + embedding.Label
	if embedding.ChunkIndex != nil {
		key += fmt.Sprintf(":%d", *embedding.ChunkIndex)
	}
	return key
}

func enqueueGraphSync(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, source knowledge.ProcessedSource) error {
	payload := map[string]any{
		"sourceType":      source.SourceType,
		"externalId":      source.ExternalID,
		"sourceUrl":       source.SourceURL,
		"title":           source.Title,
		"sourceCaptureId": sourceCaptureID,
		"captureHash":     source.CaptureHash,
		"summary":         source.Synthesis.Summary,
		"insights":        source.Synthesis.Insights,
		"actionItems":     source.Synthesis.ActionItems,
		"entities":        source.Entities,
		"artifact":        source.Artifact,
		"promptVersion":   source.Synthesis.PromptVersion,
		"model":           source.Synthesis.Model,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		insert into graph_sync_outbox (
			owner_id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload
		)
		values ($1, 'source_item', $2, 'source_item.processed', $3)
	`, ownerIDForSource(source), sourceItemID, string(raw))
	return err
}

func saveThemes(ctx context.Context, tx pgx.Tx, ownerID string, runID string, themes []knowledge.ThemeCluster, sourceIDs map[string]string) error {
	for _, theme := range themes {
		var themeID string
		err := tx.QueryRow(ctx, `
			insert into theme_clusters (owner_id, run_id, label, evidence, score)
			values ($1, $2, $3, $4, $5)
			returning id
		`, ownerID, runID, theme.Label, theme.Evidence, theme.Score).Scan(&themeID)
		if err != nil {
			return err
		}
		for _, source := range theme.Sources {
			sourceItemID, ok := sourceIDs[source]
			if !ok {
				continue
			}
			if _, err := tx.Exec(ctx, `
				insert into theme_cluster_items (theme_cluster_id, source_item_id, evidence)
				values ($1, $2, $3)
				on conflict (theme_cluster_id, source_item_id) do update set
					evidence = excluded.evidence
			`, themeID, sourceItemID, theme.Evidence); err != nil {
				return err
			}
		}
	}
	return nil
}

func saveInsightClusters(ctx context.Context, tx pgx.Tx, ownerID string, clusters []knowledge.InsightCluster, insightIDs map[string]string) error {
	for _, cluster := range clusters {
		var clusterID string
		err := tx.QueryRow(ctx, `
			insert into insight_clusters (
				owner_id,
				external_cluster_key,
				label,
				canonical_insight,
				cluster_summary,
				cluster_layer
			)
			values ($1, $2, $3, $4, $5, $6)
			on conflict (owner_id, cluster_layer, external_cluster_key) do update set
				label = excluded.label,
				canonical_insight = excluded.canonical_insight,
				cluster_summary = excluded.cluster_summary,
				updated_at = now()
			returning id
		`, ownerID, cluster.ID, cluster.Label, cluster.CanonicalInsight, cluster.Summary, fallbackText(cluster.Layer, "similar_insight")).Scan(&clusterID)
		if err != nil {
			return err
		}
		for _, externalInsightID := range cluster.InsightIDs {
			insightID, ok := insightIDs[externalInsightID]
			if !ok {
				continue
			}
			if _, err := tx.Exec(ctx, `
				insert into cluster_memberships (
					cluster_id,
					insight_id,
					similarity_score,
					membership_confidence
				)
				values ($1, $2, $3, 'medium')
				on conflict (cluster_id, insight_id) do update set
					similarity_score = excluded.similarity_score,
					membership_confidence = excluded.membership_confidence
			`, clusterID, insightID, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}

func saveConnections(ctx context.Context, tx pgx.Tx, ownerID string, runID string, connections []knowledge.SourceConnection, sourceIDs map[string]string) error {
	for _, connection := range connections {
		leftID, leftOK := sourceIDs[connection.LeftSourceID]
		rightID, rightOK := sourceIDs[connection.RightSourceID]
		if !leftOK || !rightOK {
			continue
		}
		_, err := tx.Exec(ctx, `
			insert into source_connections_evidence (
				owner_id,
				run_id,
				left_source_item_id,
				right_source_item_id,
				relationship,
				evidence,
				confidence
			)
			values ($1, $2, $3, $4, $5, $6, $7)
			on conflict (run_id, left_source_item_id, right_source_item_id, relationship) do update set
				evidence = excluded.evidence,
				confidence = excluded.confidence
		`, ownerID, runID, leftID, rightID, connection.Relationship, connection.Evidence, connection.Confidence)
		if err != nil {
			return err
		}
	}
	return nil
}

func saveDigestTx(ctx context.Context, tx pgx.Tx, ownerID string, runID string, digest knowledge.DigestIssue) (*knowledge.DigestIssue, error) {
	var digestID string
	err := tx.QueryRow(ctx, `
		insert into digest_issues (
			id,
			owner_id,
			digest_date,
			scheduled_for,
			idempotency_key,
			subject,
			body_markdown,
			illustration_prompt,
			illustration_alt,
			illustration_mime_type,
			illustration_base64,
			illustration_model,
			status,
			generated_from_run_id
		)
		values (coalesce(nullif($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		on conflict (owner_id, idempotency_key) do update set
			subject = excluded.subject,
			body_markdown = excluded.body_markdown,
			illustration_prompt = excluded.illustration_prompt,
			illustration_alt = excluded.illustration_alt,
			illustration_mime_type = excluded.illustration_mime_type,
			illustration_base64 = coalesce(nullif(excluded.illustration_base64, ''), digest_issues.illustration_base64),
			illustration_model = excluded.illustration_model,
			status = excluded.status,
			generated_from_run_id = excluded.generated_from_run_id,
			updated_at = now()
		returning id
	`, digest.ID, ownerID, digest.DigestDate, digest.ScheduledFor, digest.IdempotencyKey, digest.Subject, digest.BodyMarkdown, digest.IllustrationPrompt, digest.IllustrationAlt, digest.IllustrationMimeType, digest.IllustrationBase64, digest.IllustrationModel, digest.Status, nullableRunID(runID)).Scan(&digestID)
	if err != nil {
		return nil, err
	}
	digest.ID = digestID
	for _, source := range digest.SourceRefs {
		if strings.TrimSpace(source.Source) == "" || strings.TrimSpace(source.ExternalID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			insert into digest_source_items (
				owner_id,
				digest_issue_id,
				source_item_id,
				source_capture_id,
				knowledge_synthesis_id,
				source_type,
				external_id,
				capture_hash,
				source_url,
				title,
				first_seen_at,
				captured_at,
				synthesized_at,
				digest_role
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, coalesce(nullif($14, ''), 'input'))
			on conflict (digest_issue_id, source_type, external_id, capture_hash) do update set
				owner_id = excluded.owner_id,
				source_item_id = excluded.source_item_id,
				source_capture_id = excluded.source_capture_id,
				knowledge_synthesis_id = excluded.knowledge_synthesis_id,
				source_url = excluded.source_url,
				title = excluded.title,
				first_seen_at = excluded.first_seen_at,
				captured_at = excluded.captured_at,
				synthesized_at = excluded.synthesized_at,
				digest_role = excluded.digest_role
		`, ownerID, digestID, nullableUUID(source.SourceItemID), nullableUUID(source.SourceCaptureID), nullableUUID(source.KnowledgeSynthesisID), source.Source, source.ExternalID, source.CaptureHash, safeUTF8(source.SourceURL), safeUTF8(source.Title), source.FirstSeenAt, source.CapturedAt, source.SynthesizedAt, source.DigestRole); err != nil {
			return nil, err
		}
	}
	for _, delivery := range digest.Deliveries {
		attemptedAt := time.Now().UTC()
		if delivery.AttemptedAt != nil {
			attemptedAt = *delivery.AttemptedAt
		}
		if _, err := tx.Exec(ctx, `
			insert into digest_deliveries (
				owner_id,
				digest_issue_id,
				provider,
				recipient,
				status,
				provider_message_id,
				error,
				attempted_at
			)
			values ($1, $2, $3, $4, $5, $6, $7, $8)
		`, ownerID, digestID, delivery.Provider, delivery.Recipient, delivery.Status, delivery.ProviderMessageID, delivery.Error, attemptedAt); err != nil {
			return nil, err
		}
	}
	return &digest, nil
}

func (s *Store) SaveFeedback(ctx context.Context, event knowledge.FeedbackEvent) error {
	ownerID := event.OwnerID
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	_, err := s.pool.Exec(ctx, `
		insert into feedback_events (
			owner_id,
			target_type,
			target_id,
			signal,
			note,
			source_url
		)
		values ($1, $2, $3, $4, $5, $6)
	`, ownerID, event.TargetType, event.TargetID, event.Signal, event.Note, event.SourceURL)
	return err
}

func (s *Store) SaveDigest(ctx context.Context, digest knowledge.DigestIssue) (*knowledge.DigestIssue, error) {
	ownerID := digest.OwnerID
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	saved, err := saveDigestTx(ctx, tx, ownerID, "", digest)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return saved, nil
}

func (s *Store) ReadXTokens(ctx context.Context, ownerID string) (*knowledge.EncryptedXTokens, error) {
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	var tokens knowledge.EncryptedXTokens
	err := s.pool.QueryRow(ctx, `
		select
			owner_id::text,
			access_token_ciphertext,
			refresh_token_ciphertext,
			access_expires_at,
			scope,
			token_type,
			authenticated_x_user_id,
			authenticated_x_username,
			authenticated_x_name,
			updated_at
		from x_oauth_tokens
		where owner_id = $1
	`, ownerID).Scan(
		&tokens.OwnerID,
		&tokens.AccessTokenCiphertext,
		&tokens.RefreshTokenCiphertext,
		&tokens.AccessExpiresAt,
		&tokens.Scope,
		&tokens.TokenType,
		&tokens.AuthenticatedXUserID,
		&tokens.AuthenticatedXUsername,
		&tokens.AuthenticatedXName,
		&tokens.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tokens, nil
}

func (s *Store) SaveXTokens(ctx context.Context, tokens knowledge.EncryptedXTokens) error {
	ownerID := tokens.OwnerID
	if ownerID == "" {
		ownerID = "00000000-0000-0000-0000-000000000001"
	}
	updatedAt := tokens.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		insert into x_oauth_tokens (
			owner_id,
			access_token_ciphertext,
			refresh_token_ciphertext,
			access_expires_at,
			scope,
			token_type,
			authenticated_x_user_id,
			authenticated_x_username,
			authenticated_x_name,
			updated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		on conflict (owner_id) do update set
			access_token_ciphertext = excluded.access_token_ciphertext,
			refresh_token_ciphertext = excluded.refresh_token_ciphertext,
			access_expires_at = excluded.access_expires_at,
			scope = excluded.scope,
			token_type = excluded.token_type,
			authenticated_x_user_id = coalesce(nullif(excluded.authenticated_x_user_id, ''), x_oauth_tokens.authenticated_x_user_id),
			authenticated_x_username = coalesce(nullif(excluded.authenticated_x_username, ''), x_oauth_tokens.authenticated_x_username),
			authenticated_x_name = coalesce(nullif(excluded.authenticated_x_name, ''), x_oauth_tokens.authenticated_x_name),
			updated_at = excluded.updated_at
	`, ownerID, tokens.AccessTokenCiphertext, tokens.RefreshTokenCiphertext, tokens.AccessExpiresAt, tokens.Scope, tokens.TokenType, tokens.AuthenticatedXUserID, tokens.AuthenticatedXUsername, tokens.AuthenticatedXName, updatedAt)
	return err
}

func (s *Store) ResolveOwnerForAuthUser(ctx context.Context, authUserID string, email string, publicOwnerID string, publicOwnerEmail string) (string, error) {
	authUserID = strings.TrimSpace(authUserID)
	email = strings.TrimSpace(email)
	publicOwnerID = defaultOwnerID(publicOwnerID)
	publicOwnerEmail = strings.TrimSpace(strings.ToLower(publicOwnerEmail))
	if authUserID == "" {
		return "", fmt.Errorf("authenticated Supabase user id is required")
	}
	if publicOwnerEmail != "" && strings.EqualFold(email, publicOwnerEmail) {
		var ownerID string
		err := s.pool.QueryRow(ctx, `
			update user_profiles
			set auth_user_id = $1,
			    email = nullif($2, ''),
			    updated_at = now()
			where id = $3
			returning id::text
		`, authUserID, email, publicOwnerID).Scan(&ownerID)
		if err != nil {
			return "", err
		}
		return ownerID, nil
	}
	var ownerID string
	err := s.pool.QueryRow(ctx, `
		select id::text
		from user_profiles
		where auth_user_id = $1
		limit 1
	`, authUserID).Scan(&ownerID)
	if err == nil {
		return ownerID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	handle := "user-" + strings.ReplaceAll(authUserID, "-", "")
	if len(handle) > 17 {
		handle = handle[:17]
	}
	displayName := "Second Brain User"
	if email != "" {
		displayName = email
	}
	err = s.pool.QueryRow(ctx, `
		insert into user_profiles (id, email, auth_user_id, handle, display_name)
		values ($1, nullif($2, ''), $1, $3, $4)
		on conflict (id) do update set
			email = coalesce(nullif(excluded.email, ''), user_profiles.email),
			auth_user_id = excluded.auth_user_id,
			handle = coalesce(nullif(user_profiles.handle, ''), excluded.handle),
			display_name = coalesce(nullif(user_profiles.display_name, ''), excluded.display_name),
			updated_at = now()
		returning id::text
	`, authUserID, email, handle, displayName).Scan(&ownerID)
	if err != nil {
		return "", err
	}
	return ownerID, nil
}

func (s *Store) ReadSourceProviderConnections(ctx context.Context, ownerID string) ([]knowledge.SourceProviderConnection, error) {
	ownerID = defaultOwnerID(ownerID)
	rows, err := s.pool.Query(ctx, `
		select
			id::text,
			provider,
			provider_account_id,
			scopes,
			token_status,
			last_validated_at,
			updated_at
		from source_connections
		where owner_id = $1
		order by provider, updated_at desc
	`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	connections := []knowledge.SourceProviderConnection{}
	for rows.Next() {
		var connection knowledge.SourceProviderConnection
		if err := rows.Scan(
			&connection.ID,
			&connection.Provider,
			&connection.ProviderAccountID,
			&connection.Scopes,
			&connection.TokenStatus,
			&connection.LastValidatedAt,
			&connection.UpdatedAt,
		); err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return connections, nil
}

func (s *Store) SaveYouTubePlaylistConnection(ctx context.Context, ownerID string, playlistID string) (*knowledge.SourceProviderConnection, error) {
	ownerID = defaultOwnerID(ownerID)
	playlistID = strings.TrimSpace(playlistID)
	if playlistID == "" {
		return nil, fmt.Errorf("YouTube playlist id is required")
	}
	tx, err := s.beginWriteTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		delete from source_connections
		where owner_id = $1
		  and provider = 'youtube'
		  and provider_account_id <> $2
	`, ownerID, playlistID); err != nil {
		return nil, err
	}
	var connection knowledge.SourceProviderConnection
	err = tx.QueryRow(ctx, `
		insert into source_connections (
			owner_id,
			provider,
			provider_account_id,
			scopes,
			token_ref,
			token_status,
			last_validated_at,
			updated_at
		)
		values ($1, 'youtube', $2, '{}'::text[], $3, 'active', now(), now())
		on conflict (owner_id, provider, provider_account_id) do update set
			token_ref = excluded.token_ref,
			token_status = 'active',
			last_validated_at = now(),
			updated_at = now()
		returning id::text, provider, provider_account_id, scopes, token_status, last_validated_at, updated_at
	`, ownerID, playlistID, "public-playlist:"+playlistID).Scan(
		&connection.ID,
		&connection.Provider,
		&connection.ProviderAccountID,
		&connection.Scopes,
		&connection.TokenStatus,
		&connection.LastValidatedAt,
		&connection.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &connection, nil
}

func (s *Store) ClaimYouTubeTranscriptRequest(ctx context.Context, ownerID string, videoID string, monthlyLimit int) (bool, error) {
	ownerID = defaultOwnerID(ownerID)
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return false, fmt.Errorf("youtube video id is required")
	}
	tx, err := s.beginWriteTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtextextended($1, 0))`, ownerID+":supadata-transcript-budget"); err != nil {
		return false, err
	}
	var claimed bool
	err = tx.QueryRow(ctx, `
		insert into youtube_transcript_requests (
			owner_id,
			video_id,
			status,
			detail,
			attempted_at
		)
		select $1, $2, 'claimed', 'Supadata request claimed before provider call.', now()
		where $3 > 0
		  and (
		    select count(*)
		    from youtube_transcript_requests
		    where owner_id = $1
		      and attempted_at >= date_trunc('month', now())
		      and attempted_at < date_trunc('month', now()) + interval '1 month'
		  ) < $3
		on conflict (owner_id, video_id) do nothing
		returning true
	`, ownerID, videoID, monthlyLimit).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return claimed, nil
}

func (s *Store) CompleteYouTubeTranscriptRequest(ctx context.Context, ownerID string, videoID string, status string, detail string) error {
	ownerID = defaultOwnerID(ownerID)
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return fmt.Errorf("youtube video id is required")
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "completed"
	}
	_, err := s.pool.Exec(ctx, `
		update youtube_transcript_requests
		set status = $3,
		    detail = $4,
		    completed_at = now()
		where owner_id = $1
		  and video_id = $2
	`, ownerID, videoID, status, safeUTF8(detail))
	return err
}

func sourceKey(sourceType knowledge.SourceType, externalID string) string {
	return string(sourceType) + ":" + externalID
}

func defaultOwnerID(ownerID string) string {
	if strings.TrimSpace(ownerID) == "" {
		return config.DefaultOwnerID
	}
	return strings.TrimSpace(ownerID)
}

func ownerIDFromSources(sources []knowledge.ProcessedSource) string {
	for _, source := range sources {
		if source.OwnerID != "" {
			return source.OwnerID
		}
	}
	return config.DefaultOwnerID
}

func ownerIDForSource(source knowledge.ProcessedSource) string {
	if source.OwnerID != "" {
		return source.OwnerID
	}
	return config.DefaultOwnerID
}

func contentTypeForSource(source knowledge.ProcessedSource) string {
	if source.ContentType != "" {
		return source.ContentType
	}
	switch source.SourceType {
	case knowledge.SourceTypeYouTube:
		return "video"
	case knowledge.SourceTypeX:
		if source.Artifact.Kind == "article" {
			return "article"
		}
		return "post"
	default:
		return "document"
	}
}

func sourceArtifactKind(source knowledge.ProcessedSource) string {
	if source.Artifact.Kind != "" {
		return source.Artifact.Kind
	}
	switch source.SourceType {
	case knowledge.SourceTypeYouTube:
		return "transcript"
	case knowledge.SourceTypeX:
		if source.ContentType == "article" {
			return "article"
		}
		return "tweet"
	default:
		return "source"
	}
}

func nullableRunID(runID string) any {
	if runID == "" {
		return nil
	}
	return runID
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func fallbackText(value string, fallbackValue string) string {
	if value != "" {
		return value
	}
	return fallbackValue
}

func safeUTF8(value string) string {
	return strings.ToValidUTF8(value, "")
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
