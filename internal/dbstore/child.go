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

const pgCheckViolation = "23514"

// Every query filters on deleted_at IS NULL: a soft-deleted child is invisible. Parent links live in
// user_children and are only read through active users and active children.
type childRepo struct {
	store sqlClientProvider
}

type dbChild struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	Age       int16     `db:"age"`
	Gender    string    `db:"gender"`
	PhotoID   *string   `db:"photo_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

const childColumns = `id, name, age, gender, photo_id, created_at, updated_at`

func (r *childRepo) Create(ctx context.Context, c domain.Child) (domain.Child, error) {
	l := logger.FromCtx(ctx, "childRepo.Create")

	q := `
		INSERT INTO children (name, age, gender, photo_id)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + childColumns

	return r.one(ctx, l, q, c.Name, c.Age, c.Gender, c.PhotoID)
}

func (r *childRepo) Get(ctx context.Context, id string) (domain.Child, error) {
	l := logger.FromCtx(ctx, "childRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+childColumns+` FROM children WHERE id = $1 AND deleted_at IS NULL`, id)
}

// List returns one page of active children and the total number matching f. With f.ParentID set,
// only that user's children are counted and listed.
func (r *childRepo) List(ctx context.Context, f domain.ChildFilter) ([]domain.Child, int, error) {
	l := logger.FromCtx(ctx, "childRepo.List")

	var (
		sqlClient  = r.store.sqlClientByCtx(ctx)
		from, args = childFrom(f)
		limitArg   = len(args) + 1
		countQ     = `SELECT COUNT(*) ` + from
		listQ      = `SELECT ` + prefixColumns("c", childColumns) + ` ` + from +
			` ORDER BY c.created_at DESC, c.id LIMIT $` + strconv.Itoa(limitArg) + ` OFFSET $` + strconv.Itoa(limitArg+1)
	)

	var total int
	if err := sqlClient.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, listQ, append(args, f.Limit, f.Offset)...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbChild])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Child, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, total, nil
}

// Update applies the non-nil fields of upd. PhotoID pointing to "" stores NULL.
func (r *childRepo) Update(ctx context.Context, id string, upd domain.ChildUpdate) (domain.Child, error) {
	l := logger.FromCtx(ctx, "childRepo.Update").With(zap.String("id", id))

	var (
		sets, args = childSet(upd)
		q          = `UPDATE children SET ` + sets +
			` WHERE id = $` + strconv.Itoa(len(args)+1) + ` AND deleted_at IS NULL RETURNING ` + childColumns
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the child. Its user_children links stay, but every read skips deleted children.
func (r *childRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "childRepo.Delete").With(zap.String("id", id))

	q := `UPDATE children SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrChildNotFound
	}

	return nil
}

// AddParent links an active user to an active child. Linking an existing pair again is a no-op.
func (r *childRepo) AddParent(ctx context.Context, childID, userID string) error {
	l := logger.FromCtx(ctx, "childRepo.AddParent").With(zap.String("child_id", childID), zap.String("user_id", userID))

	q := `
		WITH u AS (SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL) AS ok),
		     c AS (SELECT EXISTS (SELECT 1 FROM children WHERE id = $2 AND deleted_at IS NULL) AS ok),
		     ins AS (
				INSERT INTO user_children (user_id, child_id)
				SELECT $1, $2 FROM u, c WHERE u.ok AND c.ok
				ON CONFLICT DO NOTHING
			)
		SELECT u.ok, c.ok FROM u, c`

	var userOK, childOK bool
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, userID, childID).Scan(&userOK, &childOK); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	switch {
	case !childOK:
		return errs.ErrChildNotFound
	case !userOK:
		return errs.ErrUserNotFound
	default:
		return nil
	}
}

// RemoveParent unlinks a user from a child. A missing link is ErrChildNotFound: the user has no such child.
func (r *childRepo) RemoveParent(ctx context.Context, childID, userID string) error {
	l := logger.FromCtx(ctx, "childRepo.RemoveParent").With(zap.String("child_id", childID), zap.String("user_id", userID))

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx,
		`DELETE FROM user_children WHERE user_id = $1 AND child_id = $2`, userID, childID)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrChildNotFound
	}

	return nil
}

// Parents returns the active users linked to an active child, oldest link first. A missing or deleted
// child has no parents.
func (r *childRepo) Parents(ctx context.Context, childID string) ([]domain.User, error) {
	l := logger.FromCtx(ctx, "childRepo.Parents").With(zap.String("child_id", childID))

	q := `
		SELECT ` + prefixColumns("u", userColumns) + `
		FROM user_children uc
		JOIN users u ON u.id = uc.user_id AND u.deleted_at IS NULL
		JOIN children c ON c.id = uc.child_id AND c.deleted_at IS NULL
		WHERE uc.child_id = $1
		ORDER BY uc.created_at, u.id`

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, childID)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbUser])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.User, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// one runs q, which returns at most one child row (SELECT, or a write with RETURNING). No row means
// ErrChildNotFound; a photo_id with no media row means ErrPhotoNotFound; a failed CHECK means ErrValidation.
func (r *childRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Child, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var c dbChild

		c, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbChild])
		if err == nil {
			return c.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Child{}, errs.ErrChildNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "children_photo_id_fkey":
			return domain.Child{}, errs.ErrPhotoNotFound
		case pgErr.Code == pgCheckViolation:
			return domain.Child{}, errs.Errf(errs.ErrValidation, "%s", pgErr.ConstraintName)
		}
	}

	l.Error("childRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Child{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// childFrom builds the FROM ... WHERE part of a listing from fixed fragments.
func childFrom(f domain.ChildFilter) (string, []any) {
	if f.ParentID == "" {
		return `FROM children c WHERE c.deleted_at IS NULL`, nil
	}

	return `FROM children c JOIN user_children uc ON uc.child_id = c.id
		WHERE c.deleted_at IS NULL AND uc.user_id = $1`, []any{f.ParentID}
}

// childSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func childSet(upd domain.ChildUpdate) (string, []any) {
	var (
		sets = []string{"updated_at = NOW()"}
		args []any
	)

	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
	}

	if upd.Name != nil {
		add("name", *upd.Name)
	}

	if upd.Age != nil {
		add("age", *upd.Age)
	}

	if upd.Gender != nil {
		add("gender", *upd.Gender)
	}

	if upd.PhotoID != nil {
		add("photo_id", nullIfEmpty(*upd.PhotoID))
	}

	return strings.Join(sets, ", "), args
}

// prefixColumns qualifies a comma-separated column list with a table alias.
func prefixColumns(alias, columns string) string {
	parts := strings.Split(columns, ", ")
	for i := range parts {
		parts[i] = alias + "." + parts[i]
	}

	return strings.Join(parts, ", ")
}

func (c dbChild) toDomain() domain.Child {
	return domain.Child{
		ID:        c.ID,
		Name:      c.Name,
		Age:       int(c.Age),
		Gender:    c.Gender,
		PhotoID:   c.PhotoID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}
