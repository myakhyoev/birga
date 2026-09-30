package rest

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/pkg/logger/ginlog"
)

// resolveErrCode labels HTTP metrics with the application error code set by
// Return/fail, so dashboards can tell validation failures from internal ones.
func resolveErrCode(c *gin.Context) string {
	code, ok := c.Get(ginlog.ErrCodeKey)
	if !ok {
		return strconv.Itoa(int(_errCodeNoError))
	}

	if v, ok := code.(int); ok {
		return strconv.Itoa(v)
	}

	return ""
}
