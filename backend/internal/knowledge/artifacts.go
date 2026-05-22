package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Service) writeEvidenceArtifact(ctx context.Context, candidate sourceCandidate, captureHash string) SourceArtifact {
	raw := []byte(candidate.body)
	artifact := SourceArtifact{
		Source:      string(candidate.sourceType),
		SourceID:    candidate.externalID,
		Kind:        candidate.artifactKind,
		Bucket:      s.cfg.SupabaseStorageBucket,
		Path:        candidate.storagePath(captureHash),
		ContentType: candidate.contentType,
	}
	return s.writeStorageArtifact(ctx, artifact, raw, "evidence artifact")
}

func (s *Service) writeSynthesisArtifact(ctx context.Context, candidate sourceCandidate, captureHash string, record SynthesisRecord) SourceArtifact {
	payload := map[string]any{
		"sourceType":    candidate.sourceType,
		"externalId":    candidate.externalID,
		"sourceUrl":     candidate.sourceURL,
		"title":         candidate.title,
		"captureHash":   captureHash,
		"promptVersion": record.PromptVersion,
		"model":         record.Model,
		"summary":       record.Summary,
		"insights":      record.Insights,
		"actionItems":   record.ActionItems,
		"generatedAt":   record.GeneratedAt,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	artifact := SourceArtifact{
		Source:      string(candidate.sourceType),
		SourceID:    candidate.externalID,
		Kind:        "summary_json",
		Bucket:      s.cfg.SupabaseStorageBucket,
		Path:        synthesisStoragePath(candidate, captureHash, record),
		ContentType: "application/json; charset=utf-8",
	}
	if err != nil {
		artifact.Error = err.Error()
		return artifact
	}
	return s.writeStorageArtifact(ctx, artifact, raw, "synthesis artifact")
}

func (s *Service) writeStorageArtifact(ctx context.Context, artifact SourceArtifact, raw []byte, label string) SourceArtifact {
	start := time.Now()
	checksumBytes := sha256.Sum256(raw)
	artifact.Checksum = hex.EncodeToString(checksumBytes[:])
	artifact.ByteSize = len(raw)
	if strings.TrimSpace(s.cfg.SupabaseURL) == "" || (strings.TrimSpace(s.cfg.SupabaseStorageKey) == "" && !s.cfg.OneCLIGateway) {
		artifact.Error = "Supabase Storage credentials missing; metadata recorded without object upload."
		s.logger.Warn(
			label+" upload skipped",
			"source", artifact.Source,
			"source_id", artifact.SourceID,
			"bucket", artifact.Bucket,
			"path", artifact.Path,
			"byte_size", artifact.ByteSize,
			"reason", artifact.Error,
		)
		return artifact
	}

	objectURL := strings.TrimRight(s.cfg.SupabaseURL, "/") + "/storage/v1/object/" + escapeObjectPath(artifact.Bucket) + "/" + escapeObjectPath(artifact.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, bytes.NewReader(raw))
	if err != nil {
		artifact.Error = err.Error()
		s.logger.Warn(label+" request build failed", "source", artifact.Source, "source_id", artifact.SourceID, "error", err)
		return artifact
	}
	if s.cfg.SupabaseStorageKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.cfg.SupabaseStorageKey)
		req.Header.Set("apikey", s.cfg.SupabaseStorageKey)
	}
	req.Header.Set("Content-Type", artifact.ContentType)
	req.Header.Set("x-upsert", "true")

	response, err := s.client.Do(req)
	if err != nil {
		artifact.Error = fmt.Sprintf("Supabase Storage upload failed: %v", err)
		s.logger.Warn(label+" upload failed", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return artifact
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		artifact.Error = fmt.Sprintf("Supabase Storage upload failed: %s", response.Status)
		s.logger.Warn(label+" upload rejected", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "status", response.Status)
		return artifact
	}
	artifact.Stored = true
	s.logger.Info(label+" stored", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "byte_size", artifact.ByteSize, "duration_ms", time.Since(start).Milliseconds())
	return artifact
}

func synthesisStoragePath(candidate sourceCandidate, captureHash string, record SynthesisRecord) string {
	return strings.Join([]string{
		"artifacts",
		string(candidate.sourceType),
		candidate.externalID,
		captureHash,
		storagePathSegment(record.PromptVersion),
		storagePathSegment(record.Model),
		"summary.json",
	}, "/")
}

func storagePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func escapeObjectPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
