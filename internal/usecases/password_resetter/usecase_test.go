package passwordresetter

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const phone = "+998901234567"

type fakeVerified struct {
	ok       bool
	consumed bool
}

func (f *fakeVerified) IsVerified(_ context.Context, p string, purpose domain.OTPPurpose) (bool, error) {
	return f.ok && p == phone && purpose == domain.OTPPurposeResetPassword, nil
}

func (f *fakeVerified) ConsumeVerified(context.Context, string, domain.OTPPurpose) error {
	f.consumed = true

	return nil
}

type fakeUsers struct{}

func (fakeUsers) Get(_ context.Context, id string) (domain.User, error) {
	p := phone

	return domain.User{ID: id, PhoneNumber: &p}, nil
}

func (fakeUsers) GetByPhone(_ context.Context, p string) (domain.User, error) {
	if p != phone {
		return domain.User{}, errs.ErrUserNotFound
	}

	return domain.User{ID: "u1", PhoneNumber: &p}, nil
}

type fakeAuth struct {
	set *domain.UserAuth
}

func (f *fakeAuth) Get(_ context.Context, userID string) (domain.UserAuth, error) {
	return domain.UserAuth{UserID: userID, Role: domain.UserRolePaidUser}, nil
}

func (f *fakeAuth) SetCredentials(_ context.Context, a domain.UserAuth) error {
	f.set = &a

	return nil
}

type fakeTokens struct{ roles []domain.UserRole }

func (f *fakeTokens) Issue(_ string, role domain.UserRole, typ domain.TokenType) (string, error) {
	f.roles = append(f.roles, role)

	return string(typ) + "-token", nil
}

func (f *fakeTokens) AccessTTL() time.Duration { return time.Hour }

func newUC(v *fakeVerified, a *fakeAuth, tk *fakeTokens) *UseCase {
	uc := New(nil, v, fakeUsers{}, a, tk)
	uc.cost = bcrypt.MinCost

	return uc
}

func TestExecute(t *testing.T) {
	v, a, tk := &fakeVerified{ok: true}, &fakeAuth{}, &fakeTokens{}

	pair, err := newUC(v, a, tk).Execute(context.Background(), "u1", "new-pass-123")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if pair.AccessToken != "access-token" || pair.RefreshToken != "refresh-token" || pair.AccessExpiresIn != time.Hour {
		t.Fatalf("pair %+v", pair)
	}

	if a.set == nil || bcrypt.CompareHashAndPassword([]byte(a.set.PasswordHash), []byte("new-pass-123")) != nil ||
		a.set.AccessTokenHash != domain.HashToken("access-token") || a.set.RefreshTokenHash != domain.HashToken("refresh-token") {
		t.Fatalf("stored %+v", a.set)
	}

	if !v.consumed || tk.roles[0] != domain.UserRolePaidUser {
		t.Fatalf("consumed %v roles %v", v.consumed, tk.roles)
	}
}

func TestExecute_NotVerified(t *testing.T) {
	v, a := &fakeVerified{}, &fakeAuth{}

	if _, err := newUC(v, a, &fakeTokens{}).Execute(context.Background(), "u1", "new-pass-123"); !errors.Is(err, errs.ErrResetNotVerified) || a.set != nil {
		t.Fatalf("expected not verified and nothing stored, got %v", err)
	}
}

func TestExecute_BadPassword(t *testing.T) {
	for _, p := range []string{"short", string(make([]byte, domain.MaxPasswordLength+1))} {
		a := &fakeAuth{}
		if _, err := newUC(&fakeVerified{ok: true}, a, &fakeTokens{}).Execute(context.Background(), "u1", p); !errors.Is(err, errs.ErrValidation) || a.set != nil {
			t.Fatalf("len %d: %v", len(p), err)
		}
	}
}

func TestExecuteByPhone(t *testing.T) {
	v, a, tk := &fakeVerified{ok: true}, &fakeAuth{}, &fakeTokens{}

	pair, err := newUC(v, a, tk).ExecuteByPhone(context.Background(), " "+phone+" ", "new-pass-123")
	if err != nil {
		t.Fatalf("ExecuteByPhone: %v", err)
	}

	if pair.AccessToken != "access-token" || a.set == nil || a.set.UserID != "u1" ||
		bcrypt.CompareHashAndPassword([]byte(a.set.PasswordHash), []byte("new-pass-123")) != nil || !v.consumed {
		t.Fatalf("pair %+v stored %+v consumed %v", pair, a.set, v.consumed)
	}
}

func TestExecuteByPhone_Errors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verified bool
		phone    string
		password string
		want     error
	}{
		{"not verified", false, phone, "new-pass-123", errs.ErrResetNotVerified},
		{"unknown phone", true, "+998907654321", "new-pass-123", errs.ErrPhoneNumberNotRegistered},
		{"bad phone", true, "12345", "new-pass-123", errs.ErrValidation},
		{"short password", true, phone, "short", errs.ErrValidation},
	} {
		a := &fakeAuth{}
		if _, err := newUC(&fakeVerified{ok: tc.verified}, a, &fakeTokens{}).ExecuteByPhone(context.Background(), tc.phone, tc.password); !errors.Is(err, tc.want) || a.set != nil {
			t.Fatalf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}
