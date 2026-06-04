package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/postgres"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type sourceGraphPayload struct {
	SourceType      string `json:"sourceType"`
	ExternalID      string `json:"externalId"`
	SourceURL       string `json:"sourceUrl"`
	Title           string `json:"title"`
	SourceCaptureID string `json:"sourceCaptureId"`
	CaptureHash     string `json:"captureHash"`
	PromptVersion   string `json:"promptVersion"`
	Model           string `json:"model"`
	Summary         struct {
		ID       string `json:"id"`
		Summary  string `json:"summary"`
		Decision string `json:"decision"`
		Quote    string `json:"quote"`
	} `json:"summary"`
	Insights []struct {
		ID               string   `json:"id"`
		Title            string   `json:"title"`
		Insight          string   `json:"insight"`
		CanonicalInsight string   `json:"canonicalInsight"`
		Mechanism        string   `json:"mechanism"`
		InsightType      string   `json:"insightType"`
		Domain           string   `json:"domain"`
		Topics           []string `json:"topics"`
		Confidence       string   `json:"confidence"`
	} `json:"insights"`
	ActionItems []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Action   string `json:"action"`
		Priority string `json:"priority"`
	} `json:"actionItems"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required for graph sync")
		os.Exit(1)
	}
	if cfg.Neo4jURI == "" || cfg.Neo4jUsername == "" || cfg.Neo4jPassword == "" {
		logger.Error("NEO4J_URI, NEO4J_USERNAME, and NEO4J_PASSWORD are required for graph sync")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUsername, cfg.Neo4jPassword, ""))
	if err != nil {
		logger.Error("connect neo4j", "error", err)
		os.Exit(1)
	}
	defer driver.Close(ctx)

	events, err := store.ReadPendingGraphEvents(ctx, cfg.GraphSyncBatchSize)
	if err != nil {
		logger.Error("read graph outbox", "error", err)
		os.Exit(1)
	}
	if len(events) == 0 {
		logger.Info("graph sync completed", "events", 0)
		return
	}

	session := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: cfg.Neo4jDatabase})
	defer session.Close(ctx)

	processed := 0
	failed := 0
	for _, event := range events {
		if err := syncEvent(ctx, session, event); err != nil {
			failed++
			logger.Warn("graph event sync failed", "event_id", event.ID, "error", err)
			if markErr := store.MarkGraphEventFailed(ctx, event.ID, event.Attempts); markErr != nil {
				logger.Warn("mark graph event failed", "event_id", event.ID, "error", markErr)
			}
			continue
		}
		if err := store.MarkGraphEventProcessed(ctx, event.ID); err != nil {
			failed++
			logger.Warn("mark graph event processed failed", "event_id", event.ID, "error", err)
			continue
		}
		processed++
	}
	logger.Info("graph sync completed", "events", len(events), "processed", processed, "failed", failed)
}

func syncEvent(ctx context.Context, session neo4j.SessionWithContext, event postgres.GraphSyncEvent) error {
	var payload sourceGraphPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := neo4j.ExecuteWrite(ctx, session, func(tx neo4j.ManagedTransaction) (bool, error) {
		_, err := tx.Run(ctx, `
			merge (source:Source {id: $source_id})
			set source.type = $source_type,
			    source.externalId = $external_id,
			    source.url = $source_url,
			    source.title = $title,
			    source.updatedAt = datetime()
			merge (capture:Capture {id: $capture_id})
			set capture.captureHash = $capture_hash,
			    capture.promptVersion = $prompt_version,
			    capture.model = $model,
			    capture.summary = $summary,
			    capture.decision = $decision,
			    capture.updatedAt = datetime()
			merge (source)-[:HAS_CAPTURE]->(capture)
		`, map[string]any{
			"source_id":      payload.SourceType + ":" + payload.ExternalID,
			"source_type":    payload.SourceType,
			"external_id":    payload.ExternalID,
			"source_url":     payload.SourceURL,
			"title":          payload.Title,
			"capture_id":     payload.SourceCaptureID,
			"capture_hash":   payload.CaptureHash,
			"prompt_version": payload.PromptVersion,
			"model":          payload.Model,
			"summary":        payload.Summary.Summary,
			"decision":       payload.Summary.Decision,
		})
		if err != nil {
			return false, err
		}
		for _, insight := range payload.Insights {
			if _, err := tx.Run(ctx, `
				match (capture:Capture {id: $capture_id})
				merge (insight:Insight {id: $insight_id})
				set insight.title = $title,
				    insight.text = $text,
				    insight.canonical = $canonical,
				    insight.mechanism = $mechanism,
				    insight.type = $type,
				    insight.domain = $domain,
				    insight.topics = $topics,
				    insight.confidence = $confidence,
				    insight.updatedAt = datetime()
				merge (capture)-[:YIELDED_INSIGHT]->(insight)
			`, map[string]any{
				"capture_id": payload.SourceCaptureID,
				"insight_id": insight.ID,
				"title":      insight.Title,
				"text":       insight.Insight,
				"canonical":  insight.CanonicalInsight,
				"mechanism":  insight.Mechanism,
				"type":       insight.InsightType,
				"domain":     insight.Domain,
				"topics":     insight.Topics,
				"confidence": insight.Confidence,
			}); err != nil {
				return false, err
			}
		}
		for _, action := range payload.ActionItems {
			if _, err := tx.Run(ctx, `
				match (capture:Capture {id: $capture_id})
				merge (action:ActionItem {id: $action_id})
				set action.title = $title,
				    action.text = $text,
				    action.priority = $priority,
				    action.updatedAt = datetime()
				merge (capture)-[:SUGGESTS_ACTION]->(action)
			`, map[string]any{
				"capture_id": payload.SourceCaptureID,
				"action_id":  action.ID,
				"title":      action.Title,
				"text":       action.Action,
				"priority":   action.Priority,
			}); err != nil {
				return false, err
			}
		}
		return true, nil
	})
	return err
}
