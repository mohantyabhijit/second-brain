package postgres

import (
	"context"
	"encoding/json"
	"time"
)

type GraphSyncEvent struct {
	ID            string
	AggregateID   string
	AggregateType string
	EventType     string
	Payload       json.RawMessage
	Attempts      int
}

func (s *Store) ReadPendingGraphEvents(ctx context.Context, limit int) ([]GraphSyncEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		select id, aggregate_id, aggregate_type, event_type, payload, attempts
		from graph_sync_outbox
		where status = 'pending'
		  and available_at <= now()
		order by created_at asc
		limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []GraphSyncEvent{}
	for rows.Next() {
		var event GraphSyncEvent
		if err := rows.Scan(&event.ID, &event.AggregateID, &event.AggregateType, &event.EventType, &event.Payload, &event.Attempts); err != nil {
			return events, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) MarkGraphEventProcessed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `
		update graph_sync_outbox
		set status = 'processed',
		    processed_at = now()
		where id = $1
	`, id)
	return err
}

func (s *Store) MarkGraphEventFailed(ctx context.Context, id string, attempts int) error {
	nextAttempt := time.Now().UTC().Add(backoffForGraphAttempt(attempts + 1))
	_, err := s.pool.Exec(ctx, `
		update graph_sync_outbox
		set attempts = attempts + 1,
		    available_at = $2
		where id = $1
	`, id, nextAttempt)
	return err
}

func backoffForGraphAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Minute
	}
	if attempt > 6 {
		return time.Hour
	}
	return time.Duration(attempt*attempt) * time.Minute
}
