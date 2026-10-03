package profileupdater

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const currentPhone = "+998901111111"

type fakeVerified struct {
	verified map[string]bool
	consumed string
}

func (f *fakeVerified) IsVerified(_ context.Context, phone string, purpose domain.OTPPurpose) (bool, error) {
	return purpose == domain.OTPPurposeUpdateUser && f.verified[phone], nil
}

func (f *fakeVerified) ConsumeVerified(_ context.Context, phone string, _ domain.OTPPurpose) error {
	f.consumed = phone

	return nil
}

type fakeUsers struct {
	got    *domain.UserUpdate
	called bool
}

func (f *fakeUsers) get(context.Context, string) (domain.User, error) {
	p := currentPhone

	return domain.User{ID: "u1", PhoneNumber: &p}, nil
}

func (f *fakeUsers) Execute(_ context.Context, id string, upd domain.UserUpdate) (domain.User, error) {
	f.called, f.got = true, &upd

	return domain.User{ID: id}, nil
}

type getterFunc func(context.Context, string) (domain.User, error)

func (fn getterFunc) Execute(ctx context.Context, id string) (domain.User, error) { return fn(ctx, id) }

func newUC(verified *fakeVerified, users *fakeUsers) *UseCase {
	return New(nil, verified, getterFunc(users.get), users)
}

func ptr(s string) *string { return &s }

func TestExecute_NameOnly(t *testing.T) {
	v, users := &fakeVerified{}, &fakeUsers{}

	if _, err := newUC(v, users).Execute(context.Background(), "u1", domain.UserUpdate{Name: ptr("Dilnoza")}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if users.got == nil || *users.got.Name != "Dilnoza" || users.got.PhoneNumber != nil || v.consumed != "" {
		t.Fatalf("update %+v consumed %q", users.got, v.consumed)
	}
}

func TestExecute_NewPhoneNeedsVerification(t *testing.T) {
	v, users := &fakeVerified{}, &fakeUsers{}

	_, err := newUC(v, users).Execute(context.Background(), "u1", domain.UserUpdate{PhoneNumber: ptr("+998902222222")})
	if !errors.Is(err, errs.ErrNewPhoneNotVerified) || users.called {
		t.Fatalf("expected not verified and no update, got %v", err)
	}

	v.verified = map[string]bool{"+998902222222": true}

	if _, err := newUC(v, users).Execute(context.Background(), "u1", domain.UserUpdate{PhoneNumber: ptr(" +998902222222 ")}); err != nil {
		t.Fatalf("verified: %v", err)
	}

	if *users.got.PhoneNumber != "+998902222222" || v.consumed != "+998902222222" {
		t.Fatalf("update %+v consumed %q", users.got, v.consumed)
	}
}

func TestExecute_SamePhoneIsNoChange(t *testing.T) {
	v, users := &fakeVerified{}, &fakeUsers{}

	got, err := newUC(v, users).Execute(context.Background(), "u1", domain.UserUpdate{PhoneNumber: ptr(currentPhone)})
	if err != nil || users.called || got.ID != "u1" {
		t.Fatalf("got %+v, %v, update called %v", got, err, users.called)
	}

	if _, err := newUC(v, users).Execute(context.Background(), "u1",
		domain.UserUpdate{PhoneNumber: ptr(currentPhone), Name: ptr("A")}); err != nil || users.got.PhoneNumber != nil {
		t.Fatalf("with name: %v %+v", err, users.got)
	}
}

func TestExecute_Invalid(t *testing.T) {
	for name, upd := range map[string]domain.UserUpdate{
		"empty":       {},
		"not uzbek":   {PhoneNumber: ptr("+12025550123")},
		"bad phone":   {PhoneNumber: ptr("123")},
		"clear phone": {PhoneNumber: ptr("")},
	} {
		users := &fakeUsers{}
		if _, err := newUC(&fakeVerified{}, users).Execute(context.Background(), "u1", upd); !errors.Is(err, errs.ErrValidation) || users.called {
			t.Errorf("%s: expected validation error, got %v", name, err)
		}
	}
}
