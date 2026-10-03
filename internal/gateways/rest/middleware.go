package rest

import (
	"crypto/subtle"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const (
	adminKeyHeader      = "X-Admin-Key"
	authorizationHeader = "Authorization"

	// userIDKey and userRoleKey hold the signed-in user's id and role in the gin context, set by userAuth.
	userIDKey   = "user_id"
	userRoleKey = "user_role"
)

// adminAuth guards content-management endpoints. A request with an Authorization header must carry
// the access token of a user whose role is admin. Otherwise the shared X-Admin-Key is checked; it
// is a stop-gap for scripts and seeding, and with no key configured only admin users get in.
func (s *Server) adminAuth() gin.HandlerFunc {
	asAdminUser := s.userAuth(domain.UserRoleAdmin)

	return func(c *gin.Context) {
		if c.GetHeader(authorizationHeader) != "" {
			asAdminUser(c)

			return
		}

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

// userAuth requires a current access token in Authorization: Bearer <token> (401 otherwise) and,
// when roles are given, a user whose current role is one of them (403 otherwise). It stores the
// user's id and role in the gin context (see currentUserID).
func (s *Server) userAuth(roles ...domain.UserRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := strings.CutPrefix(c.GetHeader(authorizationHeader), "Bearer ")
		if !ok {
			Return(c, nil, errs.Errf(errs.ErrUnauthorized, "missing access token, send Authorization: Bearer <token>"))

			return
		}

		p, err := s.tokenChecker.Execute(c.Request.Context(), token)
		if err != nil {
			Return(c, nil, err)

			return
		}

		if len(roles) > 0 && !slices.Contains(roles, p.Role) {
			Return(c, nil, errs.ErrRoleNotAllowed)

			return
		}

		c.Set(userIDKey, p.UserID)
		c.Set(userRoleKey, p.Role)
		c.Next()
	}
}

// currentUserID returns the id userAuth stored; it is empty on routes without userAuth.
func currentUserID(c *gin.Context) string {
	return c.GetString(userIDKey)
}
