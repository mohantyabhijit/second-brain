package knowledge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestJSONEnforcesProviderResponseBoundary(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErr    string
		wantSecret bool
	}{
		{"decodes JSON", http.StatusOK, `{"status":"ok"}`, "", false},
		{"accepts empty success", http.StatusNoContent, "", "", false},
		{"rejects malformed JSON", http.StatusOK, `{`, "unexpected end", false},
		{"redacts upstream client error", http.StatusBadRequest, `{"error":"provider-secret-detail"}`, "400 Bad Request", false},
		{"redacts upstream server error", http.StatusBadGateway, `provider-secret-detail`, "502 Bad Gateway", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			var target map[string]string
			err := requestJSONWithClient(context.Background(), server.Client(), http.MethodGet, server.URL, nil, nil, &target)
			if test.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.wantErr))) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "provider-secret-detail") {
				t.Fatalf("upstream response body leaked in error: %v", err)
			}
		})
	}
}

func TestRequestJSONRejectsOversizedProviderResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(io.LimitReader(zeroReader{}, maxProviderResponseBytes+1)),
		}, nil
	})}
	var target any
	err := requestJSONWithClient(context.Background(), client, http.MethodGet, "https://provider.example/data", nil, nil, &target)
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("expected response size error, got %v", err)
	}
}

func TestRequestJSONPreservesTransportErrors(t *testing.T) {
	want := errors.New("network unavailable")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, want })}
	err := requestJSONWithClient(context.Background(), client, http.MethodGet, "https://provider.example/data", nil, nil, &struct{}{})
	if !errors.Is(err, want) {
		t.Fatalf("transport error = %v, want %v", err, want)
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}
