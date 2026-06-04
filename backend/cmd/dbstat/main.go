package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("SUPABASE_DB_URL")
	}
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	printCounts(ctx, conn)

	rows, err := conn.Query(ctx, `
		select pid, state, wait_event_type, wait_event, (now() - query_start)::text as age, left(query, 220) as query
		from pg_stat_activity
		where datname = current_database()
		order by query_start desc
		limit 20
	`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer rows.Close()
	for rows.Next() {
		var pid int
		var state, waitType, waitEvent, age, query *string
		if err := rows.Scan(&pid, &state, &waitType, &waitEvent, &age, &query); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("pid=%d state=%s wait=%s/%s age=%s query=%q\n", pid, str(state), str(waitType), str(waitEvent), str(age), str(query))
	}
}

func printCounts(ctx context.Context, conn *pgx.Conn) {
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
			fmt.Printf("table=%s count_error=%q\n", table, err.Error())
			continue
		}
		fmt.Printf("table=%s count=%d\n", table, count)
	}
}

func str(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
