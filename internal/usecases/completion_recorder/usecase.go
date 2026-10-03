package completionrecorder

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type childRepo interface {
	GetForParent(ctx context.Context, id, userID string) (domain.Child, error)
}

type activityRepo interface {
	Get(ctx context.Context, id string) (domain.Activity, error)
}

type completionRepo interface {
	Create(ctx context.Context, c domain.Completion) (domain.Completion, bool, error)
}

// Request is one "we did it" tap from a caregiver.
type Request struct {
	UserID     string
	ChildID    string
	ActivityID string
	Note       *string
}

// UseCase records that a child completed an activity today.
type UseCase struct {
	l           logger.Logger
	children    childRepo
	activities  activityRepo
	completions completionRepo
	now         func() time.Time
}

// New creates a new completion recorder use case.
func New(l logger.Logger, children childRepo, activities activityRepo, completions completionRepo) *UseCase {
	return &UseCase{l: l, children: children, activities: activities, completions: completions, now: time.Now}
}

// Execute checks that the child is the caller's and the activity is published, then records the
// completion for today (Uzbekistan time). Repeating it the same day returns the same completion and
// replaces the note when a new one is given. A blank note is stored as no note.
func (uc *UseCase) Execute(ctx context.Context, req Request) (domain.Completion, error) {
	if req.Note != nil {
		v := strings.TrimSpace(*req.Note)
		req.Note = &v

		if v == "" {
			req.Note = nil
		} else if utf8.RuneCountInString(v) > domain.MaxCompletionNoteLength {
			return domain.Completion{}, errs.Errf(errs.ErrValidation, "note must be at most %d characters",
				domain.MaxCompletionNoteLength)
		}
	}

	if _, err := uc.children.GetForParent(ctx, req.ChildID, req.UserID); err != nil {
		return domain.Completion{}, err
	}

	a, err := uc.activities.Get(ctx, req.ActivityID)
	if err != nil {
		return domain.Completion{}, err
	}

	if !a.IsPublished {
		return domain.Completion{}, errs.ErrActivityNotFound
	}

	c, created, err := uc.completions.Create(ctx, domain.Completion{
		ChildID:     req.ChildID,
		ActivityID:  req.ActivityID,
		UserID:      &req.UserID,
		CompletedOn: domain.LocalDay(uc.now()),
		Note:        req.Note,
	})
	if err != nil {
		return domain.Completion{}, err
	}

	if created {
		logger.WithContext(uc.l, ctx).Info("activity completed",
			zap.String("child_id", c.ChildID), zap.String("activity_id", c.ActivityID))
	}

	return c, nil
}
