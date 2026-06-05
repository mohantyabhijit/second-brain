package langfuse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

type Client struct {
	baseURL   string
	publicKey string
	secretKey string
	http      *http.Client
}

type TextPrompt struct {
	Name    string
	Version int
	Prompt  string
	Labels  []string
	Config  map[string]any
}

type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return e.Status
	}
	return fmt.Sprintf("%s: %s", e.Status, body)
}

func NewClient(cfg config.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.LangfuseBaseURL), "/"),
		publicKey: strings.TrimSpace(cfg.LangfusePublicKey),
		secretKey: strings.TrimSpace(cfg.LangfuseSecretKey),
		http:      httpClient,
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *Client) GetTextPrompt(ctx context.Context, name string, label string) (*TextPrompt, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("LANGFUSE_BASE_URL is not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("prompt name is required")
	}
	if strings.TrimSpace(label) == "" {
		label = "production"
	}
	endpoint := c.baseURL + "/api/public/v2/prompts/" + url.PathEscape(name)
	endpoint = appendQueryValue(endpoint, "label", label)
	var response promptAPIResponse
	if err := c.requestJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return nil, err
	}
	prompt, err := response.toTextPrompt()
	if err != nil {
		return nil, err
	}
	return prompt, nil
}

func (c *Client) EnsureTextPrompt(ctx context.Context, name string, prompt string, labels []string, promptConfig map[string]any) (*TextPrompt, bool, error) {
	name = strings.TrimSpace(name)
	prompt = strings.TrimSpace(prompt)
	if name == "" {
		return nil, false, fmt.Errorf("prompt name is required")
	}
	if prompt == "" {
		return nil, false, fmt.Errorf("prompt body is required")
	}
	label := "production"
	if len(labels) > 0 && strings.TrimSpace(labels[0]) != "" {
		label = strings.TrimSpace(labels[0])
	}
	existing, err := c.GetTextPrompt(ctx, name, label)
	if err == nil && strings.TrimSpace(existing.Prompt) == prompt {
		return existing, false, nil
	}
	if err != nil {
		var apiErr *APIError
		if !isAPIStatus(err, http.StatusNotFound, &apiErr) {
			return nil, false, err
		}
	}

	request := map[string]any{
		"name":   name,
		"type":   "text",
		"prompt": prompt,
		"labels": labels,
	}
	if len(promptConfig) > 0 {
		request["config"] = promptConfig
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, false, err
	}
	var response promptAPIResponse
	if err := c.requestJSON(ctx, http.MethodPost, c.baseURL+"/api/public/v2/prompts", bytes.NewReader(raw), &response); err != nil {
		return nil, false, err
	}
	created, err := response.toTextPrompt()
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

func CompileTextPrompt(template string, values map[string]string) string {
	compiled := template
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		compiled = strings.ReplaceAll(compiled, "{{"+key+"}}", value)
		compiled = strings.ReplaceAll(compiled, "{{ "+key+" }}", value)
	}
	return compiled
}

func (c *Client) requestJSON(ctx context.Context, method string, endpoint string, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.publicKey != "" && c.secretKey != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.secretKey))
		req.Header.Set("Authorization", "Basic "+auth)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{
			StatusCode: response.StatusCode,
			Status:     response.Status,
			Body:       string(raw),
		}
	}
	if len(raw) == 0 || target == nil {
		return nil
	}
	return json.Unmarshal(raw, target)
}

type promptAPIResponse struct {
	Name    string          `json:"name"`
	Version int             `json:"version"`
	Prompt  json.RawMessage `json:"prompt"`
	Labels  []string        `json:"labels"`
	Config  map[string]any  `json:"config"`
}

func (r promptAPIResponse) toTextPrompt() (*TextPrompt, error) {
	var prompt string
	if err := json.Unmarshal(r.Prompt, &prompt); err != nil {
		return nil, fmt.Errorf("langfuse prompt %q is not a text prompt", r.Name)
	}
	return &TextPrompt{
		Name:    r.Name,
		Version: r.Version,
		Prompt:  prompt,
		Labels:  r.Labels,
		Config:  r.Config,
	}, nil
}

func appendQueryValue(rawURL string, key string, value string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	params := parsed.Query()
	params.Set(key, value)
	parsed.RawQuery = params.Encode()
	return parsed.String()
}

func isAPIStatus(err error, status int, target **APIError) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		return false
	}
	if target != nil {
		*target = apiErr
	}
	return apiErr.StatusCode == status
}
