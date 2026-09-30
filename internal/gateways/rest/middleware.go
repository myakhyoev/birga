package rest

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

const adminKeyHeader = "X-Admin-Key"

// adminAuth guards content-management endpoints with a shared API key. It is
// a stop-gap until real admin accounts exist; with no key configured the
// admin API is disabled entirely.
func (s *Server) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.adminKey == "" {
			fail(c, http.StatusForbidden, _errCodeForbidden, "admin API is disabled")

			return
		}

		got := c.GetHeader(adminKeyHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.adminKey)) != 1 {
			fail(c, http.StatusUnauthorized, _errCodeUnauthorized, "invalid or missing "+adminKeyHeader)

			return
		}

		c.Next()
	}
}
