package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/abhijitmohanty/second-brain/backend/internal/phaseone"
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

func (s *Store) ReadLatest(ctx context.Context) (*phaseone.Result, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		select payload
		from phase_one_runs
		order by generated_at desc
		limit 1
	`).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var result phaseone.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) SaveLatest(ctx context.Context, result phaseone.Result) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into phase_one_runs (generated_at, payload)
		values ($1, $2)
	`, result.GeneratedAt, raw)
	return err
}
