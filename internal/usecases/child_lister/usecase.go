package childlister

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	List(ctx context.Context, f domain.ChildFilter) ([]domain.Child, int, error)
}

// UseCase lists the signed-in user's children.
type UseCase struct {
	l        logger.Logger
	children childRepo
}

// New creates a new child lister use case.
func New(l logger.Logger, children childRepo) *UseCase {
	return &UseCase{l: l, children: children}
}

// Execute returns one page of userID's active children, newest first, and their total number.
// f.ParentID is always set to userID, so a user only ever sees their own children.
func (uc *UseCase) Execute(ctx context.Context, userID string, f domain.ChildFilter) ([]domain.Child, int, error) {
	f.ParentID = userID

	return uc.children.List(ctx, f)
}
