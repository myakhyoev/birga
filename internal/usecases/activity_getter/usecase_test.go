package activitygetter

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	a   domain.Activity
	err error
}

func (m *mockRepo) Get(_ context.Context, _ string) (domain.Activity, error) {
	return m.a, m.err
}

func TestExecute_Published(t *testing.T) {
	repo := &mockRepo{a: domain.Activity{ID: "a1", IsPublished: true}}

	got, err := New(nil, repo).Execute(context.Background(), "a1", false)
	if err != nil || got.ID != "a1" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestExecute_UnpublishedHidden(t *testing.T) {
	repo := &mockRepo{a: domain.Activity{ID: "a1"}}

	if _, err := New(nil, repo).Execute(context.Background(), "a1", false); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	if _, err := New(nil, repo).Execute(context.Background(), "a1", true); err != nil {
		t.Fatalf("admin should see unpublished activity, got %v", err)
	}
}

func TestExecute_RepoError(t *testing.T) {
	repo := &mockRepo{err: errs.ErrActivityNotFound}

	if _, err := New(nil, repo).Execute(context.Background(), "a1", true); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
