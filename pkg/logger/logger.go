// Package logger is a thin wrapper around zap that carries request-scoped
// fields (request id and any bound fields) through context.Context.
package logger

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the subset of *zap.Logger methods the application depends on.
// Accepting the interface keeps constructors mockable in tests.
type Logger interface {
	Debug(msg string, fields ...zap.Field)
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	With(fields ...zap.Field) *zap.Logger
	Named(name string) *zap.Logger
}

type ctxKey struct{}

// New builds a JSON production logger, names it after the application and
// installs it as the zap global logger so FromCtx can reach it anywhere.
func New(level, appName string, options ...zap.Option) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))
	cfg.EncoderConfig.TimeKey = "time"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.EncoderConfig.MessageKey = "message"
	cfg.DisableStacktrace = true

	l, err := cfg.Build(options...)
	if err != nil {
		// A broken logger config is a programming error; fall back instead of crashing.
		l = zap.NewExample()
	}

	l = l.Named(appName)
	zap.ReplaceGlobals(l)

	return l
}

func parseLevel(level string) zapcore.Level {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		return zapcore.InfoLevel
	}

	return lvl
}

// BindFields returns a context that carries fields; every logger obtained
// through FromCtx/WithContext from it will include them.
func BindFields(ctx context.Context, fields ...zap.Field) context.Context {
	existing := fieldsFromCtx(ctx)
	merged := make([]zap.Field, 0, len(existing)+len(fields))
	merged = append(merged, existing...)
	merged = append(merged, fields...)

	return context.WithValue(ctx, ctxKey{}, merged)
}

// BindRequestID binds the request id to the context.
func BindRequestID(ctx context.Context, requestID string) context.Context {
	return BindFields(ctx, RequestID(requestID))
}

// FromCtx returns the global logger named by namespace with context fields attached.
func FromCtx(ctx context.Context, namespace string) *zap.Logger {
	return zap.L().Named(namespace).With(fieldsFromCtx(ctx)...)
}

// WithContext attaches context fields to l. A nil l falls back to the global logger.
func WithContext(l Logger, ctx context.Context) *zap.Logger {
	if l == nil {
		return zap.L().With(fieldsFromCtx(ctx)...)
	}

	return l.With(fieldsFromCtx(ctx)...)
}

// Cleanup flushes buffered log entries. Call it on shutdown.
func Cleanup() error {
	return zap.L().Sync()
}

func fieldsFromCtx(ctx context.Context) []zap.Field {
	if ctx == nil {
		return nil
	}

	fields, _ := ctx.Value(ctxKey{}).([]zap.Field)

	return fields
}
