package knowledge

import (
	"strings"
	"time"
)

type SourceMaterialKey struct {
	SourceType    SourceType `json:"sourceType"`
	ExternalID    string     `json:"externalId"`
	PromptVersion string     `json:"promptVersion"`
	Model         string     `json:"model"`
}

func (key SourceMaterialKey) String() string {
	return strings.Join([]string{
		string(key.SourceType),
		key.ExternalID,
		key.PromptVersion,
		key.Model,
	}, ":")
}

func (key SourceMaterialKey) Identity() string {
	return sourceMaterialIdentity(key.SourceType, key.ExternalID)
}

type SourceMaterialState struct {
	SourceType        SourceType `json:"sourceType"`
	ExternalID        string     `json:"externalId"`
	LatestCaptureHash string     `json:"latestCaptureHash,omitempty"`
	PromptVersion     string     `json:"promptVersion,omitempty"`
	Model             string     `json:"model,omitempty"`
	ContentType       string     `json:"contentType,omitempty"`
	ArtifactKind      string     `json:"artifactKind,omitempty"`
	Processed         bool       `json:"processed"`
	LastSeenAt        time.Time  `json:"lastSeenAt,omitempty"`
}

func (state SourceMaterialState) Key() SourceMaterialKey {
	return SourceMaterialKey{
		SourceType:    state.SourceType,
		ExternalID:    state.ExternalID,
		PromptVersion: state.PromptVersion,
		Model:         state.Model,
	}
}

func (state SourceMaterialState) HasProcessedTranscript() bool {
	if !state.Processed || state.SourceType != SourceTypeYouTube {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(state.ArtifactKind), "transcript")
}

func SourceMaterialStatesFromResult(result *Result) []SourceMaterialState {
	if result == nil {
		return nil
	}
	xContentTypes := map[string]string{}
	for _, bookmark := range result.XBookmarks {
		contentType := "post"
		if bookmark.ContentType == "article" {
			contentType = "article"
		}
		xContentTypes[bookmark.ID] = contentType
	}
	youtubeArtifactKinds := map[string]string{}
	for _, item := range result.YouTubeItems {
		kind := "metadata"
		if item.TranscriptStatus == "available" || strings.TrimSpace(item.TranscriptPreview) != "" || strings.TrimSpace(item.TranscriptText) != "" {
			kind = "transcript"
		}
		youtubeArtifactKinds[item.VideoID] = kind
	}

	statesByKey := map[string]SourceMaterialState{}
	for _, summary := range result.Summaries {
		sourceType := SourceType(strings.TrimSpace(summary.Source))
		if sourceType == "" || strings.TrimSpace(summary.ID) == "" || strings.TrimSpace(summary.PromptVersion) == "" || strings.TrimSpace(summary.Model) == "" {
			continue
		}
		state := SourceMaterialState{
			SourceType:        sourceType,
			ExternalID:        summary.ID,
			LatestCaptureHash: summary.CaptureHash,
			PromptVersion:     summary.PromptVersion,
			Model:             summary.Model,
			Processed:         true,
		}
		if summary.GeneratedAt != nil {
			state.LastSeenAt = *summary.GeneratedAt
		} else {
			state.LastSeenAt = result.GeneratedAt
		}
		switch sourceType {
		case SourceTypeX:
			state.ContentType = fallback(xContentTypes[summary.ID], "post")
			state.ArtifactKind = state.ContentType
		case SourceTypeYouTube:
			state.ContentType = "video"
			state.ArtifactKind = fallback(youtubeArtifactKinds[summary.ID], "metadata")
		default:
			state.ContentType = "document"
			state.ArtifactKind = "source"
		}
		statesByKey[state.Key().String()] = state
	}

	states := make([]SourceMaterialState, 0, len(statesByKey))
	for _, state := range statesByKey {
		states = append(states, state)
	}
	return states
}

func sourceMaterialIdentity(sourceType SourceType, externalID string) string {
	return string(sourceType) + ":" + strings.TrimSpace(externalID)
}

func sourceMaterialKeyForCandidate(candidate sourceCandidate, promptVersion string, model string) SourceMaterialKey {
	return SourceMaterialKey{
		SourceType:    candidate.sourceType,
		ExternalID:    candidate.externalID,
		PromptVersion: promptVersion,
		Model:         model,
	}
}
