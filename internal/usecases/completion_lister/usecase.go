package completionlister

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type completionRepo interface {
	List(ctx context.Context, f domain.CompletionFilter) ([]domain.Completion, int, error)
}

// UseCase lists a child's completions for one of its parents.
type UseCase struct {
	l           logger.Logger
	children    childRepo
	completions completionRepo
}

// New creates a new completion lister use case.
func New(l logger.Logger, children childRepo, completions completionRepo) *UseCase {
	return &UseCase{l: l, children: children, completions: completions}
}

// Execute returns a page of the child's completions, newest first, and the total count.
func (uc *UseCase) Execute(ctx context.Context, userID string, f domain.CompletionFilter) ([]domain.Completion, int, error) {
	if _, err := uc.children.GetForParent(ctx, f.ChildID, userID); err != nil {
		return nil, 0, err
	}

	return uc.completions.List(ctx, f)
}
