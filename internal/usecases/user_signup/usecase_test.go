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
	user    domain.User
	auth    domain.UserAuth
	userErr error
	inTx    bool
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
	uc := New(nil, f,
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
	f := &fakes{}

	pair, err := newUseCase(f).Execute(context.Background(), req())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if pair.AccessToken != "access."+userID+".unverified_user" || pair.RefreshToken != "refresh."+userID+".unverified_user" || pair.AccessExpiresIn != 15*time.Minute {
		t.Fatalf("unexpected pair: %+v", pair)
	}

	if *f.user.Name != "Dilnoza" || *f.user.Username != "dilnoza_k" || *f.user.PhoneNumber != "+998901234567" {
		t.Fatalf("unexpected user: %+v", f.user)
	}

	if f.auth.UserID != userID || f.auth.Username != "dilnoza_k" || f.auth.Role != domain.UserRoleUnverifiedUser ||
		f.auth.AccessTokenHash != domain.HashToken(pair.AccessToken) || f.auth.RefreshTokenHash != domain.HashToken(pair.RefreshToken) {
		t.Fatalf("unexpected auth row: %+v", f.auth)
	}

	if bcrypt.CompareHashAndPassword([]byte(f.auth.PasswordHash), []byte("s3cret-pass")) != nil {
		t.Fatal("password hash does not match the password")
	}
}

func TestExecute_CreateFails(t *testing.T) {
	f := &fakes{userErr: errs.ErrUsernameTaken}

	if _, err := newUseCase(f).Execute(context.Background(), req()); !errors.Is(err, errs.ErrUsernameTaken) {
		t.Fatalf("got %v", err)
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
	}

	for name, mutate := range cases {
		r := req()
		mutate(&r)

		if _, err := newUseCase(&fakes{}).Execute(context.Background(), r); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}
