package otpsender

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// limitWindow is the window of the per-phone and per-IP limits.
const limitWindow = time.Hour

type otpRepo interface {
	LockPhone(ctx context.Context, phone string) error
	SendStats(ctx context.Context, phone string, purpose domain.OTPPurpose, ip string, since time.Time) (domain.OTPSendStats, error)
	Create(ctx context.Context, o domain.OTP) (domain.OTP, error)
	Delete(ctx context.Context, id string) error
}

type userRepo interface {
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
}

type txRunner interface {
	InTx(ctx context.Context, h func(context.Context) error) error
}

type smsSender interface {
	Send(ctx context.Context, messageID, phone, text string) error
}

// UseCase generates a one-time code, stores its hash and sends it by SMS.
type UseCase struct {
	l     logger.Logger
	cfg   config.OTPConfig
	tx    txRunner
	otps  otpRepo
	users userRepo
	sms   smsSender
	now   func() time.Time
}

// New creates a new OTP sender use case.
func New(l logger.Logger, cfg config.OTPConfig, tx txRunner, otps otpRepo, users userRepo, sms smsSender) *UseCase {
	return &UseCase{
		l:     l,
		cfg:   cfg,
		tx:    tx,
		otps:  otps,
		users: users,
		sms:   sms,
		now:   time.Now,
	}
}

// Execute validates the request, applies the rate limits, stores the code and sends the SMS.
// If the SMS fails the stored code is removed, so a failed send does not count against the limits.
func (uc *UseCase) Execute(ctx context.Context, req domain.OTPSendRequest) (domain.OTPSendResult, error) {
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.IPAddress = strings.TrimSpace(req.IPAddress)

	if err := validate(req); err != nil {
		return domain.OTPSendResult{}, err
	}

	if req.Purpose == domain.OTPPurposeSignUp {
		exists, err := uc.users.ExistsByPhone(ctx, req.PhoneNumber)
		if err != nil {
			return domain.OTPSendResult{}, err
		}

		if exists {
			return domain.OTPSendResult{}, errs.ErrPhoneNumberRegistered
		}
	}

	code, err := generateCode()
	if err != nil {
		return domain.OTPSendResult{}, errs.Wrap(err)
	}

	id, now := uuid.NewString(), uc.now()
	otp := domain.OTP{
		ID:          id,
		PhoneNumber: req.PhoneNumber,
		Purpose:     req.Purpose,
		CodeHash:    domain.HashOTPCode(id, code),
		IPAddress:   req.IPAddress,
		ExpiresAt:   now.Add(uc.cfg.TTL),
		CreatedAt:   now,
	}

	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		if err := uc.otps.LockPhone(ctx, req.PhoneNumber); err != nil {
			return err
		}

		if err := uc.checkLimits(ctx, req, now); err != nil {
			return err
		}

		_, err := uc.otps.Create(ctx, otp)

		return err
	})
	if err != nil {
		return domain.OTPSendResult{}, err
	}

	l := logger.WithContext(uc.l, ctx).With(zap.String("otp_id", id), zap.String("purpose", string(req.Purpose)))

	if err := uc.sms.Send(ctx, id, req.PhoneNumber, smsText(code)); err != nil {
		if delErr := uc.otps.Delete(context.WithoutCancel(ctx), id); delErr != nil {
			l.Error("otps.Delete after failed send", zap.Error(delErr))
		}

		return domain.OTPSendResult{}, err
	}

	l.Info("otp sent")

	return domain.OTPSendResult{ExpiresIn: uc.cfg.TTL, ResendIn: uc.cfg.ResendCooldown}, nil
}

func (uc *UseCase) checkLimits(ctx context.Context, req domain.OTPSendRequest, now time.Time) error {
	st, err := uc.otps.SendStats(ctx, req.PhoneNumber, req.Purpose, req.IPAddress, now.Add(-limitWindow))
	if err != nil {
		return err
	}

	if st.LastSentAt != nil {
		if wait := st.LastSentAt.Add(uc.cfg.ResendCooldown).Sub(now); wait > 0 {
			return errs.Errf(errs.ErrRateLimited, "a code was sent recently, request a new one in %d seconds", int(math.Ceil(wait.Seconds())))
		}
	}

	switch {
	case st.PhoneCount >= uc.cfg.MaxPerPhoneHour:
		return errs.ErrOTPPhoneLimit
	case st.IPCount >= uc.cfg.MaxPerIPHour:
		return errs.ErrOTPIPLimit
	}

	return nil
}

func validate(req domain.OTPSendRequest) error {
	switch {
	case req.PhoneNumber == "":
		return errs.Errf(errs.ErrValidation, "phone_number is required")
	case !domain.IsUzbekPhoneNumber(req.PhoneNumber):
		return errs.Errf(errs.ErrValidation, "phone_number must be an Uzbek number in E.164 format, e.g. +998901234567")
	case !req.Purpose.IsKnown():
		return errs.Errf(errs.ErrValidation, "purpose must be %q or %q", domain.OTPPurposeSignUp, domain.OTPPurposeUpdateUser)
	case req.IPAddress == "":
		return errs.Errf(errs.ErrValidation, "ip_address is required")
	case net.ParseIP(req.IPAddress) == nil:
		return errs.Errf(errs.ErrValidation, "ip_address must be an IPv4 or IPv6 address")
	}

	return nil
}

// generateCode returns a uniformly random numeric code of domain.OTPCodeLength digits.
func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(math.Pow10(domain.OTPCodeLength))))
	if err != nil {
		return "", fmt.Errorf("generate otp code: %w", err)
	}

	return fmt.Sprintf("%0*d", domain.OTPCodeLength, n), nil
}

func smsText(code string) string {
	return fmt.Sprintf("Birga: tasdiqlash kodingiz %s. Kodni hech kimga bermang.", code)
}
