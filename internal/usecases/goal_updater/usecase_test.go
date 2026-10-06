package goalupdater

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got    domain.GoalUpdate
	called bool
}

func (m *mockRepo) Update(_ context.Context, id string, upd domain.GoalUpdate) (domain.Goal, error) {
	m.called = true
	m.got = upd

	return domain.Goal{ID: id}, nil
}

func ptr(s string) *string { return &s }

func TestExecute_Trims(t *testing.T) {
	repo := &mockRepo{}

	if _, err := New(nil, repo).Execute(context.Background(), "id", domain.GoalUpdate{NameEn: ptr(" Speech ")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *repo.got.NameEn != "Speech" || repo.got.NameUz != nil {
		t.Fatalf("unexpected update: %+v", repo.got)
	}
}

func TestExecute_Validation(t *testing.T) {
	for name, upd := range map[string]domain.GoalUpdate{
		"empty":    {},
		"blank uz": {NameUz: ptr("  ")},
	} {
		repo := &mockRepo{}

		if _, err := New(nil, repo).Execute(context.Background(), "id", upd); !errors.Is(err, errs.ErrValidation) || repo.called {
			t.Fatalf("%s: got %v, repo called %v", name, err, repo.called)
		}
	}
}
