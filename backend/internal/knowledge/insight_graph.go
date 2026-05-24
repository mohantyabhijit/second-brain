package knowledge

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const (
	DefaultInsightGraphLimit = 180
	MaxInsightGraphLimit     = 500

	maxInsightGraphEdges      = 650
	maxInsightGraphNodeDegree = 7
)

type InsightGraphResponse struct {
	Nodes []InsightGraphNode `json:"nodes"`
	Edges []InsightGraphEdge `json:"edges"`
	Stats InsightGraphStats  `json:"stats"`
}

type InsightGraphNode struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	CanonicalInsight string   `json:"canonicalInsight,omitempty"`
	Mechanism        string   `json:"mechanism,omitempty"`
	Domain           string   `json:"domain,omitempty"`
	Type             string   `json:"type,omitempty"`
	Topics           []string `json:"topics"`
	Confidence       string   `json:"confidence,omitempty"`
	SourceURL        string   `json:"sourceUrl,omitempty"`
	Score            float64  `json:"score,omitempty"`
	CaptureIDs       []string `json:"-"`
}

type InsightGraphEdge struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	Target string  `json:"target"`
	Reason string  `json:"reason"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

type InsightGraphStats struct {
	TotalInsights    int `json:"totalInsights"`
	ReturnedInsights int `json:"returnedInsights"`
	ReturnedEdges    int `json:"returnedEdges"`
}

func NormalizeInsightGraphLimit(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultInsightGraphLimit, nil
	}
	limit, err := strconv.Atoi(trimmed)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if limit > MaxInsightGraphLimit {
		return MaxInsightGraphLimit, nil
	}
	return limit, nil
}

func (s *Service) ReadInsightGraph(ctx context.Context, limit int) (InsightGraphResponse, error) {
	if limit <= 0 {
		limit = DefaultInsightGraphLimit
	}
	if limit > MaxInsightGraphLimit {
		limit = MaxInsightGraphLimit
	}
	if strings.TrimSpace(s.cfg.Neo4jURI) == "" || strings.TrimSpace(s.cfg.Neo4jUsername) == "" || strings.TrimSpace(s.cfg.Neo4jPassword) == "" {
		if s.cache != nil {
			latest, err := s.cache.ReadLatest(ctx, s.cfg.OwnerID)
			if err == nil && latest != nil && len(latest.Insights) > 0 {
				s.logger.Info("read model cache hit", "surface", "insight-graph")
				return buildInsightGraphFromResult(latest, limit), nil
			}
			if err != nil && !errors.Is(err, ErrReadModelCacheMiss) {
				s.logger.Warn("read model cache fallback", "surface", "insight-graph", "error", err)
			}
		}
		latest, err := s.readLatestCanonical(ctx)
		if err == nil && latest != nil && len(latest.Insights) > 0 {
			return buildInsightGraphFromResult(latest, limit), nil
		}
		return InsightGraphResponse{}, fmt.Errorf("NEO4J_URI, NEO4J_USERNAME, and NEO4J_PASSWORD are required for the insight graph")
	}

	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	driver, err := neo4j.NewDriverWithContext(s.cfg.Neo4jURI, neo4j.BasicAuth(s.cfg.Neo4jUsername, s.cfg.Neo4jPassword, ""))
	if err != nil {
		return InsightGraphResponse{}, fmt.Errorf("connect neo4j: %w", err)
	}
	defer driver.Close(queryCtx)

	session := driver.NewSession(queryCtx, neo4j.SessionConfig{DatabaseName: s.cfg.Neo4jDatabase})
	defer session.Close(queryCtx)

	graph, err := neo4j.ExecuteRead(queryCtx, session, func(tx neo4j.ManagedTransaction) (InsightGraphResponse, error) {
		countResult, err := tx.Run(queryCtx, `match (insight:Insight) return count(insight) as total`, nil)
		if err != nil {
			return InsightGraphResponse{}, err
		}
		countRecord, err := countResult.Single(queryCtx)
		if err != nil {
			return InsightGraphResponse{}, err
		}

		rows, err := tx.Run(queryCtx, `
			match (capture:Capture)-[:YIELDED_INSIGHT]->(insight:Insight)
			optional match (source:Source)-[:HAS_CAPTURE]->(capture)
			with insight,
			     collect(distinct capture.id) as captureIds,
			     collect(distinct source.url) as sourceUrls
			return coalesce(insight.id, elementId(insight)) as id,
			       coalesce(insight.title, insight.canonical, insight.text, 'Insight') as label,
			       coalesce(insight.canonical, '') as canonicalInsight,
			       coalesce(insight.mechanism, '') as mechanism,
			       coalesce(insight.domain, '') as domain,
			       coalesce(insight.type, '') as type,
			       coalesce(insight.topics, []) as topics,
			       coalesce(insight.confidence, '') as confidence,
			       coalesce(sourceUrls[0], '') as sourceUrl,
			       coalesce(insight.importanceScore, insight.score, 0.0) as score,
			       captureIds as captureIds
			order by label asc
			limit $limit
		`, map[string]any{"limit": limit})
		if err != nil {
			return InsightGraphResponse{}, err
		}

		nodes := []InsightGraphNode{}
		for rows.Next(queryCtx) {
			record := rows.Record()
			id := strings.TrimSpace(stringRecordValue(record, "id"))
			if id == "" {
				continue
			}
			nodes = append(nodes, InsightGraphNode{
				ID:               id,
				Label:            fallback(stringRecordValue(record, "label"), "Insight"),
				CanonicalInsight: stringRecordValue(record, "canonicalInsight"),
				Mechanism:        stringRecordValue(record, "mechanism"),
				Domain:           stringRecordValue(record, "domain"),
				Type:             stringRecordValue(record, "type"),
				Topics:           normalizedGraphTopics(stringSliceRecordValue(record, "topics")),
				Confidence:       stringRecordValue(record, "confidence"),
				SourceURL:        stringRecordValue(record, "sourceUrl"),
				Score:            floatRecordValue(record, "score"),
				CaptureIDs:       normalizedGraphTopics(stringSliceRecordValue(record, "captureIds")),
			})
		}
		if err := rows.Err(); err != nil {
			return InsightGraphResponse{}, err
		}

		edges := buildInsightGraphEdges(nodes)
		return InsightGraphResponse{
			Nodes: nodes,
			Edges: edges,
			Stats: InsightGraphStats{
				TotalInsights:    int(floatRecordValue(countRecord, "total")),
				ReturnedInsights: len(nodes),
				ReturnedEdges:    len(edges),
			},
		}, nil
	})
	if err != nil {
		return InsightGraphResponse{}, fmt.Errorf("read insight graph: %w", err)
	}
	return graph, nil
}

func buildInsightGraphFromResult(result *Result, limit int) InsightGraphResponse {
	if result == nil {
		return InsightGraphResponse{
			Nodes: []InsightGraphNode{},
			Edges: []InsightGraphEdge{},
			Stats: InsightGraphStats{},
		}
	}
	NormalizeResultForReadModel(result)
	total := len(result.Insights)
	if limit <= 0 {
		limit = DefaultInsightGraphLimit
	}
	if limit > MaxInsightGraphLimit {
		limit = MaxInsightGraphLimit
	}
	insights := result.Insights
	if len(insights) > limit {
		insights = insights[:limit]
	}
	nodes := make([]InsightGraphNode, 0, len(insights))
	for index, insight := range insights {
		id := strings.TrimSpace(insight.ID)
		if id == "" {
			id = fmt.Sprintf("%s:%s:%d", insight.Source, insight.SourceID, index)
		}
		captureID := strings.TrimSpace(insight.Source + ":" + insight.SourceID)
		nodes = append(nodes, InsightGraphNode{
			ID:               id,
			Label:            fallback(insight.Title, "Insight"),
			CanonicalInsight: fallback(insight.CanonicalInsight, insight.Insight),
			Mechanism:        insight.Mechanism,
			Domain:           insight.Domain,
			Type:             insight.InsightType,
			Topics:           normalizedGraphTopics(insight.Topics),
			Confidence:       insight.Confidence,
			SourceURL:        insight.SourceURL,
			Score:            insight.ImportanceScore,
			CaptureIDs:       normalizedGraphTopics([]string{captureID}),
		})
	}
	edges := buildInsightGraphEdges(nodes)
	return InsightGraphResponse{
		Nodes: nodes,
		Edges: edges,
		Stats: InsightGraphStats{
			TotalInsights:    total,
			ReturnedInsights: len(nodes),
			ReturnedEdges:    len(edges),
		},
	}
}

func buildInsightGraphEdges(nodes []InsightGraphNode) []InsightGraphEdge {
	edges := make([]InsightGraphEdge, 0, min(len(nodes)*2, maxInsightGraphEdges))
	seen := map[string]bool{}
	for leftIndex := range nodes {
		for rightIndex := leftIndex + 1; rightIndex < len(nodes); rightIndex++ {
			reason, label, weight := insightGraphEdgeReason(nodes[leftIndex], nodes[rightIndex])
			if reason == "" {
				continue
			}
			source, target := orderedGraphPair(nodes[leftIndex].ID, nodes[rightIndex].ID)
			id := source + "::" + target
			if seen[id] {
				continue
			}
			seen[id] = true
			edges = append(edges, InsightGraphEdge{
				ID:     id,
				Source: source,
				Target: target,
				Reason: reason,
				Label:  label,
				Weight: weight,
			})
		}
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Weight != edges[j].Weight {
			return edges[i].Weight > edges[j].Weight
		}
		if edges[i].Label != edges[j].Label {
			return edges[i].Label < edges[j].Label
		}
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		return edges[i].Target < edges[j].Target
	})
	degrees := map[string]int{}
	filtered := make([]InsightGraphEdge, 0, min(len(edges), maxInsightGraphEdges))
	for _, edge := range edges {
		if len(filtered) >= maxInsightGraphEdges {
			break
		}
		if degrees[edge.Source] >= maxInsightGraphNodeDegree || degrees[edge.Target] >= maxInsightGraphNodeDegree {
			continue
		}
		filtered = append(filtered, edge)
		degrees[edge.Source]++
		degrees[edge.Target]++
	}
	return filtered
}

func insightGraphEdgeReason(left InsightGraphNode, right InsightGraphNode) (string, string, float64) {
	if shared := firstSharedGraphValue(left.CaptureIDs, right.CaptureIDs); shared != "" {
		return "same_capture", "Same source", 3
	}
	if shared := firstSharedGraphValue(left.Topics, right.Topics); shared != "" {
		return "shared_topic", shared, 2.4
	}
	if strings.TrimSpace(left.Domain) != "" && strings.EqualFold(strings.TrimSpace(left.Domain), strings.TrimSpace(right.Domain)) {
		return "shared_domain", strings.TrimSpace(left.Domain), 1.4
	}
	if strings.TrimSpace(left.Type) != "" && strings.EqualFold(strings.TrimSpace(left.Type), strings.TrimSpace(right.Type)) {
		return "shared_type", strings.TrimSpace(left.Type), 1
	}
	return "", "", 0
}

func firstSharedGraphValue(left []string, right []string) string {
	seen := map[string]string{}
	for _, value := range left {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		seen[strings.ToLower(trimmed)] = trimmed
	}
	matches := []string{}
	for _, value := range right {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if display, ok := seen[strings.ToLower(trimmed)]; ok {
			matches = append(matches, display)
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func normalizedGraphTopics(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func orderedGraphPair(left string, right string) (string, string) {
	if left <= right {
		return left, right
	}
	return right, left
}
