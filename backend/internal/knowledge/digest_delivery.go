package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
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
		"html":    digestHTML(digest),
		"text":    digestText(digest),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		delivery.Error = err.Error()
		return delivery
	}
	headers := resendAuthHeader(s.cfg.ResendAPIKey)
	headers.Set("Content-Type", "application/json")
	headers.Set("Idempotency-Key", digest.IdempotencyKey)
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

func resendAuthHeader(apiKey string) http.Header {
	header := http.Header{}
	if strings.TrimSpace(apiKey) != "" {
		header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		return header
	}
	return authHeader("RESEND_API_KEY", "Bearer {value}")
}

func digestHTML(digest DigestIssue) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html><body style="margin:0;background:#f6f5f1;color:#1f2933;font-family:Arial,Helvetica,sans-serif;">`)
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f6f5f1;padding:28px 12px;"><tr><td align="center">`)
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:720px;background:#ffffff;border:1px solid #dedbd2;border-radius:8px;overflow:hidden;">`)
	builder.WriteString(`<tr><td style="padding:28px 32px 18px 32px;border-bottom:1px solid #e8e5dc;">`)
	builder.WriteString(`<div style="font-size:12px;line-height:16px;letter-spacing:0.08em;text-transform:uppercase;color:#667085;font-weight:700;">Second Brain</div>`)
	builder.WriteString(`<h1 style="margin:8px 0 0 0;font-size:28px;line-height:34px;color:#111827;font-weight:700;">`)
	builder.WriteString(html.EscapeString(digest.Subject))
	builder.WriteString(`</h1>`)
	if digest.DigestDate != "" {
		builder.WriteString(`<div style="margin-top:8px;font-size:14px;line-height:20px;color:#667085;">Digest for `)
		builder.WriteString(html.EscapeString(digest.DigestDate))
		builder.WriteString(`</div>`)
	}
	builder.WriteString(`</td></tr><tr><td style="padding:8px 32px 30px 32px;">`)

	lines := strings.Split(digest.BodyMarkdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "## "):
			builder.WriteString(`<h2 style="margin:24px 0 12px 0;font-size:18px;line-height:24px;color:#111827;font-weight:700;">`)
			builder.WriteString(renderInlineMarkdown(strings.TrimPrefix(trimmed, "## ")))
			builder.WriteString(`</h2>`)
		case strings.HasPrefix(trimmed, "- "):
			builder.WriteString(`<div style="margin:10px 0;padding:12px 14px;border-left:3px solid #3b82f6;background:#f8fafc;border-radius:6px;font-size:15px;line-height:22px;color:#344054;">`)
			builder.WriteString(renderInlineMarkdown(strings.TrimPrefix(trimmed, "- ")))
			builder.WriteString(`</div>`)
		case strings.HasPrefix(trimmed, "Evidence:"):
			builder.WriteString(`<div style="margin:-4px 0 12px 17px;font-size:13px;line-height:19px;color:#667085;">`)
			builder.WriteString(renderInlineMarkdown(trimmed))
			builder.WriteString(`</div>`)
		default:
			builder.WriteString(`<p style="margin:10px 0;font-size:15px;line-height:22px;color:#344054;">`)
			builder.WriteString(renderInlineMarkdown(trimmed))
			builder.WriteString(`</p>`)
		}
	}
	builder.WriteString(`</td></tr></table>`)
	builder.WriteString(`</td></tr></table></body></html>`)
	return builder.String()
}

func digestText(digest DigestIssue) string {
	lines := strings.Split(digest.BodyMarkdown, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			lines[index] = strings.TrimPrefix(trimmed, "# ")
		case strings.HasPrefix(trimmed, "## "):
			lines[index] = strings.TrimPrefix(trimmed, "## ")
		case strings.HasPrefix(trimmed, "- "):
			lines[index] = "- " + renderPlainMarkdownLinks(strings.TrimPrefix(trimmed, "- "))
		default:
			lines[index] = renderPlainMarkdownLinks(trimmed)
		}
	}
	return strings.Join(lines, "\n")
}

func renderInlineMarkdown(value string) string {
	var builder strings.Builder
	remaining := value
	for {
		start := strings.Index(remaining, "[")
		if start < 0 {
			builder.WriteString(html.EscapeString(remaining))
			return builder.String()
		}
		middle := strings.Index(remaining[start:], "](")
		if middle < 0 {
			builder.WriteString(html.EscapeString(remaining))
			return builder.String()
		}
		middle += start
		end := strings.Index(remaining[middle+2:], ")")
		if end < 0 {
			builder.WriteString(html.EscapeString(remaining))
			return builder.String()
		}
		end += middle + 2
		builder.WriteString(html.EscapeString(remaining[:start]))
		label := remaining[start+1 : middle]
		href := remaining[middle+2 : end]
		builder.WriteString(`<a href="`)
		builder.WriteString(html.EscapeString(href))
		builder.WriteString(`" style="color:#2563eb;text-decoration:underline;">`)
		builder.WriteString(html.EscapeString(label))
		builder.WriteString(`</a>`)
		remaining = remaining[end+1:]
	}
}

func renderPlainMarkdownLinks(value string) string {
	var builder strings.Builder
	remaining := value
	for {
		start := strings.Index(remaining, "[")
		if start < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		middle := strings.Index(remaining[start:], "](")
		if middle < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		middle += start
		end := strings.Index(remaining[middle+2:], ")")
		if end < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		end += middle + 2
		builder.WriteString(remaining[:start])
		builder.WriteString(remaining[start+1 : middle])
		builder.WriteString(": ")
		builder.WriteString(remaining[middle+2 : end])
		remaining = remaining[end+1:]
	}
}
