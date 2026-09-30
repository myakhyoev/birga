package errs

import (
	"errors"
	"fmt"
)

// Error is an application error. It keeps a parent sentinel (see list.go)
// reachable through errors.Is while carrying a more specific message.
type Error struct {
	err error
	msg string
}

func (e *Error) Error() string {
	if e.msg != "" {
		return e.msg
	}

	return e.err.Error()
}

func (e *Error) Unwrap() error {
	return e.err
}

// New creates a sentinel error.
func New(text string) *Error {
	return &Error{err: errors.New(text), msg: ""}
}

// Errf creates an error that matches err via errors.Is with a formatted message.
func Errf(err error, tmpl string, args ...any) error {
	return &Error{err: err, msg: fmt.Sprintf(tmpl, args...)}
}

// Fallback returns err unchanged if it is already an *Error, otherwise wraps
// it under fallback so the transport layer can classify it.
func Fallback(err error, fallback *Error) error {
	if err == nil {
		return nil
	}

	var customErr *Error
	if errors.As(err, &customErr) {
		return err
	}

	return Errf(fallback, "%s", err.Error())
}

// Wrap classifies unknown errors as ErrInternal.
func Wrap(err error) error {
	return Fallback(err, ErrInternal)
}
