package postgres

import (
	"context"
	"strings"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
)

func (s *Store) SearchAskSources(ctx context.Context, ownerID string, model string, queryVector string, queryText string, limit int) ([]knowledge.AskSecondBrainSource, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := s.pool.Query(ctx, `
		with query as (
			select $2::vector as embedding, plainto_tsquery('english', $3) as tsq
		),
		insight_hits as (
			select
				coalesce(i.title, si.title, 'Insight') as title,
				'pgvector_insight' as source,
				coalesce(si.source_url, '') as source_url,
				concat_ws(' ', nullif(i.canonical_text, ''), nullif(i.raw_text, ''), nullif(i.mechanism, ''), nullif(i.embedding_text, '')) as excerpt,
				(
					1 - (ie.embedding <=> (select embedding from query)) +
					least(0.4, greatest(0, ts_rank_cd(to_tsvector('english', concat_ws(' ', i.title, i.raw_text, i.canonical_text, i.mechanism, i.embedding_text)), (select tsq from query))))
				)::double precision as score
			from insight_embeddings ie
			join insights i on i.id = ie.insight_id
			join source_items si on si.id = i.source_item_id
			where ie.owner_id = $1::uuid
			  and ie.model = $4
			order by ie.embedding <=> (select embedding from query)
			limit $5
		),
		chunk_hits as (
			select
				coalesce(si.title, se.label, 'Source evidence') as title,
				'pgvector_source' as source,
				coalesce(si.source_url, '') as source_url,
				coalesce(sc.content, se.label, '') as excerpt,
				(
					0.9 - (se.embedding <=> (select embedding from query)) +
					case when to_tsvector('english', coalesce(sc.content, '')) @@ (select tsq from query)
						then least(0.5, greatest(0, ts_rank_cd(to_tsvector('english', coalesce(sc.content, '')), (select tsq from query))))
						else 0
					end
				)::double precision as score
			from source_embeddings se
			join source_items si on si.id = se.source_item_id
			left join source_chunks sc on sc.id = se.source_chunk_id
			where se.owner_id = $1::uuid
			  and se.model = $4
			  and se.embedding_type in ('chunk', 'summary')
			order by se.embedding <=> (select embedding from query)
			limit $5
		)
		select title, source, source_url, excerpt, score from insight_hits
		union all
		select title, source, source_url, excerpt, score from chunk_hits
		order by score desc
		limit $5
	`, ownerID, queryVector, strings.TrimSpace(queryText), model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := []knowledge.AskSecondBrainSource{}
	for rows.Next() {
		var source knowledge.AskSecondBrainSource
		if err := rows.Scan(&source.Title, &source.Source, &source.SourceURL, &source.Excerpt, &source.Score); err != nil {
			return sources, err
		}
		source.Excerpt = truncateForAskSearch(source.Excerpt)
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func truncateForAskSearch(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 760 {
		return value
	}
	return value[:757] + "..."
}
