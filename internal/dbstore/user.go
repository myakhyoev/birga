package dbstore

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

const pgUniqueViolation = "23505"

// Every query filters on deleted_at IS NULL: a soft-deleted user is invisible.
type userRepo struct {
	store sqlClientProvider
}

type dbUser struct {
	ID          string    `db:"id"`
	Name        *string   `db:"name"`
	Username    *string   `db:"username"`
	PhoneNumber *string   `db:"phone_number"`
	PhotoID     *string   `db:"photo_id"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

const userColumns = `id, name, username, phone_number, photo_id, created_at, updated_at`

func (r *userRepo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	l := logger.FromCtx(ctx, "userRepo.Create")

	q := `
		INSERT INTO users (name, username, phone_number, photo_id)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + userColumns

	return r.one(ctx, l, q, u.Name, u.Username, u.PhoneNumber, u.PhotoID)
}

func (r *userRepo) Get(ctx context.Context, id string) (domain.User, error) {
	l := logger.FromCtx(ctx, "userRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id)
}

// List returns one page of active users and the total number of active users.
func (r *userRepo) List(ctx context.Context, f domain.UserFilter) ([]domain.User, int, error) {
	l := logger.FromCtx(ctx, "userRepo.List")

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		countQ    = `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`
		listQ     = `SELECT ` + userColumns + ` FROM users WHERE deleted_at IS NULL
			ORDER BY created_at DESC, id LIMIT $1 OFFSET $2`
	)

	var total int
	if err := sqlClient.QueryRow(ctx, countQ).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, listQ, f.Limit, f.Offset)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbUser])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.User, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, total, nil
}

// Update applies the non-nil fields of upd. A field pointing to "" is stored as NULL.
func (r *userRepo) Update(ctx context.Context, id string, upd domain.UserUpdate) (domain.User, error) {
	l := logger.FromCtx(ctx, "userRepo.Update").With(zap.String("id", id))

	var (
		sets, args = userSet(upd)
		q          = `UPDATE users SET ` + sets +
			` WHERE id = $` + strconv.Itoa(len(args)+1) + ` AND deleted_at IS NULL RETURNING ` + userColumns
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// one runs q, which returns at most one user row (SELECT, or a write with RETURNING). No row means ErrUserNotFound;
// a unique-index violation means the matching conflict error.
func (r *userRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.User, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var u dbUser

		u, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbUser])
		if err == nil {
			return u.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, errs.ErrUserNotFound
	}

	if conflict := userConflict(err); conflict != nil {
		return domain.User{}, conflict
	}

	l.Error("userRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.User{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// Delete soft-deletes the user. The users_soft_delete_auth_trg trigger removes the user_auth row.
func (r *userRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "userRepo.Delete").With(zap.String("id", id))

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		q         = `UPDATE users SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	)

	tag, err := sqlClient.Exec(ctx, q, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrUserNotFound
	}

	return nil
}

// userSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func userSet(upd domain.UserUpdate) (string, []any) {
	var (
		sets = []string{"updated_at = NOW()"}
		args []any
	)

	for _, f := range []struct {
		column string
		value  *string
	}{
		{"name", upd.Name},
		{"username", upd.Username},
		{"phone_number", upd.PhoneNumber},
		{"photo_id", upd.PhotoID},
	} {
		if f.value == nil {
			continue
		}

		args = append(args, nullIfEmpty(*f.value))
		sets = append(sets, f.column+" = $"+strconv.Itoa(len(args)))
	}

	return strings.Join(sets, ", "), args
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// userConflict maps a unique-index violation to the matching conflict error, or returns nil.
func userConflict(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgUniqueViolation {
		return nil
	}

	switch pgErr.ConstraintName {
	case "users_username_uniq":
		return errs.ErrUsernameTaken
	case "users_phone_number_uniq":
		return errs.ErrPhoneNumberTaken
	default:
		return errs.Errf(errs.ErrConflict, "%s", pgErr.Message)
	}
}

func (u dbUser) toDomain() domain.User {
	return domain.User{
		ID:          u.ID,
		Name:        u.Name,
		Username:    u.Username,
		PhoneNumber: u.PhoneNumber,
		PhotoID:     u.PhotoID,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

// ExistsByPhone reports whether an active user has this phone number.
func (r *userRepo) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	l := logger.FromCtx(ctx, "userRepo.ExistsByPhone")

	var exists bool

	q := `SELECT EXISTS (SELECT 1 FROM users WHERE phone_number = $1 AND deleted_at IS NULL)`
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, phone).Scan(&exists); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return exists, nil
}
