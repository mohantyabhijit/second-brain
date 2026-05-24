package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type sourceCandidate struct {
	sourceType   SourceType
	externalID   string
	sourceURL    string
	title        string
	authorName   string
	username     string
	publishedAt  string
	body         string
	artifactKind string
	contentType  string
	cachedHash   string
}

func candidatesFromBookmarks(bookmarks []XBookmark) []sourceCandidate {
	candidates := make([]sourceCandidate, 0, len(bookmarks))
	for _, bookmark := range bookmarks {
		title := bookmark.Title
		if title == "" && bookmark.Username != "" {
			title = "@" + bookmark.Username
		}
		if title == "" {
			title = fallback(bookmark.AuthorName, "X bookmark")
		}
		contentType := "text/plain; charset=utf-8"
		artifactKind := "tweet"
		if bookmark.ContentType == "article" {
			artifactKind = "article"
		}
		candidates = append(candidates, sourceCandidate{
			sourceType:   SourceTypeX,
			externalID:   bookmark.ID,
			sourceURL:    bookmark.SourceURL,
			title:        title,
			authorName:   bookmark.AuthorName,
			username:     bookmark.Username,
			publishedAt:  bookmark.CreatedAt,
			body:         fallback(bookmark.Body, bookmark.Text),
			artifactKind: artifactKind,
			contentType:  contentType,
		})
	}
	return candidates
}

func candidatesFromVideos(items []YouTubeItem) []sourceCandidate {
	candidates := []sourceCandidate{}
	for _, item := range items {
		body := fallback(fallback(item.TranscriptTimedText, item.TranscriptText), fallback(item.TranscriptPreview, item.TranscriptOriginalPreview))
		artifactKind := "transcript"
		contentType := "text/plain; charset=utf-8"
		if body == "" {
			body = youtubeMetadataBody(item)
			artifactKind = "metadata"
			contentType = "text/markdown; charset=utf-8"
		}
		if body == "" {
			continue
		}
		candidates = append(candidates, sourceCandidate{
			sourceType:   SourceTypeYouTube,
			externalID:   item.VideoID,
			sourceURL:    item.SourceURL,
			title:        fallback(item.Title, "Untitled YouTube video"),
			authorName:   item.ChannelTitle,
			publishedAt:  item.PublishedAt,
			body:         body,
			artifactKind: artifactKind,
			contentType:  contentType,
			cachedHash:   item.CachedCaptureHash,
		})
	}
	return candidates
}

func youtubeMetadataBody(item YouTubeItem) string {
	parts := []string{
		"Title: " + strings.TrimSpace(item.Title),
		"Channel: " + strings.TrimSpace(item.ChannelTitle),
		"Published: " + strings.TrimSpace(item.PublishedAt),
		"Source: " + strings.TrimSpace(item.SourceURL),
		"Transcript status: " + strings.TrimSpace(item.TranscriptStatus),
	}
	if strings.TrimSpace(item.Description) != "" {
		parts = append(parts, "Description: "+strings.TrimSpace(item.Description))
	}
	if strings.TrimSpace(item.TranscriptError) != "" {
		parts = append(parts, "Transcript note: "+strings.TrimSpace(item.TranscriptError))
	}
	return strings.Join(compact(parts), "\n")
}

func (candidate sourceCandidate) captureHash() string {
	if candidate.cachedHash != "" {
		return candidate.cachedHash
	}
	hash := sha256.Sum256([]byte(strings.Join([]string{
		string(candidate.sourceType),
		candidate.externalID,
		candidate.title,
		candidate.sourceURL,
		candidate.body,
	}, "\x00")))
	return hex.EncodeToString(hash[:])
}

func (candidate sourceCandidate) itemContentType() string {
	switch candidate.sourceType {
	case SourceTypeX:
		if candidate.artifactKind == "article" {
			return "article"
		}
		return "post"
	case SourceTypeYouTube:
		return "video"
	default:
		return "document"
	}
}

func (candidate sourceCandidate) storagePath(captureHash string) string {
	if captureHash == "" {
		captureHash = candidate.captureHash()
	}
	switch candidate.sourceType {
	case SourceTypeX:
		return "x/" + candidate.externalID + "/" + captureHash + "/" + candidate.artifactKind + ".txt"
	case SourceTypeYouTube:
		return "youtube/" + candidate.externalID + "/" + captureHash + "/" + candidate.artifactKind + ".txt"
	default:
		return "unknown/" + candidate.externalID + "/" + captureHash + "/" + candidate.artifactKind + ".txt"
	}
}
