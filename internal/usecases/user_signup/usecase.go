package usersignup

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// verifiedStore knows which phone numbers passed a sign_up OTP check (Redis).
type verifiedStore interface {
	IsVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) (bool, error)
	ConsumeVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) error
}

type userRepo interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
}

type authRepo interface {
	Create(ctx context.Context, a domain.UserAuth) error
}

type txRunner interface {
	InTx(ctx context.Context, h func(context.Context) error) error
}

type tokenIssuer interface {
	Issue(userID string, role domain.UserRole, typ domain.TokenType) (string, error)
	AccessTTL() time.Duration
}

// UseCase creates a user whose phone number was verified with a sign_up code and signs them in.
type UseCase struct {
	l        logger.Logger
	verified verifiedStore
	tx       txRunner
	users    userRepo
	auth     authRepo
	tokens   tokenIssuer
	cost     int
}

// New creates a new sign-up use case.
func New(l logger.Logger, verified verifiedStore, tx txRunner, users userRepo, auth authRepo, tokens tokenIssuer) *UseCase {
	return &UseCase{
		l:        l,
		verified: verified,
		tx:       tx,
		users:    users,
		auth:     auth,
		tokens:   tokens,
		cost:     bcrypt.DefaultCost,
	}
}

// Execute checks the request and the phone verification, then stores the user and their
// user_auth row (bcrypt password, token hashes) in one transaction and returns the tokens.
// The verification is consumed only after the user is stored, so a failed attempt can be retried.
func (uc *UseCase) Execute(ctx context.Context, req domain.SignUpRequest) (domain.TokenPair, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.Role = domain.UserRole(strings.TrimSpace(string(req.Role)))

	if req.Role == "" {
		req.Role = domain.UserRoleUser
	}

	if err := validate(req); err != nil {
		return domain.TokenPair{}, err
	}

	verified, err := uc.verified.IsVerified(ctx, req.PhoneNumber, domain.OTPPurposeSignUp)
	if err != nil {
		return domain.TokenPair{}, err
	}

	if !verified {
		return domain.TokenPair{}, errs.ErrPhoneNotVerified
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), uc.cost)
	if err != nil {
		return domain.TokenPair{}, errs.Errf(errs.ErrInternal, "bcrypt: %s", err.Error())
	}

	var pair domain.TokenPair

	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		user, err := uc.users.Create(ctx, domain.User{Name: &req.Name, Username: &req.Username, PhoneNumber: &req.PhoneNumber})
		if err != nil {
			return err
		}

		if pair, err = uc.issue(user.ID, req.Role); err != nil {
			return err
		}

		return uc.auth.Create(ctx, domain.UserAuth{
			UserID:           user.ID,
			Username:         req.Username,
			PasswordHash:     string(hash),
			Role:             req.Role,
			AccessTokenHash:  domain.HashToken(pair.AccessToken),
			RefreshTokenHash: domain.HashToken(pair.RefreshToken),
		})
	})
	if err != nil {
		return domain.TokenPair{}, err
	}

	l := logger.WithContext(uc.l, ctx)

	// The user exists now; a leftover mark only lets the same phone hit the unique index again.
	if err := uc.verified.ConsumeVerified(context.WithoutCancel(ctx), req.PhoneNumber, domain.OTPPurposeSignUp); err != nil {
		l.Error("verified.ConsumeVerified", zap.Error(err))
	}

	l.Info("user signed up", zap.String("role", string(req.Role)))

	return pair, nil
}

func (uc *UseCase) issue(userID string, role domain.UserRole) (domain.TokenPair, error) {
	access, err := uc.tokens.Issue(userID, role, domain.TokenTypeAccess)
	if err != nil {
		return domain.TokenPair{}, err
	}

	refresh, err := uc.tokens.Issue(userID, role, domain.TokenTypeRefresh)
	if err != nil {
		return domain.TokenPair{}, err
	}

	return domain.TokenPair{AccessToken: access, RefreshToken: refresh, AccessExpiresIn: uc.tokens.AccessTTL()}, nil
}

func validate(req domain.SignUpRequest) error {
	switch {
	case req.Name == "":
		return errs.Errf(errs.ErrValidation, "name is required")
	case !domain.IsValidUserName(req.Name):
		return errs.Errf(errs.ErrValidation, "name must be at most %d characters", domain.MaxUserNameLength)
	case !domain.IsValidUsername(req.Username):
		return errs.Errf(errs.ErrValidation, "username must be 3 to 32 characters: latin letters, digits, '_' or '.'")
	case len(req.Password) < domain.MinPasswordLength || len(req.Password) > domain.MaxPasswordLength:
		return errs.Errf(errs.ErrValidation, "password must be %d to %d bytes long", domain.MinPasswordLength, domain.MaxPasswordLength)
	case !domain.IsUzbekPhoneNumber(req.PhoneNumber):
		return errs.Errf(errs.ErrValidation, "phone_number must be an Uzbek number in E.164 format, e.g. +998901234567")
	case !req.Role.IsKnown():
		return errs.Errf(errs.ErrValidation, "user_role must be %q or %q", domain.UserRoleUser, domain.UserRolePaidUser)
	case !req.Role.IsSelfAssignable():
		return errs.ErrRoleNotSelfAssignable
	}

	return nil
}
