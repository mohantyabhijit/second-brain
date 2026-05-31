package knowledge

import "strings"

func filterNewSourceCandidates(candidates []sourceCandidate, states map[string]SourceMaterialState, model string) ([]sourceCandidate, []sourceCandidate) {
	newCandidates := make([]sourceCandidate, 0, len(candidates))
	skipped := []sourceCandidate{}
	for _, candidate := range candidates {
		key := sourceMaterialKeyForCandidate(candidate, synthesisPromptVersion, model)
		state, ok := states[key.String()]
		if ok && state.Processed {
			captureHash := candidate.captureHash()
			if state.LatestCaptureHash == "" || state.LatestCaptureHash == captureHash {
				skipped = append(skipped, candidate)
				continue
			}
		}
		newCandidates = append(newCandidates, candidate)
	}
	return newCandidates, skipped
}

func sourceMaterialKeysFromFetched(bookmarks []XBookmark, youtubeItems []YouTubeItem, promptVersion string, model string) []SourceMaterialKey {
	keys := make([]SourceMaterialKey, 0, len(bookmarks)+len(youtubeItems))
	seen := map[string]bool{}
	add := func(key SourceMaterialKey) {
		if strings.TrimSpace(key.ExternalID) == "" || seen[key.String()] {
			return
		}
		seen[key.String()] = true
		keys = append(keys, key)
	}
	for _, bookmark := range bookmarks {
		add(SourceMaterialKey{
			SourceType:    SourceTypeX,
			ExternalID:    bookmark.ID,
			PromptVersion: promptVersion,
			Model:         model,
		})
	}
	for _, item := range youtubeItems {
		add(SourceMaterialKey{
			SourceType:    SourceTypeYouTube,
			ExternalID:    item.VideoID,
			PromptVersion: promptVersion,
			Model:         model,
		})
	}
	return keys
}

func processedSourceIDs(sources []ProcessedSource) map[string]bool {
	ids := map[string]bool{}
	for _, source := range sources {
		ids[sourceMaterialIdentity(source.SourceType, source.ExternalID)] = true
	}
	return ids
}

func processedSourcesFromResult(result *Result, exclude map[string]bool) []ProcessedSource {
	if result == nil {
		return nil
	}
	contentTypes := map[string]string{}
	for _, bookmark := range result.XBookmarks {
		contentType := "post"
		if bookmark.ContentType == "article" {
			contentType = "article"
		}
		contentTypes[sourceMaterialIdentity(SourceTypeX, bookmark.ID)] = contentType
	}
	for _, item := range result.YouTubeItems {
		contentTypes[sourceMaterialIdentity(SourceTypeYouTube, item.VideoID)] = "video"
	}
	insightsBySource := map[string][]Insight{}
	for _, insight := range result.Insights {
		insightsBySource[sourceMaterialIdentity(SourceType(insight.Source), insight.SourceID)] = append(insightsBySource[sourceMaterialIdentity(SourceType(insight.Source), insight.SourceID)], insight)
	}
	actionsBySource := map[string][]ActionItem{}
	for _, action := range result.ActionItems {
		actionsBySource[sourceMaterialIdentity(SourceType(action.Source), action.SourceID)] = append(actionsBySource[sourceMaterialIdentity(SourceType(action.Source), action.SourceID)], action)
	}
	sources := []ProcessedSource{}
	seen := map[string]bool{}
	for _, summary := range result.Summaries {
		sourceType := SourceType(summary.Source)
		key := sourceMaterialIdentity(sourceType, summary.ID)
		if key == ":" || exclude[key] || seen[key] {
			continue
		}
		seen[key] = true
		sources = append(sources, ProcessedSource{
			SourceType:  sourceType,
			ContentType: fallback(contentTypes[key], contentTypeForSummary(sourceType)),
			ExternalID:  summary.ID,
			SourceURL:   summary.SourceURL,
			Title:       summary.Title,
			CaptureHash: summary.CaptureHash,
			Synthesis: SynthesisRecord{
				SourceType:    sourceType,
				ExternalID:    summary.ID,
				CaptureHash:   summary.CaptureHash,
				PromptVersion: summary.PromptVersion,
				Model:         summary.Model,
				Summary:       summary,
				Insights:      insightsBySource[key],
				ActionItems:   actionsBySource[key],
			},
			Cached: true,
		})
	}
	return sources
}

func contentTypeForSummary(sourceType SourceType) string {
	switch sourceType {
	case SourceTypeX:
		return "post"
	case SourceTypeYouTube:
		return "video"
	default:
		return "document"
	}
}

func mergeXBookmarks(previous []XBookmark, fetched []XBookmark) []XBookmark {
	if len(previous) == 0 {
		return fetched
	}
	merged := make([]XBookmark, 0, len(fetched)+len(previous))
	seen := map[string]bool{}
	for _, item := range fetched {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			continue
		}
		merged = append(merged, item)
		seen[item.ID] = true
	}
	for _, item := range previous {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			continue
		}
		merged = append(merged, item)
		seen[item.ID] = true
	}
	return merged
}

func mergeYouTubeItems(previous []YouTubeItem, fetched []YouTubeItem) []YouTubeItem {
	if len(previous) == 0 {
		return fetched
	}
	previousByID := map[string]YouTubeItem{}
	for _, item := range previous {
		previousByID[item.VideoID] = item
	}
	merged := make([]YouTubeItem, 0, len(fetched)+len(previous))
	seen := map[string]bool{}
	for _, item := range fetched {
		if strings.TrimSpace(item.VideoID) == "" || seen[item.VideoID] {
			continue
		}
		if previousItem, ok := previousByID[item.VideoID]; ok && item.TranscriptStatus == "cached" {
			merged = append(merged, previousItem)
		} else {
			merged = append(merged, item)
		}
		seen[item.VideoID] = true
	}
	for _, item := range previous {
		if strings.TrimSpace(item.VideoID) == "" || seen[item.VideoID] {
			continue
		}
		merged = append(merged, item)
		seen[item.VideoID] = true
	}
	return merged
}

func appendProcessedOutput(result *Result, processed []ProcessedSource) {
	for _, item := range processed {
		result.Summaries = append(result.Summaries, item.Synthesis.Summary)
		result.Insights = append(result.Insights, item.Synthesis.Insights...)
		result.ActionItems = append(result.ActionItems, item.Synthesis.ActionItems...)
		if item.SourceType == SourceTypeYouTube && len(item.Synthesis.Summary.ImportantTimeMarkers) > 0 {
			attachYouTubeTimeMarkers(result.YouTubeItems, item.ExternalID, item.Synthesis.Summary.ImportantTimeMarkers)
		}
		if item.Artifact.Path != "" {
			result.Artifacts = append(result.Artifacts, item.Artifact)
		}
		if item.SummaryArtifact.Path != "" {
			result.Artifacts = append(result.Artifacts, item.SummaryArtifact)
		}
		status := "generated"
		detail := "Generated synthesis for current source capture."
		if item.Cached {
			status = "cached"
			detail = "Skipped synthesis because this source capture was already processed."
		}
		if item.Artifact.Error != "" {
			detail += " " + item.Artifact.Error
		}
		if item.SummaryArtifact.Error != "" {
			detail += " " + item.SummaryArtifact.Error
		}
		result.Processing = append(result.Processing, ProcessingEvent{
			Source:        string(item.SourceType),
			SourceID:      item.ExternalID,
			Title:         item.Title,
			CaptureHash:   item.CaptureHash,
			PromptVersion: item.Synthesis.PromptVersion,
			Model:         item.Synthesis.Model,
			Status:        status,
			Detail:        detail,
		})
	}
}

func summariesExcluding(summaries []Summary, exclude map[string]bool) []Summary {
	if len(exclude) == 0 {
		return summaries
	}
	filtered := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if exclude[sourceMaterialIdentity(SourceType(summary.Source), summary.ID)] {
			continue
		}
		filtered = append(filtered, summary)
	}
	return filtered
}

func insightsExcluding(insights []Insight, exclude map[string]bool) []Insight {
	if len(exclude) == 0 {
		return insights
	}
	filtered := make([]Insight, 0, len(insights))
	for _, insight := range insights {
		if exclude[sourceMaterialIdentity(SourceType(insight.Source), insight.SourceID)] {
			continue
		}
		filtered = append(filtered, insight)
	}
	return filtered
}

func actionItemsExcluding(actions []ActionItem, exclude map[string]bool) []ActionItem {
	if len(exclude) == 0 {
		return actions
	}
	filtered := make([]ActionItem, 0, len(actions))
	for _, action := range actions {
		if exclude[sourceMaterialIdentity(SourceType(action.Source), action.SourceID)] {
			continue
		}
		filtered = append(filtered, action)
	}
	return filtered
}

func countCachedTranscripts(items []YouTubeItem) int {
	count := 0
	for _, item := range items {
		if item.TranscriptStatus == "cached" {
			count++
		}
	}
	return count
}
