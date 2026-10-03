package childcreator

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	Create(ctx context.Context, c domain.Child) (domain.Child, error)
	AddParent(ctx context.Context, childID, userID string) error
}

type txRunner interface {
	InTx(ctx context.Context, h func(context.Context) error) error
}

// UseCase adds a child profile for the signed-in user.
type UseCase struct {
	l        logger.Logger
	tx       txRunner
	children childRepo
}

// New creates a new child creator use case.
func New(l logger.Logger, tx txRunner, children childRepo) *UseCase {
	return &UseCase{l: l, tx: tx, children: children}
}

// Execute validates the child, then in one transaction stores it in children and links it to
// userID in user_children, so a child never exists without a parent.
func (uc *UseCase) Execute(ctx context.Context, userID string, c domain.Child) (domain.Child, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Gender = strings.ToLower(strings.TrimSpace(c.Gender))

	if c.PhotoID != nil {
		if v := strings.TrimSpace(*c.PhotoID); v == "" {
			c.PhotoID = nil
		} else {
			c.PhotoID = &v
		}
	}

	if err := validate(c); err != nil {
		return domain.Child{}, err
	}

	var created domain.Child

	err := uc.tx.InTx(ctx, func(ctx context.Context) error {
		var err error

		created, err = uc.children.Create(ctx, c)
		if err != nil {
			return err
		}

		return uc.children.AddParent(ctx, created.ID, userID)
	})
	if err != nil {
		return domain.Child{}, err
	}

	logger.WithContext(uc.l, ctx).Info("child created", zap.String("id", created.ID), zap.String("user_id", userID))

	return created, nil
}

func validate(c domain.Child) error {
	switch {
	case !domain.IsValidChildName(c.Name):
		return errs.Errf(errs.ErrValidation, "name is required, at most %d characters", domain.MaxChildNameLength)
	case !domain.IsValidChildAge(c.Age):
		return errs.Errf(errs.ErrValidation, "age must be between 0 and %d", domain.MaxChildProfileAge)
	case !domain.IsKnownGender(c.Gender):
		return errs.Errf(errs.ErrValidation, "gender must be %s or %s", domain.GenderMale, domain.GenderFemale)
	case c.PhotoID != nil && uuid.Validate(*c.PhotoID) != nil:
		return errs.Errf(errs.ErrValidation, "photo_id must be a UUID")
	}

	return nil
}
