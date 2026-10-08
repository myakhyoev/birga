package activitylister

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	items []domain.Activity
	total int
	err   error
	got   domain.ActivityFilter
}

func (m *mockRepo) List(_ context.Context, f domain.ActivityFilter) ([]domain.Activity, int, error) {
	m.got = f

	return m.items, m.total, m.err
}

func TestExecute_Success(t *testing.T) {
	repo := &mockRepo{items: []domain.Activity{{ID: "a1"}, {ID: "a2"}}, total: 2}
	f := domain.ActivityFilter{Age: 4, GoalID: "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00", PublishedOnly: true, Limit: 20}

	items, total, err := New(nil, repo).Execute(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if total != 2 || len(items) != 2 {
		t.Fatalf("got %d items, total %d", len(items), total)
	}

	if repo.got != f {
		t.Fatalf("filter not passed through: %+v", repo.got)
	}
}

func TestExecute_Validation(t *testing.T) {
	for name, f := range map[string]domain.ActivityFilter{
		"bad goal id":  {GoalID: "motor"},
		"age too low":  {Age: 1},
		"age too high": {Age: 9},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := New(nil, &mockRepo{}).Execute(context.Background(), f); !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}

func TestExecute_Error(t *testing.T) {
	repo := &mockRepo{err: errs.ErrInternal}

	if _, _, err := New(nil, repo).Execute(context.Background(), domain.ActivityFilter{}); err == nil {
		t.Fatal("expected error but got nil")
	}
}
