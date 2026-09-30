package logger

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFromCtxCarriesBoundFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	zap.ReplaceGlobals(zap.New(core))

	ctx := BindRequestID(context.Background(), "req-1")
	ctx = BindFields(ctx, zap.String("user", "u1"))

	FromCtx(ctx, "test").Info("hello")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}

	fields := entries[0].ContextMap()
	if fields["request_id"] != "req-1" || fields["user"] != "u1" {
		t.Fatalf("fields = %v", fields)
	}

	if entries[0].LoggerName != "test" {
		t.Fatalf("logger name = %q", entries[0].LoggerName)
	}
}

func TestWithContextNilLogger(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	zap.ReplaceGlobals(zap.New(core))

	WithContext(nil, BindRequestID(context.Background(), "r")).Info("x")

	if logs.Len() != 1 {
		t.Fatal("nil logger should fall back to the global one")
	}
}

func TestParseLevel(t *testing.T) {
	if parseLevel("DEBUG") != zap.DebugLevel || parseLevel("nonsense") != zap.InfoLevel {
		t.Fatal("unexpected level parsing")
	}
}
