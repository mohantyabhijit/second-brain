package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Service) writeEvidenceArtifact(ctx context.Context, candidate sourceCandidate, captureHash string) SourceArtifact {
	start := time.Now()
	raw := []byte(candidate.body)
	checksumBytes := sha256.Sum256(raw)
	artifact := SourceArtifact{
		Source:      string(candidate.sourceType),
		SourceID:    candidate.externalID,
		Kind:        candidate.artifactKind,
		Bucket:      s.cfg.SupabaseStorageBucket,
		Path:        candidate.storagePath(),
		Checksum:    hex.EncodeToString(checksumBytes[:]),
		ContentType: candidate.contentType,
		ByteSize:    len(raw),
	}
	if artifact.Checksum == "" {
		artifact.Checksum = captureHash
	}
	if strings.TrimSpace(s.cfg.SupabaseURL) == "" || (strings.TrimSpace(s.cfg.SupabaseStorageKey) == "" && !s.cfg.OneCLIGateway) {
		artifact.Error = "Supabase Storage credentials missing; metadata recorded without object upload."
		s.logger.Warn(
			"evidence artifact upload skipped",
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
		s.logger.Warn("evidence artifact request build failed", "source", artifact.Source, "source_id", artifact.SourceID, "error", err)
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
		s.logger.Warn("evidence artifact upload failed", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return artifact
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		artifact.Error = fmt.Sprintf("Supabase Storage upload failed: %s", response.Status)
		s.logger.Warn("evidence artifact upload rejected", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "status", response.Status)
		return artifact
	}
	artifact.Stored = true
	s.logger.Info("evidence artifact stored", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "byte_size", artifact.ByteSize, "duration_ms", time.Since(start).Milliseconds())
	return artifact
}

func escapeObjectPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
