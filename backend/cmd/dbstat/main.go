package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	"github.com/jackc/pgx/v5"
)

func main() {
	cfg := config.Load()
	logger := logging.NewForConfig("dbstat", cfg)
	databaseURL := cfg.DatabaseURL
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		logger.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	printCounts(ctx, conn, logger)

	rows, err := conn.Query(ctx, `
		select pid, state, wait_event_type, wait_event, (now() - query_start)::text as age, left(query, 220) as query
		from pg_stat_activity
		where datname = current_database()
		order by query_start desc
		limit 20
	`)
	if err != nil {
		logger.Error("read postgres activity", "error", err)
		os.Exit(1)
	}
	defer rows.Close()
	for rows.Next() {
		var pid int
		var state, waitType, waitEvent, age, query *string
		if err := rows.Scan(&pid, &state, &waitType, &waitEvent, &age, &query); err != nil {
			logger.Error("scan postgres activity", "error", err)
			os.Exit(1)
		}
		logger.Info("postgres activity", "pid", pid, "state", str(state), "wait_type", str(waitType), "wait_event", str(waitEvent), "age", str(age), "query", str(query))
	}
}

func printCounts(ctx context.Context, conn *pgx.Conn, logger *logging.Logger) {
	tables := []string{
		"knowledge_runs",
		"source_items",
		"source_captures",
		"source_chunks",
		"knowledge_syntheses",
		"insights",
		"insight_embeddings",
		"source_embeddings",
		"insight_clusters",
		"graph_sync_outbox",
		"digest_issues",
	}
	for _, table := range tables {
		var count int64
		query := fmt.Sprintf("select count(*) from %s", pgx.Identifier{table}.Sanitize())
		if err := conn.QueryRow(ctx, query).Scan(&count); err != nil {
			logger.Warn("postgres table count failed", "table", table, "error", err)
			continue
		}
		logger.Info("postgres table count", "table", table, "count", count)
	}
}

func str(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
