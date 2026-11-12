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

// Soft-deleted milestones (deleted_at set) are invisible to every read; the use cases decide
// which reads see only published ones.
type milestoneRepo struct {
	store sqlClientProvider
}

type dbMilestone struct {
	ID           string    `db:"id"`
	Domain       string    `db:"domain"`
	AgeMonths    int16     `db:"age_months"`
	SortOrder    int32     `db:"sort_order"`
	TextUz       string    `db:"text_uz"`
	TextRu       string    `db:"text_ru"`
	TextEn       string    `db:"text_en"`
	MeaningUz    string    `db:"meaning_uz"`
	MeaningRu    string    `db:"meaning_ru"`
	MeaningEn    string    `db:"meaning_en"`
	HowToUz      string    `db:"how_to_uz"`
	HowToRu      string    `db:"how_to_ru"`
	HowToEn      string    `db:"how_to_en"`
	SpecialistUz string    `db:"specialist_uz"`
	SpecialistRu string    `db:"specialist_ru"`
	SpecialistEn string    `db:"specialist_en"`
	ActivityIDs  []string  `db:"activity_ids"`
	Status       string    `db:"status"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type dbChildMilestone struct {
	dbMilestone
	AchievedAt *time.Time `db:"achieved_at"`
}

// milestoneColumns selects from "milestones m".
const milestoneColumns = `m.id, m.domain, m.age_months, m.sort_order, m.text_uz, m.text_ru, m.text_en,
	m.meaning_uz, m.meaning_ru, m.meaning_en, m.how_to_uz, m.how_to_ru, m.how_to_en,
	m.specialist_uz, m.specialist_ru, m.specialist_en, m.activity_ids, m.status, m.created_at, m.updated_at`

// milestoneOrder is the order of every list: by stage, then the domain order of the app, then sort_order.
const milestoneOrder = ` ORDER BY m.age_months,
	array_position(ARRAY['communication', 'social', 'thinking', 'movement'], m.domain), m.sort_order, m.created_at, m.id`

func (r *milestoneRepo) Create(ctx context.Context, m domain.Milestone) (domain.Milestone, error) {
	l := logger.FromCtx(ctx, "milestoneRepo.Create")

	q := `
		INSERT INTO milestones AS m (domain, age_months, sort_order, text_uz, text_ru, text_en,
			meaning_uz, meaning_ru, meaning_en, how_to_uz, how_to_ru, how_to_en,
			specialist_uz, specialist_ru, specialist_en, activity_ids, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING ` + milestoneColumns

	return r.one(ctx, l, q, m.Domain, m.AgeMonths, m.SortOrder, m.TextUz, m.TextRu, m.TextEn,
		m.MeaningUz, m.MeaningRu, m.MeaningEn, m.HowToUz, m.HowToRu, m.HowToEn,
		m.SpecialistUz, m.SpecialistRu, m.SpecialistEn, nonNil(m.ActivityIDs), m.Status)
}

// Get returns the milestone in any status.
func (r *milestoneRepo) Get(ctx context.Context, id string) (domain.Milestone, error) {
	l := logger.FromCtx(ctx, "milestoneRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+milestoneColumns+` FROM milestones m WHERE m.id = $1 AND m.deleted_at IS NULL`, id)
}

// Update applies the non-nil fields of upd. A value that breaks a CHECK constraint is ErrValidation.
func (r *milestoneRepo) Update(ctx context.Context, id string, upd domain.MilestoneUpdate) (domain.Milestone, error) {
	l := logger.FromCtx(ctx, "milestoneRepo.Update").With(zap.String("id", id))

	var (
		sets, args = milestoneSet(upd)
		q          = `UPDATE milestones AS m SET ` + sets +
			` WHERE m.id = $` + strconv.Itoa(len(args)+1) + ` AND m.deleted_at IS NULL RETURNING ` + milestoneColumns
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the milestone. Children's ticks stay but are no longer shown.
func (r *milestoneRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "milestoneRepo.Delete").With(zap.String("id", id))

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx,
		`UPDATE milestones SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrMilestoneNotFound
	}

	return nil
}

// List returns one page of milestones matching f and the total number of matches.
func (r *milestoneRepo) List(ctx context.Context, f domain.MilestoneFilter) ([]domain.Milestone, int, error) {
	l := logger.FromCtx(ctx, "milestoneRepo.List")

	var (
		sqlClient   = r.store.sqlClientByCtx(ctx)
		where, args = milestoneWhere(f)
		countQ      = `SELECT COUNT(*) FROM milestones m` + where
		listQ       = `SELECT ` + milestoneColumns + ` FROM milestones m` + where + milestoneOrder +
			` LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2) //nolint:mnd
		total int
	)

	if err := sqlClient.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err), zap.String("query", countQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, listQ, append(args, f.Limit, f.Offset)...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err), zap.String("query", listQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbMilestone])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Milestone, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, total, nil
}

// Stages returns the distinct stages (age_months) of published milestones, ascending.
func (r *milestoneRepo) Stages(ctx context.Context) ([]int, error) {
	l := logger.FromCtx(ctx, "milestoneRepo.Stages")

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, `
		SELECT DISTINCT age_months::int FROM milestones
		WHERE deleted_at IS NULL AND status = 'published' ORDER BY 1`)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return out, nil
}

// ForChild returns the published milestones of one stage with the child's ticks.
func (r *milestoneRepo) ForChild(ctx context.Context, childID string, ageMonths int) ([]domain.ChildMilestone, error) {
	return r.childRows(ctx, "milestoneRepo.ForChild", `m.status = 'published' AND m.age_months = $2`, childID, ageMonths)
}

// GetForChild returns one milestone in any status with the child's tick.
func (r *milestoneRepo) GetForChild(ctx context.Context, id, childID string) (domain.ChildMilestone, error) {
	out, err := r.childRows(ctx, "milestoneRepo.GetForChild", `m.id = $2`, childID, id)
	if err != nil {
		return domain.ChildMilestone{}, err
	}

	if len(out) == 0 {
		return domain.ChildMilestone{}, errs.ErrMilestoneNotFound
	}

	return out[0], nil
}

// childRows runs a milestone query with the child's ticks; cond uses $1 (the child) and $2.
func (r *milestoneRepo) childRows(ctx context.Context, op, cond string, childID string, arg any) ([]domain.ChildMilestone, error) {
	l := logger.FromCtx(ctx, op).With(zap.String("child_id", childID))

	q := `SELECT ` + milestoneColumns + `, cm.achieved_at
		FROM milestones m
		LEFT JOIN child_milestones cm ON cm.milestone_id = m.id AND cm.child_id = $1::uuid
		WHERE m.deleted_at IS NULL AND ` + cond + milestoneOrder

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, childID, arg)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbChildMilestone])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.ChildMilestone, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, domain.ChildMilestone{
			Milestone: dbRows[i].toDomain(), Achieved: dbRows[i].AchievedAt != nil, AchievedAt: dbRows[i].AchievedAt,
		})
	}

	return out, nil
}

// Mark ticks the milestone for the child; ticking it again keeps the first time.
func (r *milestoneRepo) Mark(ctx context.Context, childID, milestoneID, userID string) error {
	l := logger.FromCtx(ctx, "milestoneRepo.Mark").With(zap.String("child_id", childID))

	_, err := r.store.sqlClientByCtx(ctx).Exec(ctx, `
		INSERT INTO child_milestones (child_id, milestone_id, user_id) VALUES ($1, $2, $3)
		ON CONFLICT (child_id, milestone_id) DO NOTHING`, childID, milestoneID, userID)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// Unmark removes the child's tick; a milestone that was not ticked changes nothing.
func (r *milestoneRepo) Unmark(ctx context.Context, childID, milestoneID string) error {
	l := logger.FromCtx(ctx, "milestoneRepo.Unmark").With(zap.String("child_id", childID))

	_, err := r.store.sqlClientByCtx(ctx).Exec(ctx,
		`DELETE FROM child_milestones WHERE child_id = $1 AND milestone_id = $2`, childID, milestoneID)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// CountAchieved counts the child's ticks of milestones that are not deleted, made in [from, to).
func (r *milestoneRepo) CountAchieved(ctx context.Context, childID string, from, to time.Time) (int, error) {
	l := logger.FromCtx(ctx, "milestoneRepo.CountAchieved").With(zap.String("child_id", childID))

	var n int
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, `
		SELECT COUNT(*) FROM child_milestones cm
		JOIN milestones m ON m.id = cm.milestone_id AND m.deleted_at IS NULL
		WHERE cm.child_id = $1 AND cm.achieved_at >= $2 AND cm.achieved_at < $3`, childID, from, to).Scan(&n); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return n, nil
}

// one runs q, which returns at most one milestone row. No row means ErrMilestoneNotFound; a
// failed CHECK means ErrValidation.
func (r *milestoneRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Milestone, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var m dbMilestone

		m, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbMilestone])
		if err == nil {
			return m.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Milestone{}, errs.ErrMilestoneNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgCheckViolation {
		return domain.Milestone{}, errs.Errf(errs.ErrValidation, "%s", pgErr.ConstraintName)
	}

	l.Error("milestoneRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Milestone{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// milestoneWhere builds a parameterized WHERE clause; values never enter the SQL text.
func milestoneWhere(f domain.MilestoneFilter) (string, []any) {
	var (
		conds = []string{"m.deleted_at IS NULL"}
		args  []any
	)

	add := func(cond func(n string) string, v any) {
		args = append(args, v)
		conds = append(conds, cond("$"+strconv.Itoa(len(args))))
	}

	if f.Domain != "" {
		add(func(n string) string { return "m.domain = " + n }, f.Domain)
	}

	if f.AgeMonths > 0 {
		add(func(n string) string { return "m.age_months = " + n }, f.AgeMonths)
	}

	if f.Status != "" {
		add(func(n string) string { return "m.status = " + n }, f.Status)
	}

	if f.Search != "" {
		add(func(n string) string {
			return "(m.text_uz ILIKE " + n + " OR m.text_ru ILIKE " + n + " OR m.text_en ILIKE " + n + ")"
		}, "%"+likeEscaper.Replace(f.Search)+"%")
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

// milestoneSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func milestoneSet(upd domain.MilestoneUpdate) (string, []any) {
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
		{"domain", upd.Domain}, {colStatus, upd.Status},
		{"text_uz", upd.TextUz}, {"text_ru", upd.TextRu}, {"text_en", upd.TextEn},
		{"meaning_uz", upd.MeaningUz}, {"meaning_ru", upd.MeaningRu}, {"meaning_en", upd.MeaningEn},
		{"how_to_uz", upd.HowToUz}, {"how_to_ru", upd.HowToRu}, {"how_to_en", upd.HowToEn},
		{"specialist_uz", upd.SpecialistUz}, {"specialist_ru", upd.SpecialistRu}, {"specialist_en", upd.SpecialistEn},
	} {
		if f.value != nil {
			add(f.column, *f.value)
		}
	}

	if upd.AgeMonths != nil {
		add("age_months", *upd.AgeMonths)
	}

	if upd.SortOrder != nil {
		add("sort_order", *upd.SortOrder)
	}

	if upd.ActivityIDs != nil {
		add("activity_ids", upd.ActivityIDs)
	}

	return strings.Join(sets, ", "), args
}

func (m dbMilestone) toDomain() domain.Milestone {
	return domain.Milestone{
		ID: m.ID, Domain: m.Domain, AgeMonths: int(m.AgeMonths), SortOrder: int(m.SortOrder),
		TextUz: m.TextUz, TextRu: m.TextRu, TextEn: m.TextEn,
		MeaningUz: m.MeaningUz, MeaningRu: m.MeaningRu, MeaningEn: m.MeaningEn,
		HowToUz: m.HowToUz, HowToRu: m.HowToRu, HowToEn: m.HowToEn,
		SpecialistUz: m.SpecialistUz, SpecialistRu: m.SpecialistRu, SpecialistEn: m.SpecialistEn,
		ActivityIDs: nonNil(m.ActivityIDs), Status: m.Status, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}
