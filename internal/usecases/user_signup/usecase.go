package usersignup

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type userRepo interface {
	Create(ctx context.Context, u domain.User) (domain.User, error)
}

type goalRepo interface {
	CountActive(ctx context.Context, ids []string) (int, error)
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

// UseCase creates a user and signs them in. The phone number is not verified, so the new user
// gets the unverified_user role.
type UseCase struct {
	l      logger.Logger
	tx     txRunner
	users  userRepo
	goals  goalRepo
	auth   authRepo
	tokens tokenIssuer
	cost   int
}

// New creates a new sign-up use case.
func New(l logger.Logger, tx txRunner, users userRepo, goals goalRepo, auth authRepo, tokens tokenIssuer) *UseCase {
	return &UseCase{
		l:      l,
		tx:     tx,
		users:  users,
		goals:  goals,
		auth:   auth,
		tokens: tokens,
		cost:   bcrypt.DefaultCost,
	}
}

// Execute checks the request, then stores the user and their user_auth row (bcrypt password,
// role unverified_user, token hashes) in one transaction and returns the tokens. Repeated goal ids
// are stored once; every goal id must be an active goal.
func (uc *UseCase) Execute(ctx context.Context, req domain.SignUpRequest) (domain.TokenPair, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.PhoneNumber = strings.TrimSpace(req.PhoneNumber)
	req.Relationship = strings.ToLower(strings.TrimSpace(req.Relationship))
	req.GoalIDs = domain.NormalizeIDs(req.GoalIDs)
	if err := validate(req); err != nil {
		return domain.TokenPair{}, err
	}

	if len(req.GoalIDs) > 0 {
		n, err := uc.goals.CountActive(ctx, req.GoalIDs)
		if err != nil {
			return domain.TokenPair{}, err
		}

		if n != len(req.GoalIDs) {
			return domain.TokenPair{}, errs.ErrUnknownGoals
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), uc.cost)
	if err != nil {
		return domain.TokenPair{}, errs.Errf(errs.ErrInternal, "bcrypt: %s", err.Error())
	}

	var pair domain.TokenPair

	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		user, err := uc.users.Create(ctx, domain.User{
			Name:         &req.Name,
			Username:     &req.Username,
			PhoneNumber:  &req.PhoneNumber,
			Relationship: &req.Relationship,
			GoalIDs:      req.GoalIDs,
		})
		if err != nil {
			return err
		}

		if pair, err = uc.issue(user.ID, domain.UserRoleUnverifiedUser); err != nil {
			return err
		}

		pair.UserID = user.ID

		return uc.auth.Create(ctx, domain.UserAuth{
			UserID:           user.ID,
			Username:         req.Username,
			PasswordHash:     string(hash),
			Role:             domain.UserRoleUnverifiedUser,
			AccessTokenHash:  domain.HashToken(pair.AccessToken),
			RefreshTokenHash: domain.HashToken(pair.RefreshToken),
		})
	})
	if err != nil {
		return domain.TokenPair{}, err
	}

	logger.WithContext(uc.l, ctx).Info("user signed up", zap.String("role", string(domain.UserRoleUnverifiedUser)))

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
	case !domain.IsKnownRelationship(req.Relationship):
		return errs.Errf(errs.ErrValidation, "relationship must be one of: father, mother, educator, nanny")
	case len(req.GoalIDs) > domain.MaxUserGoals:
		return errs.Errf(errs.ErrValidation, "goals can hold at most %d ids", domain.MaxUserGoals)
	}

	for _, id := range req.GoalIDs {
		if uuid.Validate(id) != nil {
			return errs.Errf(errs.ErrValidation, "goals must be goal ids (UUIDs), got %q", id)
		}
	}

	return nil
}
