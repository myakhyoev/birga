package usercreator

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
	Create(ctx context.Context, u domain.User) (domain.User, error)
}

// UseCase validates and stores a new user.
type UseCase struct {
	l    logger.Logger
	repo userRepo
}

// New creates a new user creator use case.
func New(l logger.Logger, repo userRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute normalizes and validates the user, then persists it. phone_number is
// required; the other fields are optional and empty values are stored as NULL.
func (uc *UseCase) Execute(ctx context.Context, u domain.User) (domain.User, error) {
	u.Name = trimmed(u.Name, false)
	u.Username = trimmed(u.Username, true)
	u.PhoneNumber = trimmed(u.PhoneNumber, false)
	u.PhotoID = trimmed(u.PhotoID, false)

	if err := validate(u); err != nil {
		return domain.User{}, err
	}

	created, err := uc.repo.Create(ctx, u)
	if err != nil {
		return domain.User{}, err
	}

	logger.WithContext(uc.l, ctx).Info("user created", zap.String("id", created.ID))

	return created, nil
}

// trimmed trims s (and lowercases it if lower); an empty result becomes nil.
func trimmed(s *string, lower bool) *string {
	if s == nil {
		return nil
	}

	v := strings.TrimSpace(*s)
	if lower {
		v = strings.ToLower(v)
	}

	if v == "" {
		return nil
	}

	return &v
}

func validate(u domain.User) error {
	switch {
	case u.PhoneNumber == nil:
		return errs.Errf(errs.ErrValidation, "phone_number is required")
	case !domain.IsValidPhoneNumber(*u.PhoneNumber):
		return errs.Errf(errs.ErrValidation, "phone_number must be in E.164 format, e.g. +998901234567")
	case u.Username != nil && !domain.IsValidUsername(*u.Username):
		return errs.Errf(errs.ErrValidation, "username must be 3 to 32 characters: latin letters, digits, '_' or '.'")
	case u.Name != nil && !domain.IsValidUserName(*u.Name):
		return errs.Errf(errs.ErrValidation, "name must be at most %d characters", domain.MaxUserNameLength)
	case u.PhotoID != nil && uuid.Validate(*u.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a UUID")
	}

	return nil
}
