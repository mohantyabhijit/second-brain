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
		if item.TranscriptStatus != "available" {
			continue
		}
		body := fallback(item.TranscriptPreview, item.TranscriptOriginalPreview)
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
			artifactKind: "transcript",
			contentType:  "text/plain; charset=utf-8",
		})
	}
	return candidates
}

func (candidate sourceCandidate) captureHash() string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		string(candidate.sourceType),
		candidate.externalID,
		candidate.title,
		candidate.sourceURL,
		candidate.body,
	}, "\x00")))
	return hex.EncodeToString(hash[:])
}

func (candidate sourceCandidate) storagePath() string {
	switch candidate.sourceType {
	case SourceTypeX:
		return "x/" + candidate.externalID + "/" + candidate.artifactKind + ".txt"
	case SourceTypeYouTube:
		return "youtube/" + candidate.externalID + "/" + candidate.artifactKind + ".txt"
	default:
		return "unknown/" + candidate.externalID + "/" + candidate.artifactKind + ".txt"
	}
}
