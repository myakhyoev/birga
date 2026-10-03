package childgetter

import (
	"context"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

// UseCase returns one of the signed-in user's children.
type UseCase struct {
	l        logger.Logger
	children childRepo
}

// New creates a new child getter use case.
func New(l logger.Logger, children childRepo) *UseCase {
	return &UseCase{l: l, children: children}
}

// Execute returns the child if userID is one of its parents. Any other child, deleted or not, is
// errs.ErrChildNotFound, so ids of other families cannot be probed.
func (uc *UseCase) Execute(ctx context.Context, userID, childID string) (domain.Child, error) {
	return uc.children.GetForParent(ctx, childID, userID)
}
