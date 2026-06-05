package knowledge

import (
	"context"
	"encoding/json"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const knowledgeTracerName = "github.com/abhijitmohanty/second-brain/backend/internal/knowledge"

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
	ModelParams   map[string]any
}

func (s *Service) startOperationSpan(ctx context.Context, name string, tags ...string) (context.Context, oteltrace.Span) {
	opts := observationOptions{
		Name: name,
		Type: "span",
		Tags: tags,
	}
	return s.startObservationSpan(ctx, opts)
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
	attrs := []attribute.KeyValue{
		attribute.String("langfuse.trace.name", name),
		attribute.String("langfuse.observation.type", observationType),
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
	if tags := langfuseTags(opts.Tags); len(tags) > 0 {
		attrs = append(attrs, attribute.StringSlice("langfuse.trace.tags", tags))
	}
	if model := strings.TrimSpace(opts.Model); model != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.model.name", model))
	}
	if promptVersion := strings.TrimSpace(opts.PromptVersion); promptVersion != "" {
		attrs = append(attrs,
			attribute.String("langfuse.version", promptVersion),
			attribute.String("langfuse.observation.metadata.prompt_version", promptVersion),
		)
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
	if raw := compactTelemetryJSON(opts.InputSummary); raw != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.input", raw))
	}
	if raw := compactTelemetryJSON(opts.ModelParams); raw != "" {
		attrs = append(attrs, attribute.String("langfuse.observation.model.parameters", raw))
	}
	span.SetAttributes(attrs...)
	return ctx, span
}

func setSpanOutputSummary(span oteltrace.Span, output map[string]any) {
	if raw := compactTelemetryJSON(output); raw != "" {
		span.SetAttributes(attribute.String("langfuse.observation.output", raw))
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
	return truncateTelemetryValue(string(raw))
}

func truncateTelemetryValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1000 {
		return value
	}
	return value[:1000] + "...[truncated]"
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
