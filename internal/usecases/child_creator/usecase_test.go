package childcreator

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type fakeTx struct{}

func (fakeTx) InTx(ctx context.Context, h func(context.Context) error) error { return h(ctx) }

type fakeRepo struct {
	got       domain.Child
	linked    [2]string
	createErr error
	linkErr   error
}

func (f *fakeRepo) Create(_ context.Context, c domain.Child) (domain.Child, error) {
	f.got = c
	c.ID = "c1"

	return c, f.createErr
}

func (f *fakeRepo) AddParent(_ context.Context, childID, userID string) error {
	f.linked = [2]string{childID, userID}

	return f.linkErr
}

func TestExecute(t *testing.T) {
	repo := &fakeRepo{}
	blank := "  "

	got, err := New(nil, fakeTx{}, repo).Execute(context.Background(), "u1",
		domain.Child{Name: " Amir ", Age: 3, Gender: " Male", PhotoID: &blank})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if got.ID != "c1" || repo.got.Name != "Amir" || repo.got.Gender != domain.GenderMale || repo.got.PhotoID != nil {
		t.Fatalf("stored %+v, returned %+v", repo.got, got)
	}

	if repo.linked != [2]string{"c1", "u1"} {
		t.Fatalf("linked %v", repo.linked)
	}
}

func TestExecute_Invalid(t *testing.T) {
	bad := "x"

	for name, c := range map[string]domain.Child{
		"no name":  {Age: 3, Gender: domain.GenderMale},
		"old":      {Name: "A", Age: 19, Gender: domain.GenderMale},
		"negative": {Name: "A", Age: -1, Gender: domain.GenderMale},
		"gender":   {Name: "A", Age: 3, Gender: "other"},
		"photo id": {Name: "A", Age: 3, Gender: domain.GenderFemale, PhotoID: &bad},
	} {
		repo := &fakeRepo{}
		if _, err := New(nil, fakeTx{}, repo).Execute(context.Background(), "u1", c); !errors.Is(err, errs.ErrValidation) {
			t.Errorf("%s: expected validation error, got %v", name, err)
		}

		if repo.got.Name != "" {
			t.Errorf("%s: repo was called", name)
		}
	}
}

func TestExecute_LinkFails(t *testing.T) {
	repo := &fakeRepo{linkErr: errs.ErrUserNotFound}

	if _, err := New(nil, fakeTx{}, repo).Execute(context.Background(), "u1",
		domain.Child{Name: "A", Age: 3, Gender: domain.GenderMale}); !errors.Is(err, errs.ErrUserNotFound) {
		t.Fatalf("expected user not found, got %v", err)
	}
}
