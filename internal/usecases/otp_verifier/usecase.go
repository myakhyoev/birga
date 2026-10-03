package otpverifier

import (
	"context"
	"crypto/subtle"
	"strings"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type otpStore interface {
	VerifyCode(
		ctx context.Context, phone string, purpose domain.OTPPurpose, hash string, maxAttempts int, verifiedTTL time.Duration,
	) (domain.OTPVerifyOutcome, int, error)
	MarkVerified(ctx context.Context, phone string, purpose domain.OTPPurpose, verifiedTTL time.Duration) error
}

// UseCase checks a one-time code against the one sent by otp_sender.
type UseCase struct {
	l    logger.Logger
	cfg  config.OTPConfig
	otps otpStore
}

// New creates a new OTP verifier use case.
func New(l logger.Logger, cfg config.OTPConfig, otps otpStore) *UseCase {
	return &UseCase{l: l, cfg: cfg, otps: otps}
}

// Execute returns nil when the code matches; the code is then deleted and cannot be used again,
// and the phone stays verified for the purpose for OTP_VERIFIED_TTL (sign_up needs it).
// A wrong code uses one of OTP_MAX_VERIFY_ATTEMPTS; after the last one the code is deleted too.
// OTP_DEFAULT_CODE, when set, is accepted without looking at the stored code.
func (uc *UseCase) Execute(ctx context.Context, req domain.OTPVerifyRequest) error {
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.Code = strings.TrimSpace(req.Code)

	if err := validate(req); err != nil {
		return err
	}

	if uc.isDefaultCode(req.Code) {
		if err := uc.otps.MarkVerified(ctx, req.PhoneNumber, req.Purpose, uc.cfg.VerifiedTTL); err != nil {
			return err
		}

		logger.WithContext(uc.l, ctx).Warn("otp verified with the default code", zap.String("purpose", string(req.Purpose)))

		return nil
	}

	hash := domain.HashOTPCode(req.PhoneNumber, req.Purpose, req.Code)

	outcome, left, err := uc.otps.VerifyCode(ctx, req.PhoneNumber, req.Purpose, hash, uc.cfg.MaxVerifyAttempts, uc.cfg.VerifiedTTL)
	if err != nil {
		return err
	}

	switch outcome {
	case domain.OTPMatched:
		logger.WithContext(uc.l, ctx).Info("otp verified", zap.String("purpose", string(req.Purpose)))

		return nil
	case domain.OTPMismatch:
		return errs.Errf(errs.ErrValidation, "wrong code, %d attempts left", left)
	case domain.OTPNotFound:
		return errs.ErrOTPNotFound
	case domain.OTPTooManyTries:
		return errs.ErrOTPTooManyAttempts
	default:
		return errs.Errf(errs.ErrInternal, "unexpected otp verify outcome %d", outcome)
	}
}

// isDefaultCode reports whether OTP_DEFAULT_CODE is set and code equals it.
func (uc *UseCase) isDefaultCode(code string) bool {
	return uc.cfg.DefaultCode != "" && subtle.ConstantTimeCompare([]byte(code), []byte(uc.cfg.DefaultCode)) == 1
}

func validate(req domain.OTPVerifyRequest) error {
	switch {
	case !domain.IsUzbekPhoneNumber(req.PhoneNumber):
		return errs.Errf(errs.ErrValidation, "phone_number must be an Uzbek number in E.164 format, e.g. +998901234567")
	case !req.Purpose.IsKnown():
		return errs.Errf(errs.ErrValidation, "purpose must be %q or %q", domain.OTPPurposeSignUp, domain.OTPPurposeUpdateUser)
	case !domain.IsValidOTPCode(req.Code):
		return errs.Errf(errs.ErrValidation, "code must be %d digits", domain.OTPCodeLength)
	}

	return nil
}
