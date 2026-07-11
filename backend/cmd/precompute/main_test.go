package main

import (
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
)

func TestGraphInsightCountHandlesAbsentAndPopulatedGraphs(t *testing.T) {
	tests := []struct {
		name  string
		state *knowledge.AppState
		want  int
	}{
		{"nil state", nil, 0},
		{"nil graph", &knowledge.AppState{}, 0},
		{"populated graph", &knowledge.AppState{Graph: knowledge.AppStateGraph{InsightGraph: &knowledge.InsightGraphResponse{Stats: knowledge.InsightGraphStats{ReturnedInsights: 9}}}}, 9},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := graphInsightCount(test.state); got != test.want {
				t.Fatalf("graphInsightCount = %d, want %d", got, test.want)
			}
		})
	}
}
