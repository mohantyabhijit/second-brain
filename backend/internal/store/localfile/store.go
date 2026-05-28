package localfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

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
		cacheKey := key.String()
		if _, ok := wanted[cacheKey]; !ok {
			key.CaptureHash = ""
			cacheKey = key.String()
			if _, ok := wanted[cacheKey]; !ok {
				continue
			}
		}
		sourceKey := summary.Source + ":" + summary.ID
		generatedAt := latest.GeneratedAt
		if summary.GeneratedAt != nil {
			generatedAt = *summary.GeneratedAt
		}
		cached[cacheKey] = knowledge.SynthesisRecord{
			SourceType:    key.SourceType,
			ExternalID:    key.ExternalID,
			CaptureHash:   summary.CaptureHash,
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

func (s *Store) ReadNewDigestSources(ctx context.Context, ownerID string, promptVersion string, model string) ([]knowledge.DigestSourceRef, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil || latest == nil {
		return []knowledge.DigestSourceRef{}, err
	}
	cutoff := time.Time{}
	if latest.Digest != nil && !latest.Digest.ScheduledFor.IsZero() {
		cutoff = latest.Digest.ScheduledFor
	}
	refsByKey := map[string]knowledge.DigestSourceRef{}
	addRef := func(source string, externalID string, title string, sourceURL string, captureHash string, generatedAt *time.Time) {
		if source == "" || externalID == "" {
			return
		}
		seenAt := latest.GeneratedAt
		if generatedAt != nil && !generatedAt.IsZero() {
			seenAt = generatedAt.UTC()
		}
		if !cutoff.IsZero() && !seenAt.After(cutoff) {
			return
		}
		key := source + ":" + externalID
		if _, exists := refsByKey[key]; exists {
			return
		}
		seenAtCopy := seenAt
		refsByKey[key] = knowledge.DigestSourceRef{
			Source:        source,
			ExternalID:    externalID,
			SourceURL:     sourceURL,
			Title:         title,
			CaptureHash:   captureHash,
			FirstSeenAt:   &seenAtCopy,
			SynthesizedAt: &seenAtCopy,
			DigestRole:    "input",
		}
	}
	for _, summary := range latest.Summaries {
		if promptVersion != "" && summary.PromptVersion != "" && summary.PromptVersion != promptVersion {
			continue
		}
		if model != "" && summary.Model != "" && summary.Model != model {
			continue
		}
		addRef(summary.Source, summary.ID, summary.Title, summary.SourceURL, summary.CaptureHash, summary.GeneratedAt)
	}
	for _, insight := range latest.Insights {
		addRef(insight.Source, insight.SourceID, insight.Title, insight.SourceURL, "", insight.GeneratedAt)
	}
	refs := make([]knowledge.DigestSourceRef, 0, len(refsByKey))
	for _, ref := range refsByKey {
		refs = append(refs, ref)
	}
	return refs, nil
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
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	feedbackPath := s.path + ".feedback.jsonl"
	if err := os.MkdirAll(filepath.Dir(feedbackPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(feedbackPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) ReadDigests(ctx context.Context, limit int) ([]knowledge.DigestIssue, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil || latest == nil || latest.Digest == nil {
		return []knowledge.DigestIssue{}, err
	}
	return []knowledge.DigestIssue{*latest.Digest}, nil
}

func (s *Store) ReadDigestIllustration(ctx context.Context, ownerID string, digestID string) (*knowledge.DigestIllustration, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil || latest == nil || latest.Digest == nil {
		return nil, err
	}
	digest := latest.Digest
	if digest.ID != digestID || digest.IllustrationBase64 == "" {
		return nil, nil
	}
	return &knowledge.DigestIllustration{
		ID:       digest.ID,
		Alt:      digest.IllustrationAlt,
		MimeType: digest.IllustrationMimeType,
		Base64:   digest.IllustrationBase64,
	}, nil
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

func (s *Store) ReadXTokens(ctx context.Context, ownerID string) (*knowledge.EncryptedXTokens, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *Store) SaveXTokens(ctx context.Context, tokens knowledge.EncryptedXTokens) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("shared X OAuth tokens require SUPABASE_DB_URL/Postgres store")
}
