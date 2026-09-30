package activitycreator

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got    domain.Activity
	called bool
	err    error
}

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
		Goal:            domain.GoalCognitive,
		MinAge:          3,
		MaxAge:          5,
		DurationMinutes: 10,
	}
}

func TestExecute_Success(t *testing.T) {
	repo := &mockRepo{}

	got, err := New(nil, repo).Execute(context.Background(), validActivity())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "new-id" {
		t.Fatalf("ID = %q, want new-id", got.ID)
	}

	if repo.got.TitleUz != "Rangli tosh" {
		t.Fatalf("title should be trimmed, got %q", repo.got.TitleUz)
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]func(a *domain.Activity){
		"missing title":    func(a *domain.Activity) { a.TitleRu = "  " },
		"missing desc":     func(a *domain.Activity) { a.DescriptionUz = "" },
		"unknown goal":     func(a *domain.Activity) { a.Goal = "flying" },
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

			_, err := New(nil, repo).Execute(context.Background(), a)
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

	if _, err := New(nil, repo).Execute(context.Background(), validActivity()); !errors.Is(err, errs.ErrInternal) {
		t.Fatalf("expected internal error, got %v", err)
	}
}
