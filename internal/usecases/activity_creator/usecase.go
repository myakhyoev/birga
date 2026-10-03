package activitycreator

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

const maxDurationMinutes = domain.MaxActivityDurationMinutes

type activityRepo interface {
	Create(ctx context.Context, a domain.Activity) (domain.Activity, error)
}

// UseCase validates and stores a new activity.
type UseCase struct {
	l    logger.Logger
	repo activityRepo
}

// New creates a new activity creator use case.
func New(l logger.Logger, repo activityRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute validates business rules and persists the activity.
func (uc *UseCase) Execute(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	a.TitleUz = strings.TrimSpace(a.TitleUz)
	a.TitleRu = strings.TrimSpace(a.TitleRu)
	a.DescriptionUz = strings.TrimSpace(a.DescriptionUz)
	a.DescriptionRu = strings.TrimSpace(a.DescriptionRu)

	if err := validate(a); err != nil {
		return domain.Activity{}, err
	}

	created, err := uc.repo.Create(ctx, a)
	if err != nil {
		return domain.Activity{}, err
	}

	logger.WithContext(uc.l, ctx).Info("activity created", zap.String("id", created.ID))

	return created, nil
}

func validate(a domain.Activity) error {
	switch {
	case a.TitleUz == "" || a.TitleRu == "":
		return errs.Errf(errs.ErrValidation, "title_uz and title_ru are required")
	case a.DescriptionUz == "" || a.DescriptionRu == "":
		return errs.Errf(errs.ErrValidation, "description_uz and description_ru are required")
	case !domain.IsKnownGoal(a.Goal):
		return errs.Errf(errs.ErrValidation, "unknown goal %q", a.Goal)
	case a.MinAge < domain.MinChildAge || a.MaxAge > domain.MaxChildAge || a.MinAge > a.MaxAge:
		return errs.Errf(errs.ErrValidation, "age range must be within %d..%d and min_age <= max_age",
			domain.MinChildAge, domain.MaxChildAge)
	case a.DurationMinutes <= 0 || a.DurationMinutes > maxDurationMinutes:
		return errs.Errf(errs.ErrValidation, "duration_minutes must be between 1 and %d", maxDurationMinutes)
	}

	return nil
}
