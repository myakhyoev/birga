package errs

var (
	ErrValidation   = New("validation error")
	ErrBadRequest   = New("bad request")
	ErrUnauthorized = New("unauthorized")
	ErrForbidden    = New("forbidden")
	ErrNotFound     = New("not found")
	ErrConflict     = New("conflict")
	ErrInternal     = New("internal error")
	ErrConnection   = New("connection error")
	ErrRateLimited  = New("too many requests")

	ErrActivityNotFound = Errf(ErrNotFound, "activity not found")

	ErrUserNotFound     = Errf(ErrNotFound, "user not found")
	ErrUsernameTaken    = Errf(ErrConflict, "username is already taken")
	ErrPhoneNumberTaken = Errf(ErrConflict, "phone number is already taken")

	ErrPhoneNumberRegistered = Errf(ErrConflict, "phone number is already registered")
	ErrOTPNotFound           = Errf(ErrNotFound, "code expired or was not requested, request a new one")
	ErrOTPTooManyAttempts    = Errf(ErrRateLimited, "too many wrong codes, request a new one")
)
