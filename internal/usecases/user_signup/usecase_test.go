package usersignup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

const userID = "7b0c1f1e-2d7a-4d8e-9a55-0f4a0d7f9c11"

const goalID = "2b6f0cc9-0f3e-4b1a-9a7e-5d8c3e2f1a00"

type fakes struct {
	user    domain.User
	auth    domain.UserAuth
	userErr error
	inTx    bool
	// activeGoals is how many of the asked ids CountActive reports as active; -1 means all of them.
	activeGoals int
	gotGoals    []string
}

func (f *fakes) CountActive(_ context.Context, ids []string) (int, error) {
	f.gotGoals = ids
	if f.activeGoals < 0 {
		return len(ids), nil
	}

	return f.activeGoals, nil
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
		f,
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
	return domain.SignUpRequest{Name: " Dilnoza ", Username: " Dilnoza_K ", Password: "s3cret-pass", PhoneNumber: "+998901234567",
		Relationship: " Mother ", GoalIDs: []string{goalID, strings.ToUpper(goalID)}}
}

func TestExecute(t *testing.T) {
	f := &fakes{activeGoals: -1}

	pair, err := newUseCase(f).Execute(context.Background(), req())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if pair.UserID != userID || pair.AccessToken != "access."+userID+".unverified_user" || pair.RefreshToken != "refresh."+userID+".unverified_user" || pair.AccessExpiresIn != 15*time.Minute {
		t.Fatalf("unexpected pair: %+v", pair)
	}

	checkUser(t, f.user)

	if f.auth.UserID != userID || f.auth.Username != "dilnoza_k" || f.auth.Role != domain.UserRoleUnverifiedUser ||
		f.auth.AccessTokenHash != domain.HashToken(pair.AccessToken) || f.auth.RefreshTokenHash != domain.HashToken(pair.RefreshToken) {
		t.Fatalf("unexpected auth row: %+v", f.auth)
	}

	if bcrypt.CompareHashAndPassword([]byte(f.auth.PasswordHash), []byte("s3cret-pass")) != nil {
		t.Fatal("password hash does not match the password")
	}
}

func TestExecute_CreateFails(t *testing.T) {
	f := &fakes{userErr: errs.ErrUsernameTaken, activeGoals: -1}

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
		"no relation":    func(r *domain.SignUpRequest) { r.Relationship = "" },
		"bad relation":   func(r *domain.SignUpRequest) { r.Relationship = "uncle" },
		"bad goal id":    func(r *domain.SignUpRequest) { r.GoalIDs = []string{"speech"} },
		"too many goals": func(r *domain.SignUpRequest) {
			r.GoalIDs = nil
			for i := range domain.MaxUserGoals + 1 {
				r.GoalIDs = append(r.GoalIDs, fmt.Sprintf("00000000-0000-0000-0000-%012d", i))
			}
		},
	}

	for name, mutate := range cases {
		r := req()
		mutate(&r)

		if _, err := newUseCase(&fakes{activeGoals: -1}).Execute(context.Background(), r); !errors.Is(err, errs.ErrValidation) {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestExecute_UnknownGoal(t *testing.T) {
	f := &fakes{activeGoals: 0}

	if _, err := newUseCase(f).Execute(context.Background(), req()); !errors.Is(err, errs.ErrUnknownGoals) || f.user.ID != "" {
		t.Fatalf("got %v", err)
	}
}

func TestExecute_NoGoals(t *testing.T) {
	f := &fakes{activeGoals: -1}
	r := req()
	r.GoalIDs = nil

	if _, err := newUseCase(f).Execute(context.Background(), r); err != nil || f.gotGoals != nil || len(f.user.GoalIDs) != 0 {
		t.Fatalf("got %v, asked %v", err, f.gotGoals)
	}
}

// checkUser checks the user sign-up stored for req(): trimmed, lowercased, goal ids deduplicated.
func checkUser(t *testing.T, u domain.User) {
	t.Helper()

	if *u.Name != "Dilnoza" || *u.Username != "dilnoza_k" || *u.PhoneNumber != "+998901234567" {
		t.Fatalf("unexpected user: %+v", u)
	}

	if *u.Relationship != domain.RelationshipMother || len(u.GoalIDs) != 1 || u.GoalIDs[0] != goalID {
		t.Fatalf("unexpected relationship or goals: %v %v", *u.Relationship, u.GoalIDs)
	}
}
