// Package ginlog provides gin middlewares for request logging and panic recovery.
package ginlog

import (
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// Keys handlers may set on *gin.Context so the access log (and metrics) can
// report the application-level error code and note.
const (
	ErrCodeKey = "ginlog.err_code"
	ErrNoteKey = "ginlog.err_note"
)

// RequestIDHeader is read from incoming requests and echoed in responses.
const RequestIDHeader = "X-Request-Id"

// Resolver extracts extra log fields from a finished request.
type Resolver func(c *gin.Context) []zap.Field

// DefaultResolver adds the application error code/note set by handlers.
func DefaultResolver(c *gin.Context) []zap.Field {
	var fields []zap.Field

	if code, ok := c.Get(ErrCodeKey); ok {
		fields = append(fields, zap.Any("error_code", code))
	}

	if note := c.GetString(ErrNoteKey); note != "" {
		fields = append(fields, zap.String("error_note", note))
	}

	return fields
}

// RequestID makes sure each request has an id, binds it to the request
// context for logger.FromCtx and returns it in the response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}

		c.Header(RequestIDHeader, id)
		c.Request = c.Request.WithContext(logger.BindRequestID(c.Request.Context(), id))

		c.Next()
	}
}

// LogExcept writes one access log line per request, except for routes whose
// template matches one of skipPaths (e.g. "/swagger/*any").
func LogExcept(skipPaths []string, resolvers ...Resolver) gin.HandlerFunc {
	skip := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = struct{}{}
	}

	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		route := c.FullPath()
		if _, ok := skip[route]; ok {
			return
		}

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("route", route),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", status),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Duration("latency", time.Since(start)),
		}

		for _, r := range resolvers {
			fields = append(fields, r(c)...)
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("gin_errors", strings.TrimSpace(c.Errors.String())))
		}

		l := logger.FromCtx(c.Request.Context(), "http")

		switch {
		case status >= http.StatusInternalServerError:
			l.Error("request", fields...)
		case status >= http.StatusBadRequest:
			l.Warn("request", fields...)
		default:
			l.Info("request", fields...)
		}
	}
}

// Recovery turns panics into a 500 response and logs them with the stack.
func Recovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			fields := []zap.Field{
				zap.Any("panic", rec),
				zap.String("method", c.Request.Method),
				zap.String("path", c.Request.URL.Path),
			}
			if stack {
				fields = append(fields, zap.ByteString("stack", debug.Stack()))
			}

			logger.FromCtx(c.Request.Context(), "http.recovery").Error("panic recovered", fields...)

			c.AbortWithStatus(http.StatusInternalServerError)
		}()

		c.Next()
	}
}
