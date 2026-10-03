package activityupdater

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got    domain.ActivityUpdate
	called bool
}

func (m *mockRepo) Update(_ context.Context, id string, upd domain.ActivityUpdate) (domain.Activity, error) {
	m.called, m.got = true, upd

	return domain.Activity{ID: id}, nil
}

func ptr[T any](v T) *T { return &v }

func TestExecute_Success(t *testing.T) {
	repo := &mockRepo{}

	if _, err := New(nil, repo).Execute(context.Background(), "id", domain.ActivityUpdate{
		TitleUz: ptr("  Yangi nom "), IsPublished: ptr(true),
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *repo.got.TitleUz != "Yangi nom" || !*repo.got.IsPublished {
		t.Fatalf("update not passed through: %+v", repo.got)
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]domain.ActivityUpdate{
		"empty":         {},
		"blank title":   {TitleRu: ptr("  ")},
		"blank desc":    {DescriptionUz: ptr("")},
		"unknown goal":  {Goal: ptr("flying")},
		"age too low":   {MinAge: ptr(1)},
		"age too high":  {MaxAge: ptr(7)},
		"min above max": {MinAge: ptr(5), MaxAge: ptr(3)},
		"zero duration": {DurationMinutes: ptr(0)},
		"long duration": {DurationMinutes: ptr(61)},
	}

	for name, upd := range cases {
		repo := &mockRepo{}

		_, err := New(nil, repo).Execute(context.Background(), "id", upd)
		if !errors.Is(err, errs.ErrValidation) || repo.called {
			t.Errorf("%s: err=%v called=%v", name, err, repo.called)
		}
	}
}
