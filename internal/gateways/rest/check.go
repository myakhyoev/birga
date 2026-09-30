package rest

import (
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

var uuidRegex = regexp.MustCompile(
	`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

// ValidateUUID checks if the given string is a valid UUID.
func ValidateUUID(id string) error {
	if !uuidRegex.MatchString(id) {
		return errs.Errf(errs.ErrBadRequest, "invalid UUID format")
	}

	return nil
}

// parsePagination reads ?limit and ?offset, applying defaults and the max limit.
func parsePagination(c *gin.Context) (limit, offset int, err error) {
	limit = defaultPageLimit

	if v := c.Query("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit <= 0 {
			return 0, 0, errs.Errf(errs.ErrBadRequest, "limit must be a positive integer")
		}
	}

	limit = min(limit, maxPageLimit)

	if v := c.Query("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 {
			return 0, 0, errs.Errf(errs.ErrBadRequest, "offset must be a non-negative integer")
		}
	}

	return limit, offset, nil
}

// parseOptionalInt reads an optional integer query parameter (0 when absent).
func parseOptionalInt(c *gin.Context, name string) (int, error) {
	v := c.Query(name)
	if v == "" {
		return 0, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, errs.Errf(errs.ErrBadRequest, "%s must be an integer", name)
	}

	return n, nil
}
