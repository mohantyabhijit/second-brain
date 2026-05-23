package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func (s *Service) requestJSON(ctx context.Context, method string, url string, headers http.Header, body io.Reader, target any) error {
	return requestJSONWithClient(ctx, s.client, method, url, headers, body, target)
}

func requestJSONWithClient(ctx context.Context, client *http.Client, method string, url string, headers http.Header, body io.Reader, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", req.URL.Host, response.Status, strings.TrimSpace(string(raw)))
	}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func directHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{Transport: transport, Timeout: timeout}
}

func authHeader(name string, valueFormat string) http.Header {
	header := http.Header{}
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		header.Set("Authorization", strings.ReplaceAll(valueFormat, "{value}", value))
	}
	return header
}

func apiKeyHeader(name string, headerName string) http.Header {
	header := http.Header{}
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		header.Set(headerName, value)
	}
	return header
}

func appendQueryValue(rawURL string, key string, value string) string {
	if strings.TrimSpace(value) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	params := parsed.Query()
	params.Set(key, value)
	parsed.RawQuery = params.Encode()
	return parsed.String()
}

func credentialHint(name string) string {
	return fmt.Sprintf("%s is not present in process env. Store it in OneCLI and run the backend through onecli run, or export it only for a local validation session.", name)
}
