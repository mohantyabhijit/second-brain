package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type resendResponse struct {
	ID    string `json:"id"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Service) deliverDigest(ctx context.Context, digest DigestIssue) DigestDelivery {
	now := time.Now().UTC()
	delivery := DigestDelivery{
		Provider:    "resend",
		Recipient:   s.cfg.DigestEmailTo,
		Status:      "blocked",
		AttemptedAt: &now,
	}
	if strings.TrimSpace(s.cfg.DigestEmailTo) == "" {
		delivery.Error = "DIGEST_EMAIL_TO is not configured."
		return delivery
	}
	if strings.TrimSpace(s.cfg.ResendAPIKey) == "" && !s.cfg.OneCLIGateway {
		delivery.Error = credentialHint("RESEND_API_KEY")
		return delivery
	}

	body := map[string]any{
		"from":    s.cfg.DigestEmailFrom,
		"to":      []string{s.cfg.DigestEmailTo},
		"subject": digest.Subject,
		"text":    digest.BodyMarkdown,
		"headers": map[string]string{
			"Idempotency-Key": digest.IdempotencyKey,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		delivery.Error = err.Error()
		return delivery
	}
	headers := authHeader("RESEND_API_KEY", "Bearer {value}")
	headers.Set("Content-Type", "application/json")
	var response resendResponse
	if err := s.requestJSON(ctx, http.MethodPost, "https://api.resend.com/emails", headers, bytes.NewReader(raw), &response); err != nil {
		delivery.Status = "failed"
		delivery.Error = fmt.Sprintf("Resend delivery failed: %v", err)
		return delivery
	}
	if response.Error != nil && response.Error.Message != "" {
		delivery.Status = "failed"
		delivery.Error = response.Error.Message
		return delivery
	}
	delivery.Status = "sent"
	delivery.ProviderMessageID = response.ID
	return delivery
}
