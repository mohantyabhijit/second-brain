package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	if cfg.Neo4jURI == "" || cfg.Neo4jUsername == "" || cfg.Neo4jPassword == "" {
		logger.Error("NEO4J_URI, NEO4J_USERNAME, and NEO4J_PASSWORD are required for graph stat")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUsername, cfg.Neo4jPassword, ""))
	if err != nil {
		logger.Error("connect neo4j", "error", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: cfg.Neo4jDatabase})
	defer session.Close(ctx)

	_, err = neo4j.ExecuteRead(ctx, session, func(tx neo4j.ManagedTransaction) (bool, error) {
		result, err := tx.Run(ctx, `
			match (source:Source)
			optional match (source)-[:HAS_CAPTURE]->(capture:Capture)
			optional match (capture)-[:YIELDED_INSIGHT]->(insight:Insight)
			optional match (capture)-[:SUGGESTS_ACTION]->(action:ActionItem)
			return count(distinct source) as sources,
			       count(distinct capture) as captures,
			       count(distinct insight) as insights,
			       count(distinct action) as actions
		`, nil)
		if err != nil {
			return false, err
		}
		record, err := result.Single(ctx)
		if err != nil {
			return false, err
		}
		fmt.Printf(
			"sources=%d captures=%d insights=%d actions=%d\n",
			asInt64(record.AsMap()["sources"]),
			asInt64(record.AsMap()["captures"]),
			asInt64(record.AsMap()["insights"]),
			asInt64(record.AsMap()["actions"]),
		)
		return true, nil
	})
	if err != nil {
		logger.Error("read neo4j graph", "error", err)
		os.Exit(1)
	}
}

func asInt64(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
}
