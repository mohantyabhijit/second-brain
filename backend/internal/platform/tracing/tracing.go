package tracing

import (
	"context"
	"encoding/base64"
	"log/slog"
	"strings"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httpclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Shutdown func(context.Context)

const langfuseOTLPTracesPath = "/api/public/otel/v1/traces"

func StartLangfuse(ctx context.Context, cfg config.Config, serviceName string, logger *slog.Logger) (Shutdown, bool) {
	if logger == nil {
		logger = slog.Default()
	}
	if !cfg.LangfuseTracingEnabled {
		return noopShutdown, false
	}
	publicKey := strings.TrimSpace(cfg.LangfusePublicKey)
	secretKey := strings.TrimSpace(cfg.LangfuseSecretKey)
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.LangfuseBaseURL), "/")
	if baseURL == "" || (publicKey == "" || secretKey == "") && !cfg.OneCLIGateway {
		return noopShutdown, false
	}

	headers := map[string]string{
		"x-langfuse-ingestion-version": "4",
	}
	if publicKey != "" && secretKey != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
		headers["Authorization"] = "Basic " + auth
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(baseURL+langfuseOTLPTracesPath),
		otlptracehttp.WithHeaders(headers),
		otlptracehttp.WithHTTPClient(httpclient.New()),
	)
	if err != nil {
		logger.Warn("langfuse tracing disabled; exporter setup failed", "error", err)
		return noopShutdown, false
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			attribute.String("service.name", fallback(serviceName, "second-brain-backend")),
			attribute.String("deployment.environment", fallback(cfg.Env, "development")),
		),
	)
	if err != nil {
		logger.Warn("langfuse tracing resource setup failed; using default resource", "error", err)
		res = resource.Default()
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBatchTimeout(500*time.Millisecond),
		),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	logger.Info("langfuse tracing enabled", "base_url", baseURL, "service", fallback(serviceName, "second-brain-backend"))
	return func(ctx context.Context) {
		if err := provider.Shutdown(ctx); err != nil {
			logger.Warn("langfuse tracing shutdown failed", "error", err)
		}
	}, true
}

func noopShutdown(context.Context) {}

func fallback(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
