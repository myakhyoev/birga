package dbstore

import (
	"context"
	"errors"

	pgx "github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// authRepo reads and writes user_auth. A soft-deleted user has no row (users_soft_delete_auth_trg).
type authRepo struct {
	store sqlClientProvider
}

// Create inserts the user's auth row. The user row must exist (same transaction is fine).
func (r *authRepo) Create(ctx context.Context, a domain.UserAuth) error {
	l := logger.FromCtx(ctx, "authRepo.Create").With(zap.String("user_id", a.UserID))

	q := `
		INSERT INTO user_auth (id, username, password, role, access_token, refresh_token)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q,
		a.UserID, a.Username, a.PasswordHash, string(a.Role), nullIfEmpty(a.AccessTokenHash), nullIfEmpty(a.RefreshTokenHash))
	if err != nil {
		if conflict := userConflict(err); conflict != nil {
			return conflict
		}

		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// Get returns the auth row of an active user, or ErrUserNotFound.
func (r *authRepo) Get(ctx context.Context, userID string) (domain.UserAuth, error) {
	l := logger.FromCtx(ctx, "authRepo.Get").With(zap.String("user_id", userID))

	q := `
		SELECT id, COALESCE(username, ''), COALESCE(password, ''), role::TEXT,
			COALESCE(access_token, ''), COALESCE(refresh_token, '')
		FROM user_auth WHERE id = $1`

	var a domain.UserAuth

	err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, userID).
		Scan(&a.UserID, &a.Username, &a.PasswordHash, &a.Role, &a.AccessTokenHash, &a.RefreshTokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserAuth{}, errs.ErrUserNotFound
	}

	if err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return domain.UserAuth{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return a, nil
}

// SetAccessToken stores the hash of the user's newest access token.
func (r *authRepo) SetAccessToken(ctx context.Context, userID, tokenHash string) error {
	l := logger.FromCtx(ctx, "authRepo.SetAccessToken").With(zap.String("user_id", userID))

	q := `UPDATE user_auth SET access_token = $2, updated_at = NOW() WHERE id = $1`

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q, userID, tokenHash)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrUserNotFound
	}

	return nil
}
