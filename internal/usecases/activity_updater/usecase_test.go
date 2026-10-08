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

// fakeGoals reports active as the number of active goals among the asked ids; -1 means all of them.
type fakeGoals struct{ active int }

func (f fakeGoals) CountActive(_ context.Context, ids []string) (int, error) {
	if f.active < 0 {
		return len(ids), nil
	}

	return f.active, nil
}

const goalID = "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"

func ptr[T any](v T) *T { return &v }

func TestExecute_Success(t *testing.T) {
	repo := &mockRepo{}

	if _, err := New(nil, repo, fakeGoals{active: -1}).Execute(context.Background(), "id", domain.ActivityUpdate{
		TitleUz: ptr("  Yangi nom "), IsPublished: ptr(true), GoalIDs: []string{goalID, goalID},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *repo.got.TitleUz != "Yangi nom" || !*repo.got.IsPublished || len(repo.got.GoalIDs) != 1 {
		t.Fatalf("update not passed through: %+v", repo.got)
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]domain.ActivityUpdate{
		"empty":         {},
		"blank title":   {TitleRu: ptr("  ")},
		"blank desc":    {DescriptionUz: ptr("")},
		"no goals":      {GoalIDs: []string{}},
		"bad goal id":   {GoalIDs: []string{"motor"}},
		"age too low":   {MinAge: ptr(1)},
		"age too high":  {MaxAge: ptr(7)},
		"min above max": {MinAge: ptr(5), MaxAge: ptr(3)},
		"zero duration": {DurationMinutes: ptr(0)},
		"long duration": {DurationMinutes: ptr(61)},
	}

	for name, upd := range cases {
		repo := &mockRepo{}

		_, err := New(nil, repo, fakeGoals{active: -1}).Execute(context.Background(), "id", upd)
		if !errors.Is(err, errs.ErrValidation) || repo.called {
			t.Errorf("%s: err=%v called=%v", name, err, repo.called)
		}
	}
}

func TestExecute_UnknownGoal(t *testing.T) {
	repo := &mockRepo{}

	_, err := New(nil, repo, fakeGoals{active: 0}).Execute(context.Background(), "id", domain.ActivityUpdate{GoalIDs: []string{goalID}})
	if !errors.Is(err, errs.ErrUnknownGoals) || repo.called {
		t.Fatalf("err=%v called=%v", err, repo.called)
	}
}
