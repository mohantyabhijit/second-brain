package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const knowledgeTracerName = "github.com/abhijitmohanty/second-brain/backend/internal/knowledge"

type traceMetadataKey struct{}

type traceMetadata struct {
	Name      string
	SessionID string
	Tags      []string
}

type observationOptions struct {
	Name          string
	Type          string
	Model         string
	PromptVersion string
	PromptName    string
	PromptNumber  int
	Tags          []string
	Metadata      map[string]string
	InputSummary  map[string]any
	InputContent  string
	ModelParams   map[string]any
}

func (s *Service) startOperationSpan(ctx context.Context, name string, tags ...string) (context.Context, oteltrace.Span) {
	return s.startOperationSpanWithSession(ctx, name, "", tags...)
}

func (s *Service) startOperationSpanWithSession(ctx context.Context, name string, sessionID string, tags ...string) (context.Context, oteltrace.Span) {
	metadata, ok := ctx.Value(traceMetadataKey{}).(traceMetadata)
	if !ok || strings.TrimSpace(metadata.Name) == "" {
		sessionID = fallback(strings.TrimSpace(sessionID), metadata.SessionID)
		metadata = traceMetadata{
			Name:      fallback(strings.TrimSpace(name), "second-brain-operation"),
			SessionID: normalizeSessionID(fallback(sessionID, operationSessionID(name))),
			Tags:      langfuseTags(tags),
		}
		ctx = context.WithValue(ctx, traceMetadataKey{}, metadata)
	}
	opts := observationOptions{
		Name: name,
		Type: "span",
		Tags: tags,
	}
	return s.startObservationSpan(ctx, opts)
}

func withTraceSession(ctx context.Context, sessionID string) context.Context {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ctx
	}
	metadata, _ := ctx.Value(traceMetadataKey{}).(traceMetadata)
	metadata.SessionID = normalizeSessionID(sessionID)
	return context.WithValue(ctx, traceMetadataKey{}, metadata)
}

func (s *Service) startObservationSpan(ctx context.Context, opts observationOptions) (context.Context, oteltrace.Span) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "second-brain-operation"
	}
	observationType := strings.TrimSpace(opts.Type)
	if observationType == "" {
		observationType = "span"
	}
	ctx, span := otel.Tracer(knowledgeTracerName).Start(ctx, name)
	trace, _ := ctx.Value(traceMetadataKey{}).(traceMetadata)
	if strings.TrimSpace(trace.Name) == "" {
		trace.Name = name
	}
	if strings.TrimSpace(trace.SessionID) == "" {
		trace.SessionID = operationSessionID(trace.Name)
	}
	trace.SessionID = normalizeSessionID(trace.SessionID)
	if len(trace.Tags) == 0 {
		trace.Tags = langfuseTags(opts.Tags)
	}
	attrs := []attribute.KeyValue{
		attribute.String("langfuse.trace.name", trace.Name),
		attribute.String("langfuse.session.id", trace.SessionID),
		attribute.String("langfuse.observation.type", observationType),
	}
	if release := strings.TrimSpace(s.cfg.AppRelease); release != "" {
		attrs = append(attrs,
			attribute.String("langfuse.release", release),
			attribute.String("langfuse.version", release),
		)
	}
	if env := strings.TrimSpace(s.cfg.Env); env != "" {
		attrs = append(attrs,
			attribute.String("langfuse.environment", env),
			attribute.String("deployment.environment", env),
		)
	}
	if ownerID := strings.TrimSpace(s.cfg.OwnerID); ownerID != "" {
		attrs = append(attrs, attribute.String("langfuse.user.id", ownerID))
	}
	if len(trace.Tags) > 0 {
		attrs = append(attrs, attribute.StringSlice("langfuse.trace.tags", trace.Tags))
	}
	if tags := compactTelemetryJSON(langfuseTags(opts.Tags)); tags != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.metadata.tags", tags))
	}
	if model := strings.TrimSpace(opts.Model); model != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.model.name", model))
	}
	if promptVersion := strings.TrimSpace(opts.PromptVersion); promptVersion != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.metadata.prompt_version", promptVersion))
	}
	if promptName := strings.TrimSpace(opts.PromptName); promptName != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.prompt.name", promptName))
	}
	if opts.PromptNumber > 0 {
		attrs = append(attrs, attribute.Int("langfuse.observation.prompt.version", opts.PromptNumber))
	}
	for key, value := range opts.Metadata {
		key = metadataKey(key)
		value = truncateTelemetryValue(value)
		if key != "" && value != "" {
			attrs = append(attrs, attribute.String("langfuse.observation.metadata."+key, value))
		}
	}
	if raw := compactTelemetryJSON(telemetryPayload(opts.InputSummary, opts.InputContent, s.cfg.LangfuseCaptureContent)); raw != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.input", raw))
	}
	if raw := compactTelemetryJSON(opts.ModelParams); raw != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.model.parameters", raw))
	}
	span.SetAttributes(attrs...)
	return ctx, span
}

func (s *Service) setSpanOutput(span oteltrace.Span, output map[string]any, content string) {
	if raw := compactTelemetryJSON(telemetryPayload(output, content, s.cfg.LangfuseCaptureContent)); raw != "" {
		span.SetAttributes(attribute.String("langfuse.observation.output", raw))
	}
}

func setSpanOutputSummary(span oteltrace.Span, output map[string]any) {
	if raw := compactTelemetryJSON(output); raw != "" {
		span.SetAttributes(attribute.String("langfuse.observation.output", raw))
	}
}

func setOpenAICost(span oteltrace.Span, totalUSD float64) {
	if totalUSD <= 0 {
		return
	}
	if raw := compactTelemetryJSON(map[string]any{"total": totalUSD}); raw != "" {
		span.SetAttributes(attribute.String("langfuse.observation.cost_details", raw))
	}
}

func setSpanError(span oteltrace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.String("langfuse.observation.level", "ERROR"),
		attribute.String("langfuse.observation.status_message", truncateTelemetryValue(err.Error())),
	)
}

func setOpenAIUsage(span oteltrace.Span, usage openAIUsage) {
	details := usageDetails(usage)
	if len(details) == 0 {
		return
	}
	if raw := compactTelemetryJSON(details); raw != "" {
		span.SetAttributes(attribute.String("langfuse.observation.usage_details", raw))
	}
}

func usageDetails(usage openAIUsage) map[string]any {
	details := map[string]any{}
	input := firstPositive(usage.InputTokens, usage.PromptTokens)
	output := firstPositive(usage.OutputTokens, usage.CompletionTokens)
	total := usage.TotalTokens
	if total == 0 && (input > 0 || output > 0) {
		total = input + output
	}
	if input > 0 {
		details["input"] = input
	}
	if output > 0 {
		details["output"] = output
	}
	if total > 0 {
		details["total"] = total
	}
	return details
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func compactTelemetryJSON(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	if string(raw) == "null" {
		return ""
	}
	return truncateTelemetryValue(string(raw))
}

func truncateTelemetryValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1000 {
		return value
	}
	return value[:1000] + "...[truncated]"
}

var (
	telemetryEmailPattern = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	telemetryURLPattern   = regexp.MustCompile(`https?://[^\s)\]}>"]+`)
	telemetryUUIDPattern  = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
)

func telemetryPayload(summary map[string]any, content string, captureContent bool) map[string]any {
	if len(summary) == 0 && (!captureContent || strings.TrimSpace(content) == "") {
		return nil
	}
	payload := make(map[string]any, len(summary)+1)
	for key, value := range summary {
		payload[key] = value
	}
	if captureContent {
		if sanitized := sanitizeTelemetryContent(content); sanitized != "" {
			payload["content"] = sanitized
		}
	}
	return payload
}

func sanitizeTelemetryContent(value string) string {
	value = telemetryEmailPattern.ReplaceAllString(value, "[redacted-email]")
	value = telemetryURLPattern.ReplaceAllString(value, "[redacted-url]")
	value = telemetryUUIDPattern.ReplaceAllString(value, "[redacted-id]")
	return truncateTelemetryValue(value)
}

func operationSessionID(name string) string {
	name = metadataKey(fallback(name, "second-brain"))
	return fmt.Sprintf("%s:%s", name, time.Now().UTC().Format("20060102T150405.000000000Z"))
}

func normalizeSessionID(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 32 && r <= 126 {
			return r
		}
		return -1
	}, strings.TrimSpace(value))
	if len(value) > 200 {
		return value[:200]
	}
	return value
}

func langfuseTags(tags []string) []string {
	seen := map[string]bool{}
	values := []string{"second-brain"}
	seen["second-brain"] = true
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		values = append(values, tag)
	}
	return values
}

func metadataKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

func sourceCandidateMetadata(candidate sourceCandidate) map[string]string {
	return map[string]string{
		"source_type":   string(candidate.sourceType),
		"artifact_kind": candidate.artifactKind,
		"content_type":  candidate.contentType,
		"external_id":   candidate.externalID,
	}
}

func sourceCandidateInputSummary(candidate sourceCandidate) map[string]any {
	return map[string]any{
		"source_type":  string(candidate.sourceType),
		"title_chars":  len(candidate.title),
		"body_chars":   len(candidate.body),
		"has_url":      strings.TrimSpace(candidate.sourceURL) != "",
		"published_at": strings.TrimSpace(candidate.publishedAt) != "",
	}
}
