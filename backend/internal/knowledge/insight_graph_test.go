package knowledge

import (
	"context"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestNormalizeInsightGraphLimit(t *testing.T) {
	t.Run("defaults empty limit", func(t *testing.T) {
		limit, err := NormalizeInsightGraphLimit("")
		if err != nil {
			t.Fatalf("expected default limit without error: %v", err)
		}
		if limit != DefaultInsightGraphLimit {
			t.Fatalf("expected default limit %d, got %d", DefaultInsightGraphLimit, limit)
		}
	})

	t.Run("clamps to hard max", func(t *testing.T) {
		limit, err := NormalizeInsightGraphLimit("999")
		if err != nil {
			t.Fatalf("expected clamped limit without error: %v", err)
		}
		if limit != MaxInsightGraphLimit {
			t.Fatalf("expected max limit %d, got %d", MaxInsightGraphLimit, limit)
		}
	})

	t.Run("rejects invalid limits", func(t *testing.T) {
		for _, raw := range []string{"0", "-1", "abc"} {
			if _, err := NormalizeInsightGraphLimit(raw); err == nil {
				t.Fatalf("expected invalid limit %q to fail", raw)
			}
		}
	})
}

func TestBuildInsightGraphEdgesPrioritizesAndDedupes(t *testing.T) {
	nodes := []InsightGraphNode{
		{
			ID:         "insight-a",
			CaptureIDs: []string{"capture-1"},
			Topics:     []string{"AI", "teams"},
			Domain:     "organizations",
			Type:       "principle",
		},
		{
			ID:         "insight-b",
			CaptureIDs: []string{"capture-1"},
			Topics:     []string{"ai"},
			Domain:     "organizations",
			Type:       "principle",
		},
		{
			ID:         "insight-c",
			CaptureIDs: []string{"capture-2"},
			Topics:     []string{"AI", "teams"},
			Domain:     "strategy",
			Type:       "warning",
		},
		{
			ID:         "insight-d",
			CaptureIDs: []string{"capture-3"},
			Topics:     []string{"operations"},
			Domain:     "organizations",
			Type:       "warning",
		},
	}

	edges := buildInsightGraphEdges(nodes)
	if len(edges) != 6 {
		t.Fatalf("expected all six node pairs to be connected by strongest available reason, got %#v", edges)
	}

	edgeByID := map[string]InsightGraphEdge{}
	for _, edge := range edges {
		if edgeByID[edge.ID].ID != "" {
			t.Fatalf("duplicate edge id %q in %#v", edge.ID, edges)
		}
		edgeByID[edge.ID] = edge
	}
	if got := edgeByID["insight-a::insight-b"].Reason; got != "same_capture" {
		t.Fatalf("expected same capture to outrank shared topic/domain/type, got %q", got)
	}
	if got := edgeByID["insight-a::insight-c"].Reason; got != "shared_topic" {
		t.Fatalf("expected shared topic edge, got %q", got)
	}
	if got := edgeByID["insight-a::insight-d"].Reason; got != "shared_domain" {
		t.Fatalf("expected shared domain edge, got %q", got)
	}
	if got := edgeByID["insight-c::insight-d"].Reason; got != "shared_type" {
		t.Fatalf("expected shared type edge, got %q", got)
	}
}

func TestReadInsightGraphUsesPrecomputedReadModel(t *testing.T) {
	cache := &graphReadModelCache{
		graph: InsightGraphResponse{
			Nodes: []InsightGraphNode{{ID: "cached", Label: "Cached graph", Topics: []string{}}},
			Edges: []InsightGraphEdge{},
			Stats: InsightGraphStats{TotalInsights: 1, ReturnedInsights: 1, ReturnedEdges: 0},
		},
	}
	service := NewService(config.Config{
		OwnerID:       "owner-1",
		Neo4jURI:      "bolt://127.0.0.1:1",
		Neo4jUsername: "neo4j",
		Neo4jPassword: "password",
		Neo4jDatabase: "missing",
	}, &cacheOrderStore{}, nil)
	service.SetReadModelCache(cache)

	graph, err := service.ReadInsightGraph(context.Background(), 10)
	if err != nil {
		t.Fatalf("read precomputed graph: %v", err)
	}
	if got := graph.Nodes[0].ID; got != "cached" {
		t.Fatalf("expected precomputed graph node, got %q", got)
	}
	if cache.reads != 1 {
		t.Fatalf("expected one graph read-model lookup, got %d", cache.reads)
	}
}

type graphReadModelCache struct {
	cacheOrderReadModel
	graph InsightGraphResponse
	reads int
}

func (c *graphReadModelCache) ReadInsightGraph(context.Context, string, int) (InsightGraphResponse, error) {
	c.reads++
	return c.graph, nil
}
