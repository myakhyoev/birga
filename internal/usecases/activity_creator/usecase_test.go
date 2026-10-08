package activitycreator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got    domain.Activity
	called bool
	err    error
}

// fakeGoals reports active as the number of active goals among the asked ids; -1 means all of them.
type fakeGoals struct {
	active int
	got    []string
}

func (f *fakeGoals) CountActive(_ context.Context, ids []string) (int, error) {
	f.got = ids
	if f.active < 0 {
		return len(ids), nil
	}

	return f.active, nil
}

const goalID = "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"

func (m *mockRepo) Create(_ context.Context, a domain.Activity) (domain.Activity, error) {
	m.called = true
	m.got = a
	a.ID = "new-id"

	return a, m.err
}

func validActivity() domain.Activity {
	return domain.Activity{
		TitleUz:         " Rangli tosh ",
		TitleRu:         "Цветные камни",
		DescriptionUz:   "Toshlarni rangi bo'yicha saralang",
		DescriptionRu:   "Сортируйте камни по цвету",
		GoalIDs:         []string{goalID},
		MinAge:          3,
		MaxAge:          5,
		DurationMinutes: 10,
	}
}

func TestExecute_Success(t *testing.T) {
	repo := &mockRepo{}

	a := validActivity()
	a.GoalIDs = []string{goalID, strings.ToUpper(goalID)}

	got, err := New(nil, repo, &fakeGoals{active: -1}).Execute(context.Background(), a)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "new-id" {
		t.Fatalf("ID = %q, want new-id", got.ID)
	}

	if repo.got.TitleUz != "Rangli tosh" {
		t.Fatalf("title should be trimmed, got %q", repo.got.TitleUz)
	}

	if len(repo.got.GoalIDs) != 1 || repo.got.GoalIDs[0] != goalID {
		t.Fatalf("goal ids should be deduplicated, got %v", repo.got.GoalIDs)
	}
}

func TestExecute_UnknownGoal(t *testing.T) {
	repo := &mockRepo{}

	_, err := New(nil, repo, &fakeGoals{active: 0}).Execute(context.Background(), validActivity())
	if !errors.Is(err, errs.ErrUnknownGoals) || repo.called {
		t.Fatalf("err=%v called=%v", err, repo.called)
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]func(a *domain.Activity){
		"missing title": func(a *domain.Activity) { a.TitleRu = "  " },
		"missing desc":  func(a *domain.Activity) { a.DescriptionUz = "" },
		"no goals":      func(a *domain.Activity) { a.GoalIDs = nil },
		"bad goal id":   func(a *domain.Activity) { a.GoalIDs = []string{"motor"} },
		"too many goals": func(a *domain.Activity) {
			a.GoalIDs = []string{goalID, "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222",
				"33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444", "55555555-5555-5555-5555-555555555555"}
		},
		"age too low":      func(a *domain.Activity) { a.MinAge = 1 },
		"age too high":     func(a *domain.Activity) { a.MaxAge = 7 },
		"age inverted":     func(a *domain.Activity) { a.MinAge, a.MaxAge = 5, 3 },
		"zero duration":    func(a *domain.Activity) { a.DurationMinutes = 0 },
		"too long session": func(a *domain.Activity) { a.DurationMinutes = 61 },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := validActivity()
			mutate(&a)

			repo := &mockRepo{}

			_, err := New(nil, repo, &fakeGoals{active: -1}).Execute(context.Background(), a)
			if !errors.Is(err, errs.ErrValidation) {
				t.Fatalf("expected validation error, got %v", err)
			}

			if repo.called {
				t.Fatal("repo must not be called for invalid input")
			}
		})
	}
}

func TestExecute_RepoError(t *testing.T) {
	repo := &mockRepo{err: errs.ErrInternal}

	if _, err := New(nil, repo, &fakeGoals{active: -1}).Execute(context.Background(), validActivity()); !errors.Is(err, errs.ErrInternal) {
		t.Fatalf("expected internal error, got %v", err)
	}
}
