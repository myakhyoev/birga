package passwordresetter

import (
	"context"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// verifiedStore knows which phone numbers passed an OTP check (Redis).
type verifiedStore interface {
	IsVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) (bool, error)
	ConsumeVerified(ctx context.Context, phone string, purpose domain.OTPPurpose) error
}

type userRepo interface {
	Get(ctx context.Context, id string) (domain.User, error)
}

type authRepo interface {
	Get(ctx context.Context, userID string) (domain.UserAuth, error)
	SetCredentials(ctx context.Context, a domain.UserAuth) error
}

type tokenIssuer interface {
	Issue(userID string, role domain.UserRole, typ domain.TokenType) (string, error)
	AccessTTL() time.Duration
}

// UseCase sets a new password for the signed-in user once their phone passed a reset_password OTP check.
type UseCase struct {
	l        logger.Logger
	verified verifiedStore
	users    userRepo
	auth     authRepo
	tokens   tokenIssuer
	cost     int
}

// New creates a new password resetter use case.
func New(l logger.Logger, verified verifiedStore, users userRepo, auth authRepo, tokens tokenIssuer) *UseCase {
	return &UseCase{l: l, verified: verified, users: users, auth: auth, tokens: tokens, cost: bcrypt.DefaultCost}
}

// Execute checks that the user's own phone number passed a reset_password check, stores the new
// bcrypt password and a new token pair, and returns the pair. Storing new token hashes signs out
// every other device. The verification is used up only after the password is stored.
func (uc *UseCase) Execute(ctx context.Context, userID, password string) (domain.TokenPair, error) {
	if len(password) < domain.MinPasswordLength || len(password) > domain.MaxPasswordLength {
		return domain.TokenPair{}, errs.Errf(errs.ErrValidation,
			"password must be %d to %d bytes long", domain.MinPasswordLength, domain.MaxPasswordLength)
	}

	user, err := uc.users.Get(ctx, userID)
	if err != nil {
		return domain.TokenPair{}, err
	}

	if user.PhoneNumber == nil {
		return domain.TokenPair{}, errs.ErrResetNotVerified
	}

	phone := *user.PhoneNumber

	ok, err := uc.verified.IsVerified(ctx, phone, domain.OTPPurposeResetPassword)
	if err != nil {
		return domain.TokenPair{}, err
	}

	if !ok {
		return domain.TokenPair{}, errs.ErrResetNotVerified
	}

	a, err := uc.auth.Get(ctx, userID)
	if err != nil {
		return domain.TokenPair{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), uc.cost)
	if err != nil {
		return domain.TokenPair{}, errs.Errf(errs.ErrInternal, "bcrypt: %s", err.Error())
	}

	pair, err := uc.issue(userID, a.Role)
	if err != nil {
		return domain.TokenPair{}, err
	}

	if err := uc.auth.SetCredentials(ctx, domain.UserAuth{
		UserID:           userID,
		PasswordHash:     string(hash),
		AccessTokenHash:  domain.HashToken(pair.AccessToken),
		RefreshTokenHash: domain.HashToken(pair.RefreshToken),
	}); err != nil {
		return domain.TokenPair{}, err
	}

	l := logger.WithContext(uc.l, ctx)

	if err := uc.verified.ConsumeVerified(context.WithoutCancel(ctx), phone, domain.OTPPurposeResetPassword); err != nil {
		l.Error("verified.ConsumeVerified", zap.Error(err))
	}

	l.Info("password reset", zap.String("user_id", userID))

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
