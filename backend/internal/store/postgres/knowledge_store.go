package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool       *pgxpool.Pool
	progressMu sync.RWMutex
	progress   func(done int, total int)
}

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
	if digest, err := s.readLatestDigest(ctx); err == nil && digest != nil {
		result.Digest = digest
	}
	return &result, nil
}

func (s *Store) readLatestDigest(ctx context.Context) (*knowledge.DigestIssue, error) {
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
			status
		from digest_issues
		order by updated_at desc, created_at desc
		limit 1
	`).Scan(&digest.ID, &digest.OwnerID, &digest.DigestDate, &digest.ScheduledFor, &digest.IdempotencyKey, &digest.Subject, &digest.BodyMarkdown, &digest.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &digest, nil
}

func (s *Store) ReadDigests(ctx context.Context, limit int) ([]knowledge.DigestIssue, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		select
			id::text,
			owner_id::text,
			digest_date,
			scheduled_for,
			idempotency_key,
			subject,
			body_markdown,
			status
		from digest_issues
		order by scheduled_for desc, updated_at desc, created_at desc
		limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	digests := []knowledge.DigestIssue{}
	for rows.Next() {
		var digest knowledge.DigestIssue
		if err := rows.Scan(&digest.ID, &digest.OwnerID, &digest.DigestDate, &digest.ScheduledFor, &digest.IdempotencyKey, &digest.Subject, &digest.BodyMarkdown, &digest.Status); err != nil {
			return nil, err
		}
		digests = append(digests, digest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return digests, nil
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
			left join source_captures sc on sc.id = ks.source_capture_id
			where si.source_type = $1
			  and si.external_id = $2
			  and ($3 = '' or coalesce(sc.capture_hash, ks.capture_hash) = $3)
			  and ks.prompt_version = $4
			  and ks.model = $5
			order by ks.generated_at desc
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
		tx.Rollback(ctx)
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
		"sourceType":  source.SourceType,
		"contentType": contentTypeForSource(source),
		"externalId":  source.ExternalID,
		"sourceUrl":   safeUTF8(source.SourceURL),
		"title":       safeUTF8(source.Title),
		"authorName":  safeUTF8(source.AuthorName),
		"username":    safeUTF8(source.Username),
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

func updateSynthesisSummaryObject(ctx context.Context, tx pgx.Tx, sourceCaptureID string, synthesis knowledge.SynthesisRecord, summaryObjectID string) error {
	_, err := tx.Exec(ctx, `
		update knowledge_syntheses
		set summary_object_id = $1
		where source_capture_id = $2
		  and prompt_version = $3
		  and model = $4
	`, summaryObjectID, sourceCaptureID, synthesis.PromptVersion, synthesis.Model)
	return err
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
			owner_id,
			digest_date,
			scheduled_for,
			idempotency_key,
			subject,
			body_markdown,
			status,
			generated_from_run_id
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (owner_id, idempotency_key) do update set
			subject = excluded.subject,
			body_markdown = excluded.body_markdown,
			status = excluded.status,
			generated_from_run_id = excluded.generated_from_run_id,
			updated_at = now()
		returning id
	`, ownerID, digest.DigestDate, digest.ScheduledFor, digest.IdempotencyKey, digest.Subject, digest.BodyMarkdown, digest.Status, nullableRunID(runID)).Scan(&digestID)
	if err != nil {
		return nil, err
	}
	digest.ID = digestID
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

func sourceKey(sourceType knowledge.SourceType, externalID string) string {
	return string(sourceType) + ":" + externalID
}

func ownerIDFromSources(sources []knowledge.ProcessedSource) string {
	for _, source := range sources {
		if source.OwnerID != "" {
			return source.OwnerID
		}
	}
	return "00000000-0000-0000-0000-000000000001"
}

func ownerIDForSource(source knowledge.ProcessedSource) string {
	if source.OwnerID != "" {
		return source.OwnerID
	}
	return "00000000-0000-0000-0000-000000000001"
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
