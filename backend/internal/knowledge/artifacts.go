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
)

func (s *Service) writeEvidenceArtifact(ctx context.Context, candidate sourceCandidate, captureHash string) SourceArtifact {
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
		return artifact
	}

	objectURL := strings.TrimRight(s.cfg.SupabaseURL, "/") + "/storage/v1/object/" + escapeObjectPath(artifact.Bucket) + "/" + escapeObjectPath(artifact.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, bytes.NewReader(raw))
	if err != nil {
		artifact.Error = err.Error()
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
		return artifact
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		artifact.Error = fmt.Sprintf("Supabase Storage upload failed: %s", response.Status)
		return artifact
	}
	artifact.Stored = true
	return artifact
}

func escapeObjectPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
