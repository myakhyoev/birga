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
	ErrNoRecommendation = Errf(ErrNotFound, "no published activity matches this child's age and the filters")

	ErrUserNotFound     = Errf(ErrNotFound, "user not found")
	ErrUsernameTaken    = Errf(ErrConflict, "username is already taken")
	ErrPhoneNumberTaken = Errf(ErrConflict, "phone number is already taken")
	ErrPhotoNotFound    = Errf(ErrValidation, "photo_id is not an uploaded media id, upload the photo to /v1/media first")

	ErrChildNotFound = Errf(ErrNotFound, "child not found")

	ErrPhoneNumberRegistered    = Errf(ErrConflict, "phone number is already registered")
	ErrPhoneNumberNotRegistered = Errf(ErrNotFound, "phone number does not belong to a user")
	ErrOTPNotFound              = Errf(ErrNotFound, "code expired or was not requested, request a new one")
	ErrOTPTooManyAttempts       = Errf(ErrRateLimited, "too many wrong codes, request a new one")

	ErrPhoneNotVerified      = Errf(ErrForbidden, "phone number is not verified, verify a sign_up code with /v1/otp/verify first")
	ErrRoleNotSelfAssignable = Errf(ErrForbidden, "user_role admin cannot be chosen at sign-up")
	ErrInvalidRefreshToken   = Errf(ErrUnauthorized, "refresh token is invalid or expired, sign in again")
	ErrRoleNotAllowed        = Errf(ErrForbidden, "your role cannot use this endpoint")

	ErrNewPhoneNotVerified = Errf(ErrForbidden, "new phone number is not verified, verify an update_user code for it first")
	ErrResetNotVerified    = Errf(ErrForbidden, "phone number is not verified, verify a reset_password code with /v1/otp/verify first")

	ErrMediaUnsupportedType = Errf(ErrValidation, "unsupported file type, upload a JPEG, PNG or WebP image")
	ErrMediaEmpty           = Errf(ErrValidation, "file is empty")
)
