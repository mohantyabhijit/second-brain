package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	ServiceName       string
	Environment       string
	Level             string
	Writer            io.Writer
	IncludeErrorStack bool
	DebugSampleRate   int
}

type Logger struct {
	delegate          zerolog.Logger
	includeErrorStack bool
	debugSampleRate   uint64
	debugCounter      atomic.Uint64
}

type contextKey string

const (
	loggerContextKey   contextKey = "second_brain_logger"
	requestMetadataKey contextKey = "second_brain_request_metadata"
	defaultServiceName            = "app"
	defaultEnvironment            = "development"
	headerRequestID               = "X-Request-ID"
)

type RequestMetadata struct {
	RequestID string
	TraceID   string
	UserID    string
}

var defaultLogger atomic.Value

func init() {
	zerolog.TimestampFieldName = "timestamp"
	zerolog.LevelFieldName = "log_level"
	zerolog.MessageFieldName = "message"
	zerolog.ErrorFieldName = "error"
	zerolog.TimeFieldFormat = time.RFC3339Nano
}

func NewForConfig(serviceName string, cfg config.Config) *Logger {
	return New(Options{
		ServiceName:       serviceName,
		Environment:       cfg.Env,
		Level:             cfg.LogLevel,
		IncludeErrorStack: cfg.LogErrorStack,
		DebugSampleRate:   cfg.LogDebugSampleRate,
	})
}

func New(options Options) *Logger {
	return newLogger(options, true)
}

func Discard() *Logger {
	return newLogger(Options{
		ServiceName: "test",
		Environment: "test",
		Writer:      io.Discard,
		Level:       zerolog.Disabled.String(),
	}, false)
}

func newLogger(options Options, setDefault bool) *Logger {
	writer := options.Writer
	if writer == nil {
		writer = os.Stdout
	}
	level, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(options.Level)))
	if err != nil {
		level = zerolog.InfoLevel
	}
	serviceName := fallback(options.ServiceName, defaultServiceName)
	environment := fallback(options.Environment, defaultEnvironment)
	delegate := zerolog.New(writer).
		Level(level).
		With().
		Timestamp().
		Str("service_name", serviceName).
		Str("environment", environment).
		Logger()
	logger := &Logger{
		delegate:          delegate,
		includeErrorStack: options.IncludeErrorStack,
		debugSampleRate:   normalizeSampleRate(options.DebugSampleRate),
	}
	if setDefault {
		SetDefault(logger)
	}
	return logger
}

func SetDefault(logger *Logger) {
	if logger == nil {
		return
	}
	defaultLogger.Store(logger)
}

func Default() *Logger {
	if value := defaultLogger.Load(); value != nil {
		if logger, ok := value.(*Logger); ok {
			return logger
		}
	}
	return New(Options{})
}

func WithContext(ctx context.Context, logger *Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = Default()
	}
	return context.WithValue(ctx, loggerContextKey, logger)
}

func FromContext(ctx context.Context, fallbackLogger ...*Logger) *Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(loggerContextKey).(*Logger); ok && logger != nil {
			return logger
		}
	}
	for _, logger := range fallbackLogger {
		if logger != nil {
			return logger
		}
	}
	return Default()
}

func Info(ctx context.Context, message string, fields ...any) {
	FromContext(ctx).InfoContext(ctx, message, fields...)
}

func Warn(ctx context.Context, message string, fields ...any) {
	FromContext(ctx).WarnContext(ctx, message, fields...)
}

func Error(ctx context.Context, message string, fields ...any) {
	FromContext(ctx).ErrorContext(ctx, message, fields...)
}

func Debug(ctx context.Context, message string, fields ...any) {
	FromContext(ctx).DebugContext(ctx, message, fields...)
}

func (l *Logger) Info(message string, fields ...any) {
	l.log(context.Background(), zerolog.InfoLevel, message, fields...)
}

func (l *Logger) Warn(message string, fields ...any) {
	l.log(context.Background(), zerolog.WarnLevel, message, fields...)
}

func (l *Logger) Error(message string, fields ...any) {
	l.log(context.Background(), zerolog.ErrorLevel, message, fields...)
}

func (l *Logger) Debug(message string, fields ...any) {
	l.log(context.Background(), zerolog.DebugLevel, message, fields...)
}

func (l *Logger) InfoContext(ctx context.Context, message string, fields ...any) {
	l.log(ctx, zerolog.InfoLevel, message, fields...)
}

func (l *Logger) WarnContext(ctx context.Context, message string, fields ...any) {
	l.log(ctx, zerolog.WarnLevel, message, fields...)
}

func (l *Logger) ErrorContext(ctx context.Context, message string, fields ...any) {
	l.log(ctx, zerolog.ErrorLevel, message, fields...)
}

func (l *Logger) DebugContext(ctx context.Context, message string, fields ...any) {
	if l == nil || !l.shouldEmitDebug() {
		return
	}
	l.log(ctx, zerolog.DebugLevel, message, fields...)
}

func (l *Logger) With(fields ...any) *Logger {
	if l == nil {
		l = Default()
	}
	contextBuilder := l.delegate.With()
	addFields(contextBuilderLogger{builder: &contextBuilder}, false, fields...)
	return &Logger{
		delegate:          contextBuilder.Logger(),
		includeErrorStack: l.includeErrorStack,
		debugSampleRate:   l.debugSampleRate,
	}
}

func (l *Logger) Zerolog() zerolog.Logger {
	if l == nil {
		return Default().delegate
	}
	return l.delegate
}

func (l *Logger) log(ctx context.Context, level zerolog.Level, message string, fields ...any) {
	if l == nil {
		l = Default()
	}
	if level == zerolog.DebugLevel && !l.shouldEmitDebug() {
		return
	}
	event := l.delegate.WithLevel(level)
	addContextFields(ctx, event)
	hasError := addEventFields(event, fields...)
	if level >= zerolog.ErrorLevel && l.includeErrorStack && hasError {
		event.Str("stack", string(debug.Stack()))
	}
	event.Msg(message)
}

func (l *Logger) shouldEmitDebug() bool {
	if l == nil {
		return false
	}
	if l.debugSampleRate <= 1 {
		return true
	}
	return l.debugCounter.Add(1)%l.debugSampleRate == 0
}

func WithRequestMetadata(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	metadata := &RequestMetadata{RequestID: strings.TrimSpace(requestID)}
	return context.WithValue(ctx, requestMetadataKey, metadata)
}

func RequestMetadataFromContext(ctx context.Context) *RequestMetadata {
	if ctx == nil {
		return nil
	}
	metadata, _ := ctx.Value(requestMetadataKey).(*RequestMetadata)
	return metadata
}

func SetUserID(ctx context.Context, userID string) {
	if metadata := RequestMetadataFromContext(ctx); metadata != nil {
		metadata.UserID = strings.TrimSpace(userID)
	}
}

func SetTraceID(ctx context.Context, traceID string) {
	if metadata := RequestMetadataFromContext(ctx); metadata != nil {
		metadata.TraceID = strings.TrimSpace(traceID)
	}
}

func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
		return spanContext.TraceID().String()
	}
	if metadata := RequestMetadataFromContext(ctx); metadata != nil {
		return metadata.TraceID
	}
	return ""
}

func RequestIDFromHeaders(headers http.Header) string {
	for _, key := range []string{headerRequestID, "X-Request-Id", "X-Correlation-ID", "Traceparent"} {
		if value := strings.TrimSpace(headers.Get(key)); value != "" {
			if key == "Traceparent" {
				continue
			}
			return value
		}
	}
	return newRequestID()
}

func addContextFields(ctx context.Context, event *zerolog.Event) {
	if event == nil || ctx == nil {
		return
	}
	traceID := ""
	if metadata := RequestMetadataFromContext(ctx); metadata != nil {
		if metadata.RequestID != "" {
			event.Str("request_id", metadata.RequestID)
		}
		if metadata.UserID != "" {
			event.Str("user_id", metadata.UserID)
		}
		if metadata.TraceID != "" {
			traceID = metadata.TraceID
		}
	}
	if spanTraceID := TraceID(ctx); spanTraceID != "" {
		traceID = spanTraceID
	}
	if traceID != "" {
		event.Str("trace_id", traceID)
	}
}

func addEventFields(event *zerolog.Event, fields ...any) bool {
	if event == nil {
		return false
	}
	hasError := false
	for index := 0; index < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if !ok || strings.TrimSpace(key) == "" {
			event.Interface("field", fields[index])
			if index+1 < len(fields) {
				event.Interface("value", fields[index+1])
			}
			continue
		}
		if index+1 >= len(fields) {
			event.Str(safeKey(key), "<missing>")
			continue
		}
		if addField(event, key, fields[index+1]) {
			hasError = true
		}
	}
	return hasError
}

func addField(event *zerolog.Event, key string, value any) bool {
	key = safeKey(key)
	if shouldRedact(key) {
		event.Str(key, "[REDACTED]")
		return false
	}
	if value == nil {
		event.Interface(key, nil)
		return false
	}
	switch typed := value.(type) {
	case error:
		event.Err(typed)
		return typed != nil
	case string:
		event.Str(key, typed)
	case bool:
		event.Bool(key, typed)
	case int:
		event.Int(key, typed)
	case int8:
		event.Int8(key, typed)
	case int16:
		event.Int16(key, typed)
	case int32:
		event.Int32(key, typed)
	case int64:
		event.Int64(key, typed)
	case uint:
		event.Uint(key, typed)
	case uint8:
		event.Uint8(key, typed)
	case uint16:
		event.Uint16(key, typed)
	case uint32:
		event.Uint32(key, typed)
	case uint64:
		event.Uint64(key, typed)
	case float32:
		event.Float32(key, typed)
	case float64:
		event.Float64(key, typed)
	case time.Duration:
		event.Int64(key, typed.Milliseconds())
	case time.Time:
		event.Time(key, typed)
	case []string:
		event.Strs(key, typed)
	default:
		event.Interface(key, typed)
	}
	return false
}

type contextBuilderLogger struct {
	builder *zerolog.Context
}

func (l contextBuilderLogger) add(key string, value any) {
	if l.builder == nil || shouldRedact(key) {
		return
	}
	key = safeKey(key)
	switch typed := value.(type) {
	case string:
		*l.builder = l.builder.Str(key, typed)
	case bool:
		*l.builder = l.builder.Bool(key, typed)
	case int:
		*l.builder = l.builder.Int(key, typed)
	case int64:
		*l.builder = l.builder.Int64(key, typed)
	case time.Time:
		*l.builder = l.builder.Time(key, typed)
	default:
		*l.builder = l.builder.Interface(key, typed)
	}
}

func addFields(logger contextBuilderLogger, _ bool, fields ...any) {
	for index := 0; index < len(fields); index += 2 {
		key, ok := fields[index].(string)
		if !ok || strings.TrimSpace(key) == "" || index+1 >= len(fields) {
			continue
		}
		logger.add(key, fields[index+1])
	}
}

func shouldRedact(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"password", "token", "secret", "authorization", "cookie", "api_key", "apikey", "credential"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func safeKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, " ", "_")
	if key == "" {
		return "field"
	}
	return key
}

func fallback(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}

func normalizeSampleRate(value int) uint64 {
	if value <= 1 {
		return 1
	}
	return uint64(value)
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
