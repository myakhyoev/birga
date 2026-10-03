package rest

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const (
	adminKeyHeader      = "X-Admin-Key"
	authorizationHeader = "Authorization"

	// userIDKey holds the signed-in user's id in the gin context, set by userAuth.
	userIDKey = "user_id"
)

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

// userAuth requires a current access token in Authorization: Bearer <token> and stores the user's id
// in the gin context (see currentUserID).
func (s *Server) userAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader(authorizationHeader), "Bearer ")
		if !ok {
			Return(c, nil, errs.Errf(errs.ErrUnauthorized, "missing access token, send Authorization: Bearer <token>"))

			return
		}

		userID, err := s.tokenChecker.Execute(c.Request.Context(), token)
		if err != nil {
			Return(c, nil, err)

			return
		}

		c.Set(userIDKey, userID)
		c.Next()
	}
}

// currentUserID returns the id userAuth stored; it is empty on routes without userAuth.
func currentUserID(c *gin.Context) string {
	return c.GetString(userIDKey)
}
