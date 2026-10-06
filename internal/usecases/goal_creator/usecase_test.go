package goalcreator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got    domain.Goal
	called bool
}

func (m *mockRepo) Create(_ context.Context, g domain.Goal) (domain.Goal, error) {
	m.called = true
	m.got = g
	g.ID = "new-id"

	return g, nil
}

func TestExecute_TrimsAndStores(t *testing.T) {
	repo := &mockRepo{}

	got, err := New(nil, repo).Execute(context.Background(), domain.Goal{NameUz: " Nutq ", NameRu: "Речь", NameEn: "Speech "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "new-id" || repo.got.NameUz != "Nutq" || repo.got.NameEn != "Speech" {
		t.Fatalf("unexpected goal: %+v", repo.got)
	}
}

func TestExecute_Validation(t *testing.T) {
	for name, g := range map[string]domain.Goal{
		"blank uz": {NameUz: " ", NameRu: "Речь", NameEn: "Speech"},
		"no en":    {NameUz: "Nutq", NameRu: "Речь"},
		"long ru":  {NameUz: "Nutq", NameRu: strings.Repeat("я", domain.MaxGoalNameLength+1), NameEn: "Speech"},
	} {
		repo := &mockRepo{}

		if _, err := New(nil, repo).Execute(context.Background(), g); !errors.Is(err, errs.ErrValidation) || repo.called {
			t.Fatalf("%s: got %v, repo called %v", name, err, repo.called)
		}
	}
}
