package userdeleter

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got string
	err error
}

func (m *mockRepo) Delete(_ context.Context, id string) error {
	m.got = id

	return m.err
}

func TestExecute(t *testing.T) {
	repo := &mockRepo{}

	if err := New(nil, repo).Execute(context.Background(), "u1"); err != nil || repo.got != "u1" {
		t.Fatalf("got %q, %v", repo.got, err)
	}

	repo.err = errs.ErrUserNotFound
	if err := New(nil, repo).Execute(context.Background(), "u1"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
