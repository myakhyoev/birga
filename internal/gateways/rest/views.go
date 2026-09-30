package rest

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger/ginlog"
)

type errCode int

const (
	_errCodeNoError      errCode = 0   // Success code.
	_errCodeValidation   errCode = -10 // for http 422
	_errCodeBadRequest   errCode = -11 // for http 400
	_errCodeUnauthorized errCode = -20 // for http 401
	_errCodeForbidden    errCode = -21 // for http 403
	_errCodeNotFound     errCode = -30 // for http 404
	_errCodeConflict     errCode = -40 // for http 409
	_errCodeInternalErr  errCode = -50 // for http 500
	_errCodeUnavailable  errCode = -60 // for http 503
)

type statusType string

const (
	_statusSuccess statusType = "Success"
	_statusFailure statusType = "Failure"
)

// R is the envelope of every response.
type R struct {
	Status    statusType `json:"status"`
	ErrorCode errCode    `json:"error_code"`
	ErrorNote string     `json:"error_note"`
	Data      any        `json:"data"`
}

type InternalServerErrorResponse struct {
	Status    statusType `json:"status" example:"Failure"`
	ErrorCode errCode    `json:"error_code" example:"-50"`
	ErrorNote string     `json:"error_note" example:"internal error"`
	Data      any        `json:"data"`
}

type UnprocessableContentResponse struct {
	Status    statusType `json:"status" example:"Failure"`
	ErrorCode errCode    `json:"error_code" example:"-10"`
	ErrorNote string     `json:"error_note" example:"unknown goal \"flying\""`
	Data      any        `json:"data"`
}

type BadRequestResponse struct {
	Status    statusType `json:"status" example:"Failure"`
	ErrorCode errCode    `json:"error_code" example:"-11"`
	ErrorNote string     `json:"error_note" example:"limit must be a positive integer"`
	Data      any        `json:"data"`
}

type UnauthorizedResponse struct {
	Status    statusType `json:"status" example:"Failure"`
	ErrorCode errCode    `json:"error_code" example:"-20"`
	ErrorNote string     `json:"error_note" example:"unauthorized"`
	Data      any        `json:"data"`
}

type NotFoundResponse struct {
	Status    statusType `json:"status" example:"Failure"`
	ErrorCode errCode    `json:"error_code" example:"-30"`
	ErrorNote string     `json:"error_note" example:"activity not found"`
	Data      any        `json:"data"`
}

type ServiceUnavailableResponse struct {
	Status    statusType `json:"status" example:"Failure"`
	ErrorCode errCode    `json:"error_code" example:"-60"`
	ErrorNote string     `json:"error_note" example:"database unavailable"`
	Data      any        `json:"data"`
}

// Return writes data (on success) or maps err to an HTTP status and error code.
func Return(c *gin.Context, data any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, R{
			Status:    _statusSuccess,
			ErrorCode: _errCodeNoError,
			ErrorNote: "",
			Data:      data,
		})

		return
	}

	code, r := http.StatusInternalServerError, R{
		Status:    _statusFailure,
		ErrorCode: _errCodeInternalErr,
		ErrorNote: err.Error(),
		Data:      nil,
	}

	switch {
	case errors.Is(err, errs.ErrValidation):
		code, r.ErrorCode = http.StatusUnprocessableEntity, _errCodeValidation
	case errors.Is(err, errs.ErrBadRequest):
		code, r.ErrorCode = http.StatusBadRequest, _errCodeBadRequest
	case errors.Is(err, errs.ErrUnauthorized):
		code, r.ErrorCode = http.StatusUnauthorized, _errCodeUnauthorized
	case errors.Is(err, errs.ErrForbidden):
		code, r.ErrorCode = http.StatusForbidden, _errCodeForbidden
	case errors.Is(err, errs.ErrNotFound):
		code, r.ErrorCode = http.StatusNotFound, _errCodeNotFound
	case errors.Is(err, errs.ErrConflict):
		code, r.ErrorCode = http.StatusConflict, _errCodeConflict
	default:
		// Never leak internal details (SQL errors, hostnames) to clients;
		// the access log keeps the original message.
		c.Set(ginlog.ErrNoteKey, err.Error())
		r.ErrorNote = errs.ErrInternal.Error()
	}

	abort(c, code, r)
}

// fail writes a failure response for errors detected in the transport layer.
func fail(c *gin.Context, httpCode int, code errCode, note string) {
	abort(c, httpCode, R{Status: _statusFailure, ErrorCode: code, ErrorNote: note, Data: nil})
}

func abort(c *gin.Context, httpCode int, r R) {
	c.Set(ginlog.ErrCodeKey, int(r.ErrorCode))

	if _, ok := c.Get(ginlog.ErrNoteKey); !ok {
		c.Set(ginlog.ErrNoteKey, r.ErrorNote)
	}

	c.AbortWithStatusJSON(httpCode, r)
}
