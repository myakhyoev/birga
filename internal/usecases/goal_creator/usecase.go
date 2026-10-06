package goalcreator

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type goalRepo interface {
	Create(ctx context.Context, g domain.Goal) (domain.Goal, error)
}

// UseCase validates and stores a new goal.
type UseCase struct {
	l    logger.Logger
	repo goalRepo
}

// New creates a new goal creator use case.
func New(l logger.Logger, repo goalRepo) *UseCase {
	return &UseCase{
		l:    l,
		repo: repo,
	}
}

// Execute trims the names, requires all three and stores the goal. A name already used by an active
// goal in the same language (ignoring case) is ErrGoalNameTaken.
func (uc *UseCase) Execute(ctx context.Context, g domain.Goal) (domain.Goal, error) {
	g.NameUz = strings.TrimSpace(g.NameUz)
	g.NameRu = strings.TrimSpace(g.NameRu)
	g.NameEn = strings.TrimSpace(g.NameEn)

	if !domain.IsValidGoalName(g.NameUz) || !domain.IsValidGoalName(g.NameRu) || !domain.IsValidGoalName(g.NameEn) {
		return domain.Goal{}, errs.Errf(errs.ErrValidation,
			"name_uz, name_ru and name_en are required, at most %d characters each", domain.MaxGoalNameLength)
	}

	created, err := uc.repo.Create(ctx, g)
	if err != nil {
		return domain.Goal{}, err
	}

	logger.WithContext(uc.l, ctx).Info("goal created", zap.String("id", created.ID))

	return created, nil
}
