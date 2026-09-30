// Package httplog provides an http.RoundTripper that logs outgoing requests.
package httplog

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// Option configures the transport.
type Option func(*transport)

// WithBodies logs request and response bodies (truncated). Off by default
// because bodies may contain personal data.
func WithBodies() Option {
	return func(t *transport) { t.logBodies = true }
}

type transport struct {
	next      http.RoundTripper
	logBodies bool
}

// New wraps next (http.DefaultTransport when nil) with request logging.
func New(next http.RoundTripper, opts ...Option) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}

	t := &transport{next: next}
	for _, opt := range opts {
		opt(t)
	}

	return t
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	l := logger.FromCtx(req.Context(), "http.client").With(
		zap.String("method", req.Method),
		zap.String("url", req.URL.Redacted()),
	)

	if t.logBodies && req.Body != nil && req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			raw, _ := io.ReadAll(body)
			_ = body.Close()
			l = l.With(logger.RequestDump(raw))
		}
	}

	start := time.Now()

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		l.Error("outgoing request failed", zap.Duration("latency", time.Since(start)), zap.Error(err))

		return nil, err
	}

	fields := []zap.Field{zap.Int("status", resp.StatusCode), zap.Duration("latency", time.Since(start))}

	if t.logBodies && resp.Body != nil {
		raw, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(raw))

		if readErr == nil {
			fields = append(fields, logger.ResponseDump(raw))
		}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		l.Warn("outgoing request", fields...)
	} else {
		l.Info("outgoing request", fields...)
	}

	return resp, nil
}
