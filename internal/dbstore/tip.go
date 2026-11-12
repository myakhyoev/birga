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

// Soft-deleted tips (deleted_at set) are invisible to every read. The use cases decide which
// reads see only published tips.
type tipRepo struct {
	store sqlClientProvider
}

type dbTip struct {
	ID        string    `db:"id"`
	TextUz    string    `db:"text_uz"`
	TextRu    string    `db:"text_ru"`
	TextEn    string    `db:"text_en"`
	TopicID   string    `db:"topic_id"`
	MinAge    int16     `db:"min_age"`
	MaxAge    int16     `db:"max_age"`
	Status    string    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Locked    bool      `db:"locked"`
}

// tipColumns selects from "tips t"; locked is false unless the query asks for it.
const tipColumns = `t.id, t.text_uz, t.text_ru, t.text_en, t.topic_id, t.min_age, t.max_age, t.status,
	t.created_at, t.updated_at`

const notLocked = `, FALSE AS locked`

func (r *tipRepo) Create(ctx context.Context, t domain.Tip) (domain.Tip, error) {
	l := logger.FromCtx(ctx, "tipRepo.Create")

	q := `
		INSERT INTO tips AS t (text_uz, text_ru, text_en, topic_id, min_age, max_age, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + tipColumns + notLocked

	return r.one(ctx, l, q, t.TextUz, t.TextRu, t.TextEn, t.TopicID, t.MinAge, t.MaxAge, t.Status)
}

// Get returns the tip in any status.
func (r *tipRepo) Get(ctx context.Context, id string) (domain.Tip, error) {
	l := logger.FromCtx(ctx, "tipRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+tipColumns+notLocked+` FROM tips t WHERE t.id = $1 AND t.deleted_at IS NULL`, id)
}

// Update applies the non-nil fields of upd. A change that breaks a CHECK constraint is ErrValidation.
func (r *tipRepo) Update(ctx context.Context, id string, upd domain.TipUpdate) (domain.Tip, error) {
	l := logger.FromCtx(ctx, "tipRepo.Update").With(zap.String("id", id))

	var (
		sets, args = tipSet(upd)
		q          = `UPDATE tips AS t SET ` + sets +
			` WHERE t.id = $` + strconv.Itoa(len(args)+1) + ` AND t.deleted_at IS NULL RETURNING ` + tipColumns + notLocked
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the tip.
func (r *tipRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "tipRepo.Delete").With(zap.String("id", id))

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx,
		`UPDATE tips SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrTipNotFound
	}

	return nil
}

// List returns one page of tips matching f and the total number of matches, newest first. With
// f.LockFor, tips that were never that user's daily tip come back Locked with empty texts.
func (r *tipRepo) List(ctx context.Context, f domain.TipFilter) ([]domain.Tip, int, error) {
	l := logger.FromCtx(ctx, "tipRepo.List")

	var (
		sqlClient   = r.store.sqlClientByCtx(ctx)
		where, args = tipWhere(f)
		countQ      = `SELECT COUNT(*) FROM tips t` + where
		cols        = tipColumns + notLocked
		total       int
	)

	if err := sqlClient.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err), zap.String("query", countQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	listArgs := args
	if f.LockFor != "" {
		listArgs = append(listArgs, f.LockFor)
		p := "$" + strconv.Itoa(len(listArgs))
		cols = tipColumns + `, NOT EXISTS (SELECT 1 FROM user_daily_tips d WHERE d.tip_id = t.id AND d.user_id = ` +
			p + `::uuid) AS locked`
	}

	listQ := `SELECT ` + cols + ` FROM tips t` + where + ` ORDER BY t.created_at DESC, t.id` +
		` LIMIT $` + strconv.Itoa(len(listArgs)+1) + ` OFFSET $` + strconv.Itoa(len(listArgs)+2) //nolint:mnd

	out, err := r.many(ctx, l, listQ, append(listArgs, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}

	for i := range out {
		if out[i].Locked {
			out[i].TextUz, out[i].TextRu, out[i].TextEn = "", "", ""
		}
	}

	return out, total, nil
}

func (r *tipRepo) daily() dailyPicks {
	return dailyPicks{store: r.store, content: "tips", daily: "user_daily_tips", idColumn: "tip_id", topicColumn: "topic_id"}
}

// DailyTipID returns the tip stored for the user's day, if it is still published.
func (r *tipRepo) DailyTipID(ctx context.Context, userID string, day time.Time) (string, bool, error) {
	return r.daily().storedID(ctx, userID, day)
}

// SetDailyTip stores tipID as the user's tip for day, replacing an earlier pick.
func (r *tipRepo) SetDailyTip(ctx context.Context, userID string, day time.Time, tipID string) error {
	return r.daily().set(ctx, userID, day, tipID)
}

// LockDaily serializes daily tip picks of one user until the surrounding transaction ends.
func (r *tipRepo) LockDaily(ctx context.Context, userID string) error {
	return r.daily().lock(ctx, userID)
}

// PickDaily chooses the next daily tip for the user: age fit, not shown before, shown longest
// ago, the topic shown longest ago, oldest first (see dailyPicks.pick). ok is false when no
// published tip exists.
func (r *tipRepo) PickDaily(ctx context.Context, userID string) (string, bool, error) {
	return r.daily().pick(ctx, userID, nil)
}

func (r *tipRepo) many(ctx context.Context, l *zap.Logger, q string, args ...any) ([]domain.Tip, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err), zap.String("query", q))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbTip])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Tip, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// one runs q, which returns at most one tip row. No row means ErrTipNotFound; an unknown topic_id
// is ErrUnknownTopic; a failed CHECK means ErrValidation.
func (r *tipRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Tip, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var t dbTip

		t, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbTip])
		if err == nil {
			return t.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tip{}, errs.ErrTipNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "tips_topic_id_fkey":
			return domain.Tip{}, errs.ErrUnknownTopic
		case pgErr.Code == pgCheckViolation && pgErr.ConstraintName == "tips_age_range_chk":
			return domain.Tip{}, errs.Errf(errs.ErrValidation,
				"age range must be within %d..%d and min_age <= max_age, together with the stored values",
				domain.MinChildAge, domain.MaxChildAge)
		case pgErr.Code == pgCheckViolation:
			return domain.Tip{}, errs.Errf(errs.ErrValidation, "%s", pgErr.ConstraintName)
		}
	}

	l.Error("tipRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Tip{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// tipWhere builds a parameterized WHERE clause; values never enter the SQL text.
func tipWhere(f domain.TipFilter) (string, []any) {
	var (
		conds = []string{"t.deleted_at IS NULL"}
		args  []any
	)

	add := func(cond func(n string) string, v any) {
		args = append(args, v)
		conds = append(conds, cond("$"+strconv.Itoa(len(args))))
	}

	if f.Status != "" {
		add(func(n string) string { return "t.status = " + n }, f.Status)
	}

	if f.Search != "" {
		add(func(n string) string {
			return "(t.text_uz ILIKE " + n + " OR t.text_ru ILIKE " + n + " OR t.text_en ILIKE " + n + ")"
		}, "%"+likeEscaper.Replace(f.Search)+"%")
	}

	if f.TopicID != "" {
		add(func(n string) string { return "t.topic_id = " + n + "::uuid" }, f.TopicID)
	}

	if f.Age > 0 {
		add(func(n string) string { return "t.min_age <= " + n + " AND t.max_age >= " + n }, f.Age)
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

// tipSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func tipSet(upd domain.TipUpdate) (string, []any) {
	var (
		sets = []string{setUpdatedAt}
		args []any
	)

	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
	}

	for _, f := range []struct {
		column string
		value  *string
	}{
		{"text_uz", upd.TextUz}, {"text_ru", upd.TextRu}, {"text_en", upd.TextEn},
		{"topic_id", upd.TopicID}, {colStatus, upd.Status},
	} {
		if f.value != nil {
			add(f.column, *f.value)
		}
	}

	if upd.MinAge != nil {
		add("min_age", *upd.MinAge)
	}

	if upd.MaxAge != nil {
		add("max_age", *upd.MaxAge)
	}

	return strings.Join(sets, ", "), args
}

func (t dbTip) toDomain() domain.Tip {
	return domain.Tip{
		ID:        t.ID,
		TextUz:    t.TextUz,
		TextRu:    t.TextRu,
		TextEn:    t.TextEn,
		TopicID:   t.TopicID,
		MinAge:    int(t.MinAge),
		MaxAge:    int(t.MaxAge),
		Status:    t.Status,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		Locked:    t.Locked,
	}
}
