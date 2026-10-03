package usercreator

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

type mockRepo struct {
	got domain.User
	err error
}

func (m *mockRepo) Create(_ context.Context, u domain.User) (domain.User, error) {
	m.got = u
	u.ID = "u1"

	return u, m.err
}

func ptr(s string) *string { return &s }

func TestExecute_Normalizes(t *testing.T) {
	repo := &mockRepo{}

	_, err := New(nil, repo).Execute(context.Background(), domain.User{
		Name:        ptr("  Dilnoza "),
		Username:    ptr(" Dilnoza_95 "),
		PhoneNumber: ptr("+998901234567"),
		PhotoID:     ptr("  "),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if *repo.got.Name != "Dilnoza" || *repo.got.Username != "dilnoza_95" || repo.got.PhotoID != nil {
		t.Fatalf("not normalized: name=%q username=%q photo=%v", *repo.got.Name, *repo.got.Username, repo.got.PhotoID)
	}
}

func TestExecute_Validation(t *testing.T) {
	phone := ptr("+998901234567")

	cases := map[string]domain.User{
		"no phone":       {Name: ptr("A")},
		"blank phone":    {PhoneNumber: ptr("  ")},
		"local phone":    {PhoneNumber: ptr("901234567")},
		"long phone":     {PhoneNumber: ptr("+9989012345678901")},
		"short username": {PhoneNumber: phone, Username: ptr("ab")},
		"bad username":   {PhoneNumber: phone, Username: ptr("dil noza")},
		"long name":      {PhoneNumber: phone, Name: ptr(string(make([]rune, domain.MaxUserNameLength+1)) + "x")},
		"bad photo":      {PhoneNumber: phone, PhotoID: ptr("photo")},
	}

	for name, u := range cases {
		if _, err := New(nil, &mockRepo{}).Execute(context.Background(), u); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: expected validation error, got %v", name, err)
		}
	}
}

func TestExecute_RepoError(t *testing.T) {
	repo := &mockRepo{err: errs.ErrPhoneNumberTaken}

	_, err := New(nil, repo).Execute(context.Background(), domain.User{PhoneNumber: ptr("+998901234567")})
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
