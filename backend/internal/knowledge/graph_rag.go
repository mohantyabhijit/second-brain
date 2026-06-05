package knowledge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func (s *Service) retrieveGraphSources(ctx context.Context, question string, limit int) []AskSecondBrainSource {
	if limit <= 0 {
		limit = 6
	}
	if strings.TrimSpace(s.cfg.Neo4jURI) == "" || strings.TrimSpace(s.cfg.Neo4jUsername) == "" || strings.TrimSpace(s.cfg.Neo4jPassword) == "" {
		return nil
	}
	terms := graphSearchTerms(question)
	if len(terms) == 0 {
		return nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	driver, err := neo4j.NewDriverWithContext(s.cfg.Neo4jURI, neo4j.BasicAuth(s.cfg.Neo4jUsername, s.cfg.Neo4jPassword, ""))
	if err != nil {
		s.log(ctx).Warn("neo4j graph rag connection failed", "error", err)
		return nil
	}
	defer driver.Close(queryCtx)

	session := driver.NewSession(queryCtx, neo4j.SessionConfig{DatabaseName: s.cfg.Neo4jDatabase})
	defer session.Close(queryCtx)

	results, err := neo4j.ExecuteRead(queryCtx, session, func(tx neo4j.ManagedTransaction) ([]AskSecondBrainSource, error) {
		rows, err := tx.Run(queryCtx, `
			match (source:Source)-[:HAS_CAPTURE]->(capture:Capture)-[:YIELDED_INSIGHT]->(insight:Insight)
			with source, capture, insight,
			     toLower(coalesce(insight.title, '') + ' ' + coalesce(insight.text, '') + ' ' +
			             coalesce(insight.canonical, '') + ' ' + coalesce(insight.mechanism, '') + ' ' +
			             coalesce(capture.summary, '') + ' ' + coalesce(source.title, '')) as haystack
			where any(term in $terms where haystack contains term)
			optional match (capture)-[:SUGGESTS_ACTION]->(action:ActionItem)
			return coalesce(source.title, source.url, 'Source') as sourceTitle,
			       coalesce(source.url, '') as sourceUrl,
			       coalesce(source.type, 'source') as sourceType,
			       coalesce(insight.title, 'Graph insight') as insightTitle,
			       coalesce(insight.text, '') as insightText,
			       coalesce(insight.canonical, '') as canonical,
			       coalesce(insight.mechanism, '') as mechanism,
			       coalesce(capture.summary, '') as summary,
			       collect(distinct coalesce(action.text, action.title))[0..3] as actions,
			       reduce(score = 0, term in $terms | score + case when haystack contains term then 1 else 0 end) as score
			order by score desc, insightTitle asc
			limit $limit
		`, map[string]any{
			"terms": terms,
			"limit": limit,
		})
		if err != nil {
			return nil, err
		}
		sources := []AskSecondBrainSource{}
		for rows.Next(queryCtx) {
			record := rows.Record()
			title := stringRecordValue(record, "insightTitle")
			sourceTitle := stringRecordValue(record, "sourceTitle")
			sourceType := stringRecordValue(record, "sourceType")
			excerptParts := []string{
				stringRecordValue(record, "insightText"),
				stringRecordValue(record, "canonical"),
				stringRecordValue(record, "mechanism"),
				stringRecordValue(record, "summary"),
			}
			if actions := stringSliceRecordValue(record, "actions"); len(actions) > 0 {
				excerptParts = append(excerptParts, "Related actions: "+strings.Join(actions, "; "))
			}
			score := floatRecordValue(record, "score")
			sources = append(sources, AskSecondBrainSource{
				ID:        fmt.Sprintf("G%d", len(sources)+1),
				Title:     fallback(title, sourceTitle),
				Source:    "neo4j_graph",
				SourceURL: stringRecordValue(record, "sourceUrl"),
				Excerpt:   truncateDigestText(strings.Join(nonEmptyStrings(excerptParts), " "), 760),
				Score:     score + askSourceScore(weightedTokens(question), strings.Join(excerptParts, " ")),
			})
			if sourceType != "" && sourceType != "source" {
				sources[len(sources)-1].Source = "neo4j_graph:" + sourceType
			}
		}
		return sources, rows.Err()
	})
	if err != nil {
		s.log(ctx).Warn("neo4j graph rag query failed", "error", err)
		return nil
	}
	return results
}

func graphSearchTerms(question string) []string {
	terms := []string{}
	seen := map[string]bool{}
	for _, term := range topKeywords(question, 12) {
		term = strings.ToLower(strings.TrimSpace(term))
		if len(term) < 3 || stopwords[term] || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

func stringRecordValue(record *neo4j.Record, key string) string {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprint(value)
}

func stringSliceRecordValue(record *neo4j.Record, key string) []string {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, item := range raw {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

func floatRecordValue(record *neo4j.Record, key string) float64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case float32:
		return float64(typed)
	default:
		return 0
	}
}

func nonEmptyStrings(values []string) []string {
	out := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
