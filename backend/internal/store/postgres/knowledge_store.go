package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			left join source_captures sc on sc.id = ks.source_capture_id
			where si.source_type = $1
			  and si.external_id = $2
			  and coalesce(sc.capture_hash, ks.capture_hash) = $3
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

	sourceIDs := map[string]string{}
	for _, source := range sources {
		sourceItemID, err := upsertSourceItem(ctx, tx, source)
		if err != nil {
			return fmt.Errorf("upsert source item %s:%s: %w", source.SourceType, source.ExternalID, err)
		}
		sourceCaptureID, err := upsertSourceCapture(ctx, tx, sourceItemID, source)
		if err != nil {
			return fmt.Errorf("upsert source capture %s:%s: %w", source.SourceType, source.ExternalID, err)
		}
		sourceIDs[sourceKey(source.SourceType, source.ExternalID)] = sourceItemID
		var summaryObjectID string
		if source.Artifact.Path != "" {
			if _, err := upsertSourceObject(ctx, tx, sourceItemID, sourceCaptureID, source, source.Artifact); err != nil {
				return fmt.Errorf("upsert source object %s:%s:%s: %w", source.SourceType, source.ExternalID, source.Artifact.Kind, err)
			}
		}
		if source.SummaryArtifact.Path != "" {
			var err error
			summaryObjectID, err = upsertSourceObject(ctx, tx, sourceItemID, sourceCaptureID, source, source.SummaryArtifact)
			if err != nil {
				return fmt.Errorf("upsert summary object %s:%s: %w", source.SourceType, source.ExternalID, err)
			}
		}
		chunkIDs, err := upsertSourceChunks(ctx, tx, sourceItemID, sourceCaptureID, source)
		if err != nil {
			return fmt.Errorf("upsert source chunks %s:%s: %w", source.SourceType, source.ExternalID, err)
		}
		if err := upsertEmbeddings(ctx, tx, sourceItemID, sourceCaptureID, chunkIDs, source); err != nil {
			return fmt.Errorf("upsert embeddings %s:%s: %w", source.SourceType, source.ExternalID, err)
		}
		if !source.Cached {
			if err := upsertSynthesis(ctx, tx, sourceItemID, sourceCaptureID, source, summaryObjectID); err != nil {
				return fmt.Errorf("upsert synthesis %s:%s: %w", source.SourceType, source.ExternalID, err)
			}
		}
		if source.Cached && summaryObjectID != "" {
			if err := updateSynthesisSummaryObject(ctx, tx, sourceCaptureID, source.Synthesis, summaryObjectID); err != nil {
				return fmt.Errorf("update synthesis summary object %s:%s: %w", source.SourceType, source.ExternalID, err)
			}
		}
		if err := enqueueGraphSync(ctx, tx, sourceItemID, sourceCaptureID, source); err != nil {
			return fmt.Errorf("enqueue graph sync %s:%s: %w", source.SourceType, source.ExternalID, err)
		}
	}

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
	`, ownerID, string(source.SourceType), contentTypeForSource(source), source.ExternalID, source.SourceURL, source.Title, source.AuthorName, source.Username, publishedAt, source.CaptureHash, source.CaptureHash).Scan(&id)
	return id, err
}

func upsertSourceCapture(ctx context.Context, tx pgx.Tx, sourceItemID string, source knowledge.ProcessedSource) (string, error) {
	ownerID := ownerIDForSource(source)
	metadata, err := json.Marshal(map[string]any{
		"sourceType":  source.SourceType,
		"contentType": contentTypeForSource(source),
		"externalId":  source.ExternalID,
		"sourceUrl":   source.SourceURL,
		"title":       source.Title,
		"authorName":  source.AuthorName,
		"username":    source.Username,
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
		on conflict (source_capture_id, kind, checksum) do update set
			owner_id = excluded.owner_id,
			source_item_id = excluded.source_item_id,
			bucket = excluded.bucket,
			path = excluded.path,
			content_type = excluded.content_type,
			byte_size = excluded.byte_size,
			captured_at = now()
		returning id
	`, ownerID, sourceItemID, sourceCaptureID, artifact.Kind, artifact.Bucket, artifact.Path, artifact.Checksum, artifact.ContentType, artifact.ByteSize).Scan(&id)
	return id, err
}

func upsertSynthesis(ctx context.Context, tx pgx.Tx, sourceItemID string, sourceCaptureID string, source knowledge.ProcessedSource, summaryObjectID string) error {
	synthesis := source.Synthesis
	ownerID := ownerIDForSource(source)
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
	`, ownerID, sourceItemID, sourceCaptureID, synthesis.CaptureHash, synthesis.PromptVersion, synthesis.Model, string(summaryRaw), string(insightsRaw), string(actionsRaw), nullableUUID(summaryObjectID), synthesis.GeneratedAt)
	return err
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
		`, ownerID, sourceItemID, sourceCaptureID, chunk.Index, chunk.Content, chunk.TokenEstimate, chunk.Checksum).Scan(&id)
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
