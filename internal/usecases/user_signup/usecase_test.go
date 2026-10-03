package usersignup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const userID = "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"

type fakes struct {
	verified bool
	consumed bool
	user     domain.User
	auth     domain.UserAuth
	userErr  error
	inTx     bool
}

func (f *fakes) IsVerified(_ context.Context, phone string, purpose domain.OTPPurpose) (bool, error) {
	return f.verified && phone == "+998901234567" && purpose == domain.OTPPurposeSignUp, nil
}

func (f *fakes) ConsumeVerified(context.Context, string, domain.OTPPurpose) error {
	f.consumed = true

	return nil
}

func (f *fakes) InTx(ctx context.Context, h func(context.Context) error) error {
	f.inTx = true
	defer func() { f.inTx = false }()

	return h(ctx)
}

type userRepoFunc func(domain.User) (domain.User, error)

func (fn userRepoFunc) Create(_ context.Context, u domain.User) (domain.User, error) { return fn(u) }

type authRepoFunc func(domain.UserAuth) error

func (fn authRepoFunc) Create(_ context.Context, a domain.UserAuth) error { return fn(a) }

type fakeTokens struct{}

func (fakeTokens) Issue(userID string, role domain.UserRole, typ domain.TokenType) (string, error) {
	return string(typ) + "." + userID + "." + string(role), nil
}

func (fakeTokens) AccessTTL() time.Duration { return 15 * time.Minute }

func newUseCase(f *fakes) *UseCase {
	uc := New(nil, f, f,
		userRepoFunc(func(u domain.User) (domain.User, error) {
			if !f.inTx {
				panic("users.Create outside the transaction")
			}

			f.user = u
			u.ID = userID

			return u, f.userErr
		}),
		authRepoFunc(func(a domain.UserAuth) error {
			if !f.inTx {
				panic("auth.Create outside the transaction")
			}

			f.auth = a

			return nil
		}),
		fakeTokens{},
	)
	uc.cost = bcrypt.MinCost

	return uc
}

func req() domain.SignUpRequest {
	return domain.SignUpRequest{Name: " Dilnoza ", Username: " Dilnoza_K ", Password: "s3cret-pass", PhoneNumber: "+998901234567"}
}

func TestExecute(t *testing.T) {
	f := &fakes{verified: true}

	pair, err := newUseCase(f).Execute(context.Background(), req())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if pair.AccessToken != "access."+userID+".user" || pair.RefreshToken != "refresh."+userID+".user" || pair.AccessExpiresIn != 15*time.Minute {
		t.Fatalf("unexpected pair: %+v", pair)
	}

	if *f.user.Name != "Dilnoza" || *f.user.Username != "dilnoza_k" || *f.user.PhoneNumber != "+998901234567" {
		t.Fatalf("unexpected user: %+v", f.user)
	}

	if f.auth.UserID != userID || f.auth.Username != "dilnoza_k" || f.auth.Role != domain.UserRoleUser ||
		f.auth.AccessTokenHash != domain.HashToken(pair.AccessToken) || f.auth.RefreshTokenHash != domain.HashToken(pair.RefreshToken) {
		t.Fatalf("unexpected auth row: %+v", f.auth)
	}

	if bcrypt.CompareHashAndPassword([]byte(f.auth.PasswordHash), []byte("s3cret-pass")) != nil {
		t.Fatal("password hash does not match the password")
	}

	if !f.consumed {
		t.Fatal("verification not consumed")
	}
}

func TestExecute_Roles(t *testing.T) {
	r := req()
	r.Role = " paid_user "

	f := &fakes{verified: true}

	pair, err := newUseCase(f).Execute(context.Background(), r)
	if err != nil || f.auth.Role != domain.UserRolePaidUser || pair.AccessToken != "access."+userID+".paid_user" {
		t.Fatalf("paid_user: %+v %v, auth %+v", pair, err, f.auth)
	}

	r.Role = domain.UserRoleAdmin
	f = &fakes{verified: true}

	if _, err := newUseCase(f).Execute(context.Background(), r); !errors.Is(err, errs.ErrRoleNotSelfAssignable) || f.user.PhoneNumber != nil {
		t.Fatalf("admin: %v", err)
	}
}

func TestExecute_NotVerified(t *testing.T) {
	f := &fakes{}

	if _, err := newUseCase(f).Execute(context.Background(), req()); !errors.Is(err, errs.ErrPhoneNotVerified) {
		t.Fatalf("got %v", err)
	}

	if f.user.PhoneNumber != nil {
		t.Fatal("user created without verification")
	}
}

func TestExecute_CreateFailsKeepsVerification(t *testing.T) {
	f := &fakes{verified: true, userErr: errs.ErrUsernameTaken}

	if _, err := newUseCase(f).Execute(context.Background(), req()); !errors.Is(err, errs.ErrUsernameTaken) {
		t.Fatalf("got %v", err)
	}

	if f.consumed {
		t.Fatal("verification consumed although the user was not created")
	}
}

func TestExecute_Validation(t *testing.T) {
	cases := map[string]func(*domain.SignUpRequest){
		"no name":        func(r *domain.SignUpRequest) { r.Name = " " },
		"long name":      func(r *domain.SignUpRequest) { r.Name = strings.Repeat("a", domain.MaxUserNameLength+1) },
		"bad username":   func(r *domain.SignUpRequest) { r.Username = "a b" },
		"short password": func(r *domain.SignUpRequest) { r.Password = "1234567" },
		"long password":  func(r *domain.SignUpRequest) { r.Password = strings.Repeat("p", domain.MaxPasswordLength+1) },
		"foreign phone":  func(r *domain.SignUpRequest) { r.PhoneNumber = "+77011234567" },
		"unknown role":   func(r *domain.SignUpRequest) { r.Role = "owner" },
	}

	for name, mutate := range cases {
		r := req()
		mutate(&r)

		if _, err := newUseCase(&fakes{verified: true}).Execute(context.Background(), r); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}
