package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestWriteEvidenceArtifactRecordsMetadataWithoutStorageCredentials(t *testing.T) {
	candidate := sourceCandidate{
		sourceType:   SourceTypeX,
		externalID:   "tweet-1",
		title:        "Source tweet",
		body:         "source body",
		artifactKind: "tweet",
		contentType:  "text/plain; charset=utf-8",
	}
	service := NewService(config.Config{ObjectStorageBackend: "supabase", ObjectStorageBucket: "sources"}, cacheStore{}, http.DefaultClient)

	captureHash := candidate.captureHash()
	artifact := service.writeEvidenceArtifact(context.Background(), candidate, captureHash)

	expectedChecksumBytes := sha256.Sum256([]byte("source body"))
	if artifact.Source != "x" || artifact.SourceID != "tweet-1" || artifact.Kind != "tweet" {
		t.Fatalf("unexpected artifact identity: %#v", artifact)
	}
	if artifact.Path != "x/tweet-1/"+captureHash+"/tweet.txt" || artifact.Bucket != "sources" {
		t.Fatalf("unexpected artifact location: %#v", artifact)
	}
	if artifact.ByteSize != len("source body") {
		t.Fatalf("expected byte size %d, got %d", len("source body"), artifact.ByteSize)
	}
	if artifact.Checksum != hex.EncodeToString(expectedChecksumBytes[:]) {
		t.Fatalf("unexpected checksum: %q", artifact.Checksum)
	}
	if artifact.Stored {
		t.Fatal("expected artifact to remain unstored without credentials")
	}
	if !strings.Contains(artifact.Error, "Supabase Storage credentials missing") {
		t.Fatalf("expected missing credentials error, got %q", artifact.Error)
	}
}

func TestWriteEvidenceArtifactUploadsToSupabaseStorage(t *testing.T) {
	var captured struct {
		method        string
		path          string
		authorization string
		apiKey        string
		contentType   string
		upsert        string
		body          string
	}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		captured.method = r.Method
		captured.path = r.URL.EscapedPath()
		captured.authorization = r.Header.Get("Authorization")
		captured.apiKey = r.Header.Get("apikey")
		captured.contentType = r.Header.Get("Content-Type")
		captured.upsert = r.Header.Get("x-upsert")
		captured.body = string(raw)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("{}")),
			Request:    r,
		}, nil
	})}

	candidate := sourceCandidate{
		sourceType:   SourceTypeYouTube,
		externalID:   "video id",
		title:        "Video",
		body:         "transcript body",
		artifactKind: "transcript",
		contentType:  "text/plain; charset=utf-8",
	}
	service := NewService(config.Config{
		SupabaseURL:          "https://supabase.example",
		SupabaseStorageKey:   "service-role",
		ObjectStorageBackend: "supabase",
		ObjectStorageBucket:  "sources",
	}, cacheStore{}, client)

	captureHash := candidate.captureHash()
	artifact := service.writeEvidenceArtifact(context.Background(), candidate, captureHash)

	if !artifact.Stored || artifact.Error != "" {
		t.Fatalf("expected stored artifact without error, got %#v", artifact)
	}
	if captured.method != http.MethodPut {
		t.Fatalf("expected PUT upload, got %s", captured.method)
	}
	if captured.path != "/storage/v1/object/sources/youtube/video%20id/"+captureHash+"/transcript.txt" {
		t.Fatalf("unexpected upload path: %q", captured.path)
	}
	if captured.authorization != "Bearer service-role" || captured.apiKey != "service-role" {
		t.Fatalf("expected Supabase auth headers, got authorization=%q apikey=%q", captured.authorization, captured.apiKey)
	}
	if captured.contentType != "text/plain; charset=utf-8" || captured.upsert != "true" {
		t.Fatalf("unexpected upload headers: contentType=%q upsert=%q", captured.contentType, captured.upsert)
	}
	if captured.body != "transcript body" {
		t.Fatalf("unexpected upload body: %q", captured.body)
	}
}

func TestWriteEvidenceArtifactStoresOnFilesystem(t *testing.T) {
	root := t.TempDir()
	candidate := sourceCandidate{
		sourceType:   SourceTypeYouTube,
		externalID:   "video id",
		title:        "Video",
		body:         "transcript body",
		artifactKind: "transcript",
		contentType:  "text/plain; charset=utf-8",
	}
	service := NewService(config.Config{
		ObjectStorageBackend: "filesystem",
		ObjectStorageRoot:    root,
		ObjectStorageBucket:  "sources",
	}, cacheStore{}, http.DefaultClient)

	captureHash := candidate.captureHash()
	artifact := service.writeEvidenceArtifact(context.Background(), candidate, captureHash)

	if !artifact.Stored || artifact.Error != "" {
		t.Fatalf("expected filesystem-stored artifact without error, got %#v", artifact)
	}
	storedPath := filepath.Join(root, "sources", "youtube", "video id", captureHash, "transcript.txt")
	raw, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("read stored artifact: %v", err)
	}
	if string(raw) != "transcript body" {
		t.Fatalf("unexpected stored body: %q", string(raw))
	}
}

func TestFilesystemObjectPathRejectsTraversal(t *testing.T) {
	root := t.TempDir()

	for _, tc := range []struct {
		name   string
		bucket string
		object string
	}{
		{name: "bucket traversal", bucket: "../sources", object: "x/tweet-1/source.txt"},
		{name: "object traversal", bucket: "sources", object: "x/../source.txt"},
		{name: "empty object segment", bucket: "sources", object: "x//source.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := filesystemObjectPath(root, tc.bucket, tc.object); err == nil {
				t.Fatal("expected unsafe storage path to be rejected")
			}
		})
	}
}

func TestWriteSynthesisArtifactRecordsProcessedOutputMetadata(t *testing.T) {
	candidate := sourceCandidate{
		sourceType:   SourceTypeX,
		externalID:   "tweet-1",
		sourceURL:    "https://x.com/example/status/tweet-1",
		title:        "Source tweet",
		body:         "source body",
		artifactKind: "tweet",
		contentType:  "text/plain; charset=utf-8",
	}
	record := SynthesisRecord{
		CaptureHash:   "capture-1",
		PromptVersion: "prompt/v1",
		Model:         "model one",
		Summary: Summary{
			ID:      "tweet-1",
			Source:  "x",
			Summary: "Processed summary",
		},
	}
	service := NewService(config.Config{ObjectStorageBackend: "supabase", ObjectStorageBucket: "sources"}, cacheStore{}, http.DefaultClient)

	artifact := service.writeSynthesisArtifact(context.Background(), candidate, "capture-1", record)

	if artifact.Kind != "summary_json" || artifact.ContentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected synthesis artifact metadata: %#v", artifact)
	}
	expectedPath := "artifacts/x/tweet-1/capture-1/prompt_v1/model-one/summary.json"
	if artifact.Path != expectedPath {
		t.Fatalf("unexpected synthesis artifact path: %q", artifact.Path)
	}
	if artifact.ByteSize == 0 || artifact.Checksum == "" {
		t.Fatalf("expected serialized synthesis artifact, got %#v", artifact)
	}
	if !strings.Contains(artifact.Error, "Supabase Storage credentials missing") {
		t.Fatalf("expected missing credentials error, got %q", artifact.Error)
	}
}
