package profileupdater

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// verifiedStore knows which phone numbers passed an OTP check (Redis).
type verifiedStore interface {
	IsVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) (bool, error)
	ConsumeVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) error
}

type userGetter interface {
	Execute(ctx context.Context, id string) (domain.User, error)
}

// userUpdater is user_updater: it normalizes, validates and stores the fields.
type userUpdater interface {
	Execute(ctx context.Context, id string, upd domain.UserUpdate) (domain.User, error)
}

// UseCase lets the signed-in user edit their own profile.
type UseCase struct {
	l        logger.Logger
	verified verifiedStore
	users    userGetter
	updater  userUpdater
}

// New creates a new profile updater use case.
func New(l logger.Logger, verified verifiedStore, users userGetter, updater userUpdater) *UseCase {
	return &UseCase{l: l, verified: verified, users: users, updater: updater}
}

// Execute applies upd to the user, with the same rules as the admin update. A new phone number
// must have passed an update_user OTP check first; that verification is used up by the change.
// Sending the current phone number is not a change and needs no check.
func (uc *UseCase) Execute(ctx context.Context, userID string, upd domain.UserUpdate) (domain.User, error) {
	if upd.IsEmpty() {
		return domain.User{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	var newPhone string

	if upd.PhoneNumber != nil {
		phone := strings.TrimSpace(*upd.PhoneNumber)

		current, err := uc.users.Execute(ctx, userID)
		if err != nil {
			return domain.User{}, err
		}

		if current.PhoneNumber != nil && *current.PhoneNumber == phone {
			upd.PhoneNumber = nil
			if upd.IsEmpty() {
				return current, nil
			}
		} else {
			if err := uc.checkVerified(ctx, phone); err != nil {
				return domain.User{}, err
			}

			newPhone, upd.PhoneNumber = phone, &phone
		}
	}

	updated, err := uc.updater.Execute(ctx, userID, upd)
	if err != nil {
		return domain.User{}, err
	}

	l := logger.WithContext(uc.l, ctx)

	if newPhone != "" {
		// The number is the user's now; a leftover mark could only be used to set it again.
		if err := uc.verified.ConsumeVerified(context.WithoutCancel(ctx), newPhone, domain.OTPPurposeUpdateUser); err != nil {
			l.Error("verified.ConsumeVerified", zap.Error(err))
		}
	}

	l.Info("profile updated", zap.String("user_id", userID), zap.Bool("phone_changed", newPhone != ""))

	return updated, nil
}

func (uc *UseCase) checkVerified(ctx context.Context, phone string) error {
	if !domain.IsUzbekPhoneNumber(phone) {
		return errs.Errf(errs.ErrValidation, "phone_number must be an Uzbek number in E.164 format, e.g. +998901234567")
	}

	ok, err := uc.verified.IsVerified(ctx, phone, domain.OTPPurposeUpdateUser)
	if err != nil {
		return err
	}

	if !ok {
		return errs.ErrNewPhoneNotVerified
	}

	return nil
}
