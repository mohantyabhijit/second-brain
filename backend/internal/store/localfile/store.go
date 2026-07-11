package localfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
)

type Store struct {
	path        string
	mu          sync.Mutex
	connections map[string][]knowledge.SourceProviderConnection
}

type transcriptRequestRecord struct {
	Status      string    `json:"status"`
	Detail      string    `json:"detail,omitempty"`
	AttemptedAt time.Time `json:"attemptedAt"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
}

func New(path string) *Store {
	return &Store{path: path, connections: map[string][]knowledge.SourceProviderConnection{}}
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

func (s *Store) ReadSourceMaterialStates(ctx context.Context, ownerID string, keys []knowledge.SourceMaterialKey) (map[string]knowledge.SourceMaterialState, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil || latest == nil {
		return map[string]knowledge.SourceMaterialState{}, err
	}
	wanted := map[string]knowledge.SourceMaterialKey{}
	for _, key := range keys {
		wanted[key.String()] = key
	}
	states := map[string]knowledge.SourceMaterialState{}
	for _, state := range knowledge.SourceMaterialStatesFromResult(latest) {
		cacheKey := state.Key().String()
		if _, ok := wanted[cacheKey]; ok {
			states[cacheKey] = state
		}
	}
	return states, nil
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

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o600); err != nil {
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
	if err := os.MkdirAll(filepath.Dir(feedbackPath), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(feedbackPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
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
	return errors.New("shared X OAuth tokens require DATABASE_URL/Postgres store")
}

func (s *Store) ReadSourceProviderConnections(ctx context.Context, ownerID string) ([]knowledge.SourceProviderConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	connections := append([]knowledge.SourceProviderConnection(nil), s.connections[ownerID]...)
	return connections, nil
}

func (s *Store) SaveYouTubePlaylistConnection(ctx context.Context, ownerID string, playlistID string) (*knowledge.SourceProviderConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	connection := knowledge.SourceProviderConnection{
		ID:                "local-youtube-" + playlistID,
		Provider:          "youtube",
		ProviderAccountID: playlistID,
		TokenStatus:       "active",
		LastValidatedAt:   &now,
		UpdatedAt:         now,
	}
	others := []knowledge.SourceProviderConnection{}
	for _, existing := range s.connections[ownerID] {
		if existing.Provider != "youtube" {
			others = append(others, existing)
		}
	}
	s.connections[ownerID] = append(others, connection)
	return &connection, nil
}

func (s *Store) ClaimYouTubeTranscriptRequest(ctx context.Context, ownerID string, videoID string, monthlyLimit int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return false, err
	}
	key, err := transcriptRequestKey(ownerID, videoID)
	if err != nil {
		return false, err
	}
	records, err := s.readTranscriptRequests()
	if err != nil {
		return false, err
	}
	if _, exists := records[key]; exists {
		return false, nil
	}
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	requestsThisMonth := 0
	for _, record := range records {
		if !record.AttemptedAt.Before(monthStart) {
			requestsThisMonth++
		}
	}
	if monthlyLimit <= 0 || requestsThisMonth >= monthlyLimit {
		return false, nil
	}
	records[key] = transcriptRequestRecord{
		Status:      "claimed",
		Detail:      "Supadata request claimed before provider call.",
		AttemptedAt: now,
	}
	if err := s.writeTranscriptRequests(records); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CompleteYouTubeTranscriptRequest(ctx context.Context, ownerID string, videoID string, status string, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := transcriptRequestKey(ownerID, videoID)
	if err != nil {
		return err
	}
	records, err := s.readTranscriptRequests()
	if err != nil {
		return err
	}
	record, exists := records[key]
	if !exists {
		return errors.New("youtube transcript request was not claimed")
	}
	record.Status = strings.TrimSpace(status)
	if record.Status == "" {
		record.Status = "completed"
	}
	record.Detail = detail
	record.CompletedAt = time.Now().UTC()
	records[key] = record
	return s.writeTranscriptRequests(records)
}

func transcriptRequestKey(ownerID string, videoID string) (string, error) {
	ownerID = strings.TrimSpace(ownerID)
	videoID = strings.TrimSpace(videoID)
	if ownerID == "" || videoID == "" {
		return "", errors.New("owner id and youtube video id are required")
	}
	return ownerID + ":" + videoID, nil
}

func (s *Store) readTranscriptRequests() (map[string]transcriptRequestRecord, error) {
	records := map[string]transcriptRequestRecord{}
	raw, err := os.ReadFile(s.path + ".youtube-transcript-requests.json")
	if errors.Is(err, os.ErrNotExist) {
		return records, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) writeTranscriptRequests(records map[string]transcriptRequestRecord) error {
	raw, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	path := s.path + ".youtube-transcript-requests.json"
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
