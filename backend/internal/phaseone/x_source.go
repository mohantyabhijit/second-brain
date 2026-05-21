package phaseone

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

type xUserResponse struct {
	Data *struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"data"`
}

type xBookmarkResponse struct {
	Data []struct {
		ID            string         `json:"id"`
		Text          string         `json:"text"`
		AuthorID      string         `json:"author_id"`
		CreatedAt     string         `json:"created_at"`
		PublicMetrics map[string]int `json:"public_metrics"`
		Article       *struct {
			Title       string `json:"title"`
			PlainText   string `json:"plain_text"`
			PreviewText string `json:"preview_text"`
		} `json:"article"`
		Entities *struct {
			URLs []struct {
				ExpandedURL string `json:"expanded_url"`
				UnwoundURL  string `json:"unwound_url"`
			} `json:"urls"`
		} `json:"entities"`
	} `json:"data"`
	Includes *struct {
		Users []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"users"`
	} `json:"includes"`
}

func (s *Service) fetchXBookmarks(ctx context.Context, limit int) ([]XBookmark, error) {
	if os.Getenv("X_USER_ACCESS_TOKEN") == "" && !s.cfg.OneCLIGateway {
		return nil, fmt.Errorf(credentialHint("X_USER_ACCESS_TOKEN"))
	}

	headers := authHeader("X_USER_ACCESS_TOKEN", "Bearer {value}")
	var me xUserResponse
	if err := s.requestJSON(ctx, http.MethodGet, "https://api.x.com/2/users/me?user.fields=username,name", headers, nil, &me); err != nil {
		return nil, fmt.Errorf("X /2/users/me failed: %w", err)
	}
	if me.Data == nil || me.Data.ID == "" {
		return nil, fmt.Errorf("X /2/users/me did not return an authenticated user id")
	}

	requestURL := "https://api.x.com/2/users/" + me.Data.ID + "/bookmarks"
	requestURL = appendQueryValue(requestURL, "max_results", strconv.Itoa(limit))
	requestURL = appendQueryValue(requestURL, "tweet.fields", "article,created_at,public_metrics,author_id,entities")
	requestURL = appendQueryValue(requestURL, "expansions", "author_id,article.cover_media")
	requestURL = appendQueryValue(requestURL, "media.fields", "url,preview_image_url,type,alt_text")
	requestURL = appendQueryValue(requestURL, "user.fields", "username,name")

	var payload xBookmarkResponse
	if err := s.requestJSON(ctx, http.MethodGet, requestURL, headers, nil, &payload); err != nil {
		return nil, fmt.Errorf("X bookmarks failed: %w", err)
	}

	users := map[string]struct {
		Name     string
		Username string
	}{}
	if payload.Includes != nil {
		for _, user := range payload.Includes.Users {
			users[user.ID] = struct {
				Name     string
				Username string
			}{Name: user.Name, Username: user.Username}
		}
	}

	bookmarks := make([]XBookmark, 0, min(limit, len(payload.Data)))
	for i, tweet := range payload.Data {
		if i >= limit {
			break
		}
		user := users[tweet.AuthorID]
		body := tweet.Text
		contentType := "tweet"
		title := ""
		preview := ""
		if tweet.Article != nil && tweet.Article.PlainText != "" {
			contentType = "article"
			title = tweet.Article.Title
			body = tweet.Article.PlainText
			preview = tweet.Article.PreviewText
		}

		expandedURL := ""
		if tweet.Entities != nil {
			for _, item := range tweet.Entities.URLs {
				if item.UnwoundURL != "" {
					expandedURL = item.UnwoundURL
					break
				}
				if item.ExpandedURL != "" {
					expandedURL = item.ExpandedURL
					break
				}
			}
		}

		sourceURL := "https://x.com/i/web/status/" + tweet.ID
		if user.Username != "" && contentType == "article" {
			sourceURL = "https://x.com/" + user.Username + "/article/" + tweet.ID
		} else if user.Username != "" {
			sourceURL = "https://x.com/" + user.Username + "/status/" + tweet.ID
		}

		bookmarks = append(bookmarks, XBookmark{
			ID:            tweet.ID,
			ContentType:   contentType,
			Text:          tweet.Text,
			Title:         title,
			Body:          body,
			PreviewText:   preview,
			ExpandedURL:   expandedURL,
			AuthorID:      tweet.AuthorID,
			AuthorName:    user.Name,
			Username:      user.Username,
			CreatedAt:     tweet.CreatedAt,
			PublicMetrics: tweet.PublicMetrics,
			SourceURL:     sourceURL,
		})
	}

	return bookmarks, nil
}
