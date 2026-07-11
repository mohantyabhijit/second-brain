package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) writeEvidenceArtifact(ctx context.Context, candidate sourceCandidate, captureHash string) SourceArtifact {
	raw := []byte(candidate.body)
	artifact := SourceArtifact{
		Source:      string(candidate.sourceType),
		SourceID:    candidate.externalID,
		Kind:        candidate.artifactKind,
		Bucket:      s.objectStorageBucket(),
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
		Bucket:      s.objectStorageBucket(),
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
	switch s.objectStorageBackend() {
	case "filesystem", "file", "local":
		return s.writeFilesystemStorageArtifact(ctx, artifact, raw, label, start)
	case "", "none":
		artifact.Error = "Object storage backend is not configured; metadata recorded without object upload."
	default:
		artifact.Error = fmt.Sprintf("Object storage backend %q is not supported; metadata recorded without object upload.", s.cfg.ObjectStorageBackend)
	}
	s.log(ctx).Warn(
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

func (s *Service) objectStorageBackend() string {
	backend := strings.ToLower(strings.TrimSpace(s.cfg.ObjectStorageBackend))
	if backend != "" {
		return backend
	}
	if strings.TrimSpace(s.cfg.ObjectStorageRoot) != "" {
		return "filesystem"
	}
	return "none"
}

func (s *Service) objectStorageBucket() string {
	if bucket := strings.TrimSpace(s.cfg.ObjectStorageBucket); bucket != "" {
		return bucket
	}
	return "sources"
}

func (s *Service) writeFilesystemStorageArtifact(ctx context.Context, artifact SourceArtifact, raw []byte, label string, start time.Time) SourceArtifact {
	if err := ctx.Err(); err != nil {
		artifact.Error = fmt.Sprintf("filesystem object storage upload failed: %v", err)
		return artifact
	}
	objectPath, err := filesystemObjectPath(s.cfg.ObjectStorageRoot, artifact.Bucket, artifact.Path)
	if err != nil {
		artifact.Error = fmt.Sprintf("filesystem object storage upload failed: %v", err)
		s.log(ctx).Warn(label+" upload skipped", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "byte_size", artifact.ByteSize, "reason", artifact.Error)
		return artifact
	}
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o750); err != nil {
		artifact.Error = fmt.Sprintf("filesystem object storage upload failed: %v", err)
		s.log(ctx).Warn(label+" upload failed", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return artifact
	}
	tmpPath := fmt.Sprintf("%s.tmp-%d", objectPath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, raw, 0o600); err != nil {
		artifact.Error = fmt.Sprintf("filesystem object storage upload failed: %v", err)
		s.log(ctx).Warn(label+" upload failed", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return artifact
	}
	if err := os.Rename(tmpPath, objectPath); err != nil {
		_ = os.Remove(tmpPath)
		artifact.Error = fmt.Sprintf("filesystem object storage upload failed: %v", err)
		s.log(ctx).Warn(label+" upload failed", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "duration_ms", time.Since(start).Milliseconds(), "error", err)
		return artifact
	}
	artifact.Stored = true
	s.log(ctx).Info(label+" stored", "backend", "filesystem", "source", artifact.Source, "source_id", artifact.SourceID, "bucket", artifact.Bucket, "path", artifact.Path, "byte_size", artifact.ByteSize, "duration_ms", time.Since(start).Milliseconds())
	return artifact
}

func filesystemObjectPath(root string, bucket string, object string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("OBJECT_STORAGE_ROOT is required for filesystem object storage")
	}
	cleanBucket, err := safeStoragePathPart(bucket, false)
	if err != nil {
		return "", fmt.Errorf("invalid bucket: %w", err)
	}
	cleanObject, err := safeStoragePathPart(object, true)
	if err != nil {
		return "", fmt.Errorf("invalid object path: %w", err)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(rootAbs, filepath.FromSlash(cleanBucket), filepath.FromSlash(cleanObject))
	if fullPath != rootAbs && !strings.HasPrefix(fullPath, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("object path escapes storage root")
	}
	return fullPath, nil
}

func safeStoragePathPart(value string, allowNested bool) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return "", fmt.Errorf("value is empty")
	}
	parts := strings.Split(value, "/")
	if !allowNested && len(parts) != 1 {
		return "", fmt.Errorf("nested bucket names are not allowed")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe path segment %q", part)
		}
	}
	return strings.TrimPrefix(pathpkg.Clean("/"+value), "/"), nil
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
