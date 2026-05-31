package knowledge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/abhijitmohanty/second-brain/backend/prompts"
)

type openAIImageResponse struct {
	Data []struct {
		B64JSON       string `json:"b64_json"`
		URL           string `json:"url"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Service) addDigestIllustration(ctx context.Context, digest *DigestIssue, insights []Insight, themes []ThemeCluster, connections []SourceConnection) error {
	if digest == nil || strings.TrimSpace(digest.IllustrationBase64) != "" {
		return nil
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" && !s.cfg.OneCLIGateway {
		return fmt.Errorf("OPENAI_API_KEY is required for digest illustration generation")
	}
	illustration, err := s.generateDigestIllustration(ctx, *digest, insights, themes, connections)
	if err != nil {
		return fmt.Errorf("digest illustration generation failed: %w", err)
	}
	digest.IllustrationPrompt = illustration.prompt
	digest.IllustrationAlt = illustration.alt
	digest.IllustrationMimeType = illustration.mimeType
	digest.IllustrationModel = illustration.model
	digest.IllustrationBase64 = illustration.base64
	return nil
}

func (s *Service) annotateDigestIllustration(digest *DigestIssue) {
	if digest == nil || strings.TrimSpace(digest.ID) == "" {
		return
	}
	hasIllustration := digest.IllustrationAvailable ||
		strings.TrimSpace(digest.IllustrationBase64) != "" ||
		strings.TrimSpace(digest.IllustrationMimeType) != "" ||
		strings.TrimSpace(digest.IllustrationAlt) != ""
	if !hasIllustration {
		return
	}
	digest.IllustrationURL = s.digestIllustrationURL(digest.ID)
}

func (s *Service) digestIllustrationURL(digestID string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.PublicBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return baseURL + "/api/digests/" + url.PathEscape(digestID) + "/illustration"
}

type generatedIllustration struct {
	prompt   string
	alt      string
	mimeType string
	model    string
	base64   string
}

func (s *Service) generateDigestIllustration(ctx context.Context, digest DigestIssue, insights []Insight, themes []ThemeCluster, connections []SourceConnection) (generatedIllustration, error) {
	model := strings.TrimSpace(s.cfg.OpenAIImageModel)
	if model == "" {
		model = "gpt-image-1"
	}
	prompt := digestIllustrationPrompt(digest, insights, themes, connections)
	body := map[string]any{
		"model":  model,
		"prompt": prompt,
		"size":   "1024x1024",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return generatedIllustration{}, err
	}
	headers := authHeader("OPENAI_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")

	var response openAIImageResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", headers, bytes.NewReader(raw), &response); err != nil {
		return generatedIllustration{}, err
	}
	if response.Error != nil && response.Error.Message != "" {
		return generatedIllustration{}, fmt.Errorf(response.Error.Message)
	}
	if len(response.Data) == 0 {
		return generatedIllustration{}, fmt.Errorf("OpenAI image response returned no data")
	}
	image := response.Data[0]
	mimeType := "image/png"
	imageBase64 := strings.TrimSpace(image.B64JSON)
	if imageBase64 == "" && strings.TrimSpace(image.URL) != "" {
		var err error
		imageBase64, mimeType, err = s.fetchIllustrationURL(ctx, image.URL)
		if err != nil {
			return generatedIllustration{}, err
		}
	}
	if imageBase64 == "" {
		return generatedIllustration{}, fmt.Errorf("OpenAI image response did not include image bytes")
	}
	if _, err := base64.StdEncoding.DecodeString(imageBase64); err != nil {
		return generatedIllustration{}, fmt.Errorf("OpenAI image response was not valid base64: %w", err)
	}
	return generatedIllustration{
		prompt:   prompt,
		alt:      digestIllustrationAlt(digest, insights, themes),
		mimeType: mimeType,
		model:    model,
		base64:   imageBase64,
	}, nil
}

func (s *Service) fetchIllustrationURL(ctx context.Context, rawURL string) (string, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", "", fmt.Errorf("fetch generated illustration %s", response.Status)
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", "", err
	}
	mimeType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if mimeType == "" {
		mimeType = http.DetectContentType(raw)
	}
	if index := strings.Index(mimeType, ";"); index >= 0 {
		mimeType = strings.TrimSpace(mimeType[:index])
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	return base64.StdEncoding.EncodeToString(raw), mimeType, nil
}

func digestIllustrationPrompt(digest DigestIssue, insights []Insight, themes []ThemeCluster, connections []SourceConnection) string {
	subject := truncateDigestText(fallback(digest.Subject, "Second Brain newsletter"), 120)
	pattern := truncateDigestText(digestPreviewText(digest), 240)
	signals := []string{}
	for _, theme := range themes {
		if len(signals) >= 3 {
			break
		}
		if strings.TrimSpace(theme.Label) != "" {
			signals = append(signals, theme.Label)
		}
	}
	for _, insight := range insights {
		if len(signals) >= 5 {
			break
		}
		if strings.TrimSpace(insight.Title) != "" {
			signals = append(signals, insight.Title)
		}
	}
	for _, connection := range connections {
		if len(signals) >= 6 {
			break
		}
		if strings.TrimSpace(connection.Relationship) != "" {
			signals = append(signals, connection.Relationship)
		}
	}
	if len(signals) == 0 {
		signals = append(signals, "connected notes", "decision making", "learning loop")
	}

	return prompts.DigestIllustration(subject, pattern, signals)
}

func digestIllustrationAlt(digest DigestIssue, insights []Insight, themes []ThemeCluster) string {
	subject := strings.TrimSpace(digest.Subject)
	if subject != "" {
		return "Black-and-white hand-drawn illustration for " + subject
	}
	if len(themes) > 0 && strings.TrimSpace(themes[0].Label) != "" {
		return "Black-and-white hand-drawn illustration about " + themes[0].Label
	}
	if len(insights) > 0 && strings.TrimSpace(insights[0].Title) != "" {
		return "Black-and-white hand-drawn illustration about " + insights[0].Title
	}
	return "Black-and-white hand-drawn illustration for Abhijit's Second Brain newsletter"
}

func ensureDigestID(digest *DigestIssue) error {
	if digest == nil || strings.TrimSpace(digest.ID) != "" {
		return nil
	}
	id, err := randomUUID()
	if err != nil {
		return err
	}
	digest.ID = id
	return nil
}

func randomUUID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}
