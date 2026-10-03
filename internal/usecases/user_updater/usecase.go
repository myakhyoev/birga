package userupdater

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type userRepo interface {
	Update(ctx context.Context, id string, upd domain.UserUpdate) (domain.User, error)
}

// UseCase applies a partial update to a user.
type UseCase struct {
	l    logger.Logger
	repo userRepo
}

// New creates a new user updater use case.
func New(l logger.Logger, repo userRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute normalizes and validates the given fields and updates the user.
// An empty name, username or photo_id clears it; phone_number cannot be cleared.
func (uc *UseCase) Execute(ctx context.Context, id string, upd domain.UserUpdate) (domain.User, error) {
	if upd.IsEmpty() {
		return domain.User{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	upd.Name = trimmed(upd.Name, false)
	upd.Username = trimmed(upd.Username, true)
	upd.PhoneNumber = trimmed(upd.PhoneNumber, false)
	upd.PhotoID = trimmed(upd.PhotoID, false)

	if err := validate(upd); err != nil {
		return domain.User{}, err
	}

	updated, err := uc.repo.Update(ctx, id, upd)
	if err != nil {
		return domain.User{}, err
	}

	logger.WithContext(uc.l, ctx).Info("user updated", zap.String("id", id))

	return updated, nil
}

// trimmed trims s (and lowercases it if lower), keeping nil as nil.
func trimmed(s *string, lower bool) *string {
	if s == nil {
		return nil
	}

	v := strings.TrimSpace(*s)
	if lower {
		v = strings.ToLower(v)
	}

	return &v
}

func validate(upd domain.UserUpdate) error {
	switch {
	case upd.PhoneNumber != nil && !domain.IsValidPhoneNumber(*upd.PhoneNumber):
		return errs.Errf(errs.ErrValidation, "phone_number must be in E.164 format, e.g. +998901234567")
	case upd.Username != nil && *upd.Username != "" && !domain.IsValidUsername(*upd.Username):
		return errs.Errf(errs.ErrValidation, "username must be 3 to 32 characters: latin letters, digits, '_' or '.'")
	case upd.Name != nil && !domain.IsValidUserName(*upd.Name):
		return errs.Errf(errs.ErrValidation, "name must be at most %d characters", domain.MaxUserNameLength)
	case upd.PhotoID != nil && *upd.PhotoID != "" && uuid.Validate(*upd.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a UUID")
	}

	return nil
}
