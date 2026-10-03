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

// otpStore keeps codes and the rate limiter state (Redis).
type otpStore interface {
	AcquireSend(ctx context.Context, phone string, purpose domain.OTPPurpose, ip string, lim domain.OTPLimits) error
	ReleaseSend(ctx context.Context, phone string, purpose domain.OTPPurpose, ip string) error
	SaveCode(ctx context.Context, phone string, purpose domain.OTPPurpose, hash string, ttl time.Duration) error
}

type userRepo interface {
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
}

type smsSender interface {
	Send(ctx context.Context, messageID, phone, text string) error
}

// UseCase rate-limits the request, generates a one-time code, stores its hash and sends it by SMS.
type UseCase struct {
	l     logger.Logger
	cfg   config.OTPConfig
	otps  otpStore
	users userRepo
	sms   smsSender
}

// New creates a new OTP sender use case.
func New(l logger.Logger, cfg config.OTPConfig, otps otpStore, users userRepo, sms smsSender) *UseCase {
	return &UseCase{
		l:     l,
		cfg:   cfg,
		otps:  otps,
		users: users,
		sms:   sms,
	}
}

// Execute validates the request, applies the rate limits, stores the code and sends the SMS.
// If the SMS fails the code is dropped and the send is not counted against the limits.
func (uc *UseCase) Execute(ctx context.Context, req domain.OTPSendRequest) (domain.OTPSendResult, error) {
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)

	ip, err := validate(req)
	if err != nil {
		return domain.OTPSendResult{}, err
	}

	req.IPAddress = ip

	if req.Purpose == domain.OTPPurposeSignUp {
		exists, err := uc.users.ExistsByPhone(ctx, req.PhoneNumber)
		if err != nil {
			return domain.OTPSendResult{}, err
		}

		if exists {
			return domain.OTPSendResult{}, errs.ErrPhoneNumberRegistered
		}
	}

	limits := domain.OTPLimits{
		ResendCooldown: uc.cfg.ResendCooldown,
		Window:         limitWindow,
		MaxPerPhone:    uc.cfg.MaxPerPhoneHour,
		MaxPerIP:       uc.cfg.MaxPerIPHour,
	}
	if err := uc.otps.AcquireSend(ctx, req.PhoneNumber, req.Purpose, req.IPAddress, limits); err != nil {
		return domain.OTPSendResult{}, err
	}

	messageID := uuid.NewString()
	l := logger.WithContext(uc.l, ctx).With(zap.String("message_id", messageID), zap.String("purpose", string(req.Purpose)))

	if err := uc.saveAndSend(ctx, req, messageID); err != nil {
		if relErr := uc.otps.ReleaseSend(context.WithoutCancel(ctx), req.PhoneNumber, req.Purpose, req.IPAddress); relErr != nil {
			l.Error("otps.ReleaseSend after failed send", zap.Error(relErr))
		}

		return domain.OTPSendResult{}, err
	}

	l.Info("otp sent")

	return domain.OTPSendResult{ExpiresIn: uc.cfg.TTL, ResendIn: uc.cfg.ResendCooldown}, nil
}

func (uc *UseCase) saveAndSend(ctx context.Context, req domain.OTPSendRequest, messageID string) error {
	code, err := generateCode()
	if err != nil {
		return errs.Wrap(err)
	}

	hash := domain.HashOTPCode(req.PhoneNumber, req.Purpose, code)
	if err := uc.otps.SaveCode(ctx, req.PhoneNumber, req.Purpose, hash, uc.cfg.TTL); err != nil {
		return err
	}

	return uc.sms.Send(ctx, messageID, req.PhoneNumber, smsText(code))
}

// validate checks the request and returns the IP address in canonical form.
func validate(req domain.OTPSendRequest) (string, error) {
	switch {
	case req.PhoneNumber == "":
		return "", errs.Errf(errs.ErrValidation, "phone_number is required")
	case !domain.IsUzbekPhoneNumber(req.PhoneNumber):
		return "", errs.Errf(errs.ErrValidation, "phone_number must be an Uzbek number in E.164 format, e.g. +998901234567")
	case !req.Purpose.IsKnown():
		return "", errs.Errf(errs.ErrValidation, "purpose must be %q or %q", domain.OTPPurposeSignUp, domain.OTPPurposeUpdateUser)
	case strings.TrimSpace(req.IPAddress) == "":
		return "", errs.Errf(errs.ErrValidation, "ip_address is required")
	}

	ip := net.ParseIP(strings.TrimSpace(req.IPAddress))
	if ip == nil {
		return "", errs.Errf(errs.ErrValidation, "ip_address must be an IPv4 or IPv6 address")
	}

	return ip.String(), nil
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
