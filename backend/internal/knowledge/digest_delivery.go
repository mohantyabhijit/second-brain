package knowledge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type resendResponse struct {
	ID    string `json:"id"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

const customRecipientNewsletterIntro = "This is a newsletter from Abhijit's Second Brain. The full issue is below."

func (s *Service) deliverDigest(ctx context.Context, digest DigestIssue, recipientOverride string) DigestDelivery {
	now := time.Now().UTC()
	manualRecipient := strings.TrimSpace(recipientOverride) != ""
	recipient := strings.TrimSpace(recipientOverride)
	if recipient == "" {
		recipient = strings.TrimSpace(s.cfg.DigestEmailTo)
	}
	delivery := DigestDelivery{
		Provider:    "resend",
		Recipient:   recipient,
		Status:      "blocked",
		AttemptedAt: &now,
	}
	if recipient == "" {
		delivery.Error = "DIGEST_EMAIL_TO is not configured."
		return delivery
	}
	if strings.TrimSpace(s.cfg.ResendAPIKey) == "" && !s.cfg.OneCLIGateway {
		delivery.Error = credentialHint("RESEND_API_KEY")
		return delivery
	}

	emailDigest := digestWithCustomRecipientIntro(digest, manualRecipient)
	body := map[string]any{
		"from":    s.cfg.DigestEmailFrom,
		"to":      []string{recipient},
		"subject": digest.Subject,
		"html":    digestHTML(emailDigest),
		"text":    digestText(emailDigest),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		delivery.Error = err.Error()
		return delivery
	}
	headers := resendAuthHeader(s.cfg.ResendAPIKey)
	headers.Set("Content-Type", "application/json")
	headers.Set("Idempotency-Key", digestDeliveryIdempotencyKey(digest.IdempotencyKey, manualRecipient, now))
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

func digestWithCustomRecipientIntro(digest DigestIssue, customRecipient bool) DigestIssue {
	if !customRecipient || strings.Contains(digest.BodyMarkdown, customRecipientNewsletterIntro) {
		return digest
	}
	body := strings.TrimSpace(digest.BodyMarkdown)
	if body == "" {
		digest.BodyMarkdown = customRecipientNewsletterIntro
		return digest
	}
	lines := strings.Split(body, "\n")
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		rest := strings.TrimSpace(strings.Join(lines[1:], "\n"))
		if rest == "" {
			digest.BodyMarkdown = strings.TrimSpace(lines[0]) + "\n\n" + customRecipientNewsletterIntro
			return digest
		}
		digest.BodyMarkdown = strings.TrimSpace(lines[0]) + "\n\n" + customRecipientNewsletterIntro + "\n\n" + rest
		return digest
	}
	digest.BodyMarkdown = customRecipientNewsletterIntro + "\n\n" + body
	return digest
}

func normalizeDigestRecipient(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("recipientEmail is required")
	}
	address, err := mail.ParseAddress(trimmed)
	if err != nil || address.Address == "" {
		return "", fmt.Errorf("recipientEmail must be a valid email address")
	}
	return address.Address, nil
}

func digestDeliveryIdempotencyKey(baseKey string, manualRecipient bool, attemptedAt time.Time) string {
	key := strings.TrimSpace(baseKey)
	if key == "" {
		key = "digest"
	}
	if !manualRecipient {
		return key
	}
	sum := sha256.Sum256([]byte(attemptedAt.UTC().Format(time.RFC3339Nano)))
	return key + ":manual:" + hex.EncodeToString(sum[:])[:16]
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
	builder.WriteString(`<!doctype html><html><head><meta name="viewport" content="width=device-width, initial-scale=1"></head><body style="margin:0;background:#f4f6f8;color:#1f2933;font-family:Arial,Helvetica,sans-serif;-webkit-text-size-adjust:100%;">`)
	builder.WriteString(`<div style="display:none;max-height:0;overflow:hidden;color:transparent;opacity:0;">`)
	builder.WriteString(html.EscapeString(digestPreviewText(digest)))
	builder.WriteString(`</div>`)
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f4f6f8;padding:18px 10px;"><tr><td align="center">`)
	builder.WriteString(`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:600px;background:#ffffff;border:1px solid #d9e0dc;border-radius:8px;overflow:hidden;">`)
	builder.WriteString(`<tr><td style="padding:22px 20px 16px 20px;border-bottom:1px solid #e8ece8;background:#fbfcfb;">`)
	builder.WriteString(`<div style="font-size:12px;line-height:16px;letter-spacing:0.08em;text-transform:uppercase;color:#667085;font-weight:700;">Second Brain</div>`)
	builder.WriteString(`<h1 style="margin:8px 0 0 0;font-size:24px;line-height:30px;color:#111827;font-weight:800;">`)
	builder.WriteString(html.EscapeString(digest.Subject))
	builder.WriteString(`</h1>`)
	if digest.DigestDate != "" {
		builder.WriteString(`<div style="margin-top:10px;display:inline-block;border:1px solid #d9e0dc;border-radius:999px;padding:5px 10px;font-size:13px;line-height:18px;color:#475467;background:#ffffff;">Digest for `)
		builder.WriteString(html.EscapeString(digest.DigestDate))
		builder.WriteString(`</div>`)
	}
	if strings.TrimSpace(digest.IllustrationURL) != "" {
		alt := strings.TrimSpace(digest.IllustrationAlt)
		if alt == "" {
			alt = "Newsletter illustration"
		}
		builder.WriteString(`<div style="margin-top:16px;border:1px solid #d9e0dc;border-radius:8px;overflow:hidden;background:#ffffff;">`)
		builder.WriteString(`<img src="`)
		builder.WriteString(html.EscapeString(digest.IllustrationURL))
		builder.WriteString(`" alt="`)
		builder.WriteString(html.EscapeString(alt))
		builder.WriteString(`" width="558" style="display:block;width:100%;max-width:558px;height:auto;border:0;">`)
		builder.WriteString(`</div>`)
	}
	builder.WriteString(`</td></tr><tr><td style="padding:10px 20px 24px 20px;">`)

	lines := strings.Split(digest.BodyMarkdown, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "## "):
			builder.WriteString(`<h2 style="margin:22px 0 10px 0;font-size:19px;line-height:25px;color:#111827;font-weight:800;">`)
			builder.WriteString(renderInlineMarkdown(strings.TrimPrefix(trimmed, "## ")))
			builder.WriteString(`</h2>`)
		case strings.HasPrefix(trimmed, "### "):
			builder.WriteString(`<h3 style="margin:18px 0 8px 0;font-size:17px;line-height:23px;color:#111827;font-weight:800;">`)
			builder.WriteString(renderInlineMarkdown(strings.TrimPrefix(trimmed, "### ")))
			builder.WriteString(`</h3>`)
		case strings.HasPrefix(trimmed, "- "):
			builder.WriteString(`<p style="margin:8px 0 8px 18px;font-size:16px;line-height:24px;color:#344054;">&bull; `)
			builder.WriteString(renderInlineMarkdown(strings.TrimPrefix(trimmed, "- ")))
			builder.WriteString(`</p>`)
		case strings.HasPrefix(trimmed, "Evidence:"), strings.HasPrefix(trimmed, "Source note:"), strings.HasPrefix(trimmed, "The source-backed detail:"):
			builder.WriteString(`<div style="margin:-2px 0 14px 16px;font-size:14px;line-height:20px;color:#667085;">`)
			builder.WriteString(renderInlineMarkdown(trimmed))
			builder.WriteString(`</div>`)
		default:
			builder.WriteString(`<p style="margin:12px 0;font-size:16px;line-height:24px;color:#344054;">`)
			builder.WriteString(renderInlineMarkdown(trimmed))
			builder.WriteString(`</p>`)
		}
	}
	builder.WriteString(`<div style="margin-top:20px;border-top:1px solid #e8ece8;padding-top:14px;font-size:13px;line-height:19px;color:#667085;">Source-linked and generated from your saved knowledge base.</div>`)
	builder.WriteString(`</td></tr></table>`)
	builder.WriteString(`</td></tr></table></body></html>`)
	return builder.String()
}

func digestPreviewText(digest DigestIssue) string {
	for _, line := range strings.Split(digest.BodyMarkdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- ") {
			continue
		}
		plain := renderPlainMarkdownLinks(strings.ReplaceAll(trimmed, "**", ""))
		if len(plain) > 150 {
			return plain[:147] + "..."
		}
		return plain
	}
	return "A quick, source-linked brief from your saved X bookmarks and YouTube videos."
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
		case strings.HasPrefix(trimmed, "### "):
			lines[index] = strings.TrimPrefix(trimmed, "### ")
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
			builder.WriteString(renderInlineText(remaining))
			return builder.String()
		}
		middle := strings.Index(remaining[start:], "](")
		if middle < 0 {
			builder.WriteString(renderInlineText(remaining))
			return builder.String()
		}
		middle += start
		end := strings.Index(remaining[middle+2:], ")")
		if end < 0 {
			builder.WriteString(renderInlineText(remaining))
			return builder.String()
		}
		end += middle + 2
		builder.WriteString(renderInlineText(remaining[:start]))
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

func renderInlineText(value string) string {
	var builder strings.Builder
	remaining := value
	open := false
	for {
		index := strings.Index(remaining, "**")
		if index < 0 {
			builder.WriteString(html.EscapeString(remaining))
			break
		}
		builder.WriteString(html.EscapeString(remaining[:index]))
		if open {
			builder.WriteString("</strong>")
		} else {
			builder.WriteString("<strong>")
		}
		open = !open
		remaining = remaining[index+2:]
	}
	if open {
		builder.WriteString("</strong>")
	}
	return builder.String()
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
