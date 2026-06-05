package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()
	logger := logging.NewForConfig("migrate", cfg)
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	migrationsDir := "../supabase/migrations"
	if len(os.Args) > 1 {
		migrationsDir = os.Args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
		create table if not exists public.schema_migrations (
			version text primary key,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		logger.Error("ensure migrations table", "error", err)
		os.Exit(1)
	}

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		logger.Error("read migrations", "error", err)
		os.Exit(1)
	}
	slices.Sort(files)

	for _, file := range files {
		if err := applyMigration(ctx, pool, file, logger); err != nil {
			logger.Error("apply migration", "file", filepath.Base(file), "error", err)
			os.Exit(1)
		}
	}
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, file string, logger *logging.Logger) error {
	version := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))

	var exists bool
	if err := pool.QueryRow(ctx, "select exists(select 1 from public.schema_migrations where version = $1)", version).Scan(&exists); err != nil {
		return fmt.Errorf("check migration state: %w", err)
	}
	if exists {
		logger.Info("migration already applied", "version", version)
		return nil
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(raw)); err != nil {
		return fmt.Errorf("execute sql: %w", err)
	}
	if _, err := tx.Exec(ctx, "insert into public.schema_migrations (version) values ($1)", version); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	logger.Info("migration applied", "version", version)
	return nil
}
