package userupdater

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got domain.UserUpdate
	err error
}

func (m *mockRepo) Update(_ context.Context, id string, upd domain.UserUpdate) (domain.User, error) {
	m.got = upd

	return domain.User{ID: id}, m.err
}

func ptr(s string) *string { return &s }

func TestExecute_NormalizesAndClears(t *testing.T) {
	repo := &mockRepo{}

	_, err := New(nil, repo).Execute(context.Background(), "u1", domain.UserUpdate{
		Username: ptr(" NewName "),
		Name:     ptr("  "),
		PhotoID:  ptr(""),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if *repo.got.Username != "newname" || *repo.got.Name != "" || *repo.got.PhotoID != "" || repo.got.PhoneNumber != nil {
		t.Fatalf("unexpected update: %+v", repo.got)
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]domain.UserUpdate{
		"empty":        {},
		"clear phone":  {PhoneNumber: ptr("")},
		"bad phone":    {PhoneNumber: ptr("12345")},
		"bad username": {Username: ptr("a!")},
		"bad photo":    {PhotoID: ptr("x")},
	}

	for name, upd := range cases {
		if _, err := New(nil, &mockRepo{}).Execute(context.Background(), "u1", upd); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: expected validation error, got %v", name, err)
		}
	}
}

func TestExecute_RepoError(t *testing.T) {
	repo := &mockRepo{err: errs.ErrUserNotFound}

	if _, err := New(nil, repo).Execute(context.Background(), "u1", domain.UserUpdate{Name: ptr("A")}); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}
