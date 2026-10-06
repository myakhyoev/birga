package goalupdater

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type goalRepo interface {
	Update(ctx context.Context, id string, upd domain.GoalUpdate) (domain.Goal, error)
}

// UseCase renames a goal.
type UseCase struct {
	l    logger.Logger
	repo goalRepo
}

// New creates a new goal updater use case.
func New(l logger.Logger, repo goalRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute trims and checks the given names (same rules as create) and updates the goal.
func (uc *UseCase) Execute(ctx context.Context, id string, upd domain.GoalUpdate) (domain.Goal, error) {
	if upd.IsEmpty() {
		return domain.Goal{}, errs.Errf(errs.ErrValidation, "nothing to update")
	}

	for _, s := range []**string{&upd.NameUz, &upd.NameRu, &upd.NameEn} {
		if *s == nil {
			continue
		}

		v := strings.TrimSpace(**s)
		if !domain.IsValidGoalName(v) {
			return domain.Goal{}, errs.Errf(errs.ErrValidation,
				"name_uz, name_ru and name_en cannot be empty, at most %d characters each", domain.MaxGoalNameLength)
		}

		*s = &v
	}

	updated, err := uc.repo.Update(ctx, id, upd)
	if err != nil {
		return domain.Goal{}, err
	}

	logger.WithContext(uc.l, ctx).Info("goal updated", zap.String("id", id))

	return updated, nil
}
