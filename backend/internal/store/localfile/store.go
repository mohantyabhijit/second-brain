package localfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
)

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) ReadLatest(ctx context.Context) (*knowledge.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var result knowledge.Result
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) SaveLatest(ctx context.Context, result knowledge.Result) error {
	return s.SaveRun(ctx, result, nil)
}

func (s *Store) ReadCachedSyntheses(ctx context.Context, keys []knowledge.SynthesisCacheKey) (map[string]knowledge.SynthesisRecord, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil || latest == nil {
		return map[string]knowledge.SynthesisRecord{}, err
	}
	wanted := map[string]knowledge.SynthesisCacheKey{}
	for _, key := range keys {
		wanted[key.String()] = key
	}
	insightsBySource := map[string][]knowledge.Insight{}
	for _, insight := range latest.Insights {
		insightsBySource[insight.Source+":"+insight.SourceID] = append(insightsBySource[insight.Source+":"+insight.SourceID], insight)
	}
	actionsBySource := map[string][]knowledge.ActionItem{}
	for _, action := range latest.ActionItems {
		actionsBySource[action.Source+":"+action.SourceID] = append(actionsBySource[action.Source+":"+action.SourceID], action)
	}

	cached := map[string]knowledge.SynthesisRecord{}
	for _, summary := range latest.Summaries {
		key := knowledge.SynthesisCacheKey{
			SourceType:    knowledge.SourceType(summary.Source),
			ExternalID:    summary.ID,
			CaptureHash:   summary.CaptureHash,
			PromptVersion: summary.PromptVersion,
			Model:         summary.Model,
		}
		if _, ok := wanted[key.String()]; !ok {
			continue
		}
		sourceKey := summary.Source + ":" + summary.ID
		generatedAt := latest.GeneratedAt
		if summary.GeneratedAt != nil {
			generatedAt = *summary.GeneratedAt
		}
		cached[key.String()] = knowledge.SynthesisRecord{
			SourceType:    key.SourceType,
			ExternalID:    key.ExternalID,
			CaptureHash:   key.CaptureHash,
			PromptVersion: key.PromptVersion,
			Model:         key.Model,
			Summary:       summary,
			Insights:      insightsBySource[sourceKey],
			ActionItems:   actionsBySource[sourceKey],
			GeneratedAt:   generatedAt,
		}
	}
	return cached, nil
}

func (s *Store) SaveRun(ctx context.Context, result knowledge.Result, sources []knowledge.ProcessedSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, s.path)
}

func (s *Store) SaveFeedback(ctx context.Context, event knowledge.FeedbackEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Store) SaveDigest(ctx context.Context, digest knowledge.DigestIssue) (*knowledge.DigestIssue, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, nil
	}
	latest.Digest = &digest
	if err := s.SaveRun(ctx, *latest, nil); err != nil {
		return nil, err
	}
	return &digest, nil
}
