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

// Soft-deleted activities (deleted_at set) are invisible to every read; completions keep pointing at them.
type activityRepo struct {
	store sqlClientProvider
}

type dbActivity struct {
	ID              string    `db:"id"`
	TitleUz         string    `db:"title_uz"`
	TitleRu         string    `db:"title_ru"`
	TitleEn         string    `db:"title_en"`
	DescriptionUz   string    `db:"description_uz"`
	DescriptionRu   string    `db:"description_ru"`
	DescriptionEn   string    `db:"description_en"`
	MainIdeaUz      string    `db:"main_idea_uz"`
	MainIdeaRu      string    `db:"main_idea_ru"`
	MainIdeaEn      string    `db:"main_idea_en"`
	EasierUz        string    `db:"easier_uz"`
	EasierRu        string    `db:"easier_ru"`
	EasierEn        string    `db:"easier_en"`
	HarderUz        string    `db:"harder_uz"`
	HarderRu        string    `db:"harder_ru"`
	HarderEn        string    `db:"harder_en"`
	Difficulty      string    `db:"difficulty"`
	PhotoID         *string   `db:"photo_id"`
	GoalIDs         []string  `db:"goal_ids"`
	MinAge          int16     `db:"min_age"`
	MaxAge          int16     `db:"max_age"`
	DurationMinutes int16     `db:"duration_minutes"`
	IsPublished     bool      `db:"is_published"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

const activityColumns = `
	id, title_uz, title_ru, title_en, description_uz, description_ru, description_en, main_idea_uz, main_idea_ru, main_idea_en,
	easier_uz, easier_ru, easier_en, harder_uz, harder_ru, harder_en, difficulty, photo_id,
	goal_ids, min_age, max_age, duration_minutes, is_published, created_at, updated_at`

func (r *activityRepo) Create(ctx context.Context, a domain.Activity) (domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.Create")

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		q         = `
		INSERT INTO activities (
			title_uz, title_ru, title_en, description_uz, description_ru, description_en,
			main_idea_uz, main_idea_ru, main_idea_en, easier_uz, easier_ru, easier_en, harder_uz, harder_ru, harder_en,
			goal_ids, min_age, max_age, duration_minutes, is_published, difficulty, photo_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, COALESCE(NULLIF($21, ''), 'normal'), $22)
		RETURNING ` + activityColumns
	)

	rows, err := sqlClient.Query(ctx, q,
		a.TitleUz, a.TitleRu, a.TitleEn, a.DescriptionUz, a.DescriptionRu, a.DescriptionEn,
		a.MainIdeaUz, a.MainIdeaRu, a.MainIdeaEn,
		a.EasierUz, a.EasierRu, a.EasierEn, a.HarderUz, a.HarderRu, a.HarderEn, a.GoalIDs,
		a.MinAge, a.MaxAge, a.DurationMinutes, a.IsPublished, a.Difficulty, photoOrNull(a.PhotoID),
	)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	created, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbActivity])
	if isPhotoFKError(err, "activities_photo_id_fkey") {
		return domain.Activity{}, errs.ErrPhotoNotFound
	}

	if err != nil {
		l.Error("pgx.CollectExactlyOneRow", zap.Error(err))

		return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return created.toDomain(), nil
}

func (r *activityRepo) Get(ctx context.Context, id string) (domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+activityColumns+` FROM activities WHERE id = $1 AND deleted_at IS NULL`, id)
}

// Update applies the non-nil fields of upd. A change that breaks a CHECK constraint (for example
// min_age above the stored max_age) is ErrValidation.
func (r *activityRepo) Update(ctx context.Context, id string, upd domain.ActivityUpdate) (domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.Update").With(zap.String("id", id))

	var (
		sets, args = activitySet(upd)
		q          = `UPDATE activities SET ` + sets +
			` WHERE id = $` + strconv.Itoa(len(args)+1) + ` AND deleted_at IS NULL RETURNING ` + activityColumns
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the activity.
func (r *activityRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "activityRepo.Delete").With(zap.String("id", id))

	q := `UPDATE activities SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrActivityNotFound
	}

	return nil
}

// one runs q, which returns at most one activity row (SELECT, or a write with RETURNING). No row means
// ErrActivityNotFound; a failed CHECK means ErrValidation.
func (r *activityRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Activity, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var a dbActivity

		a, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbActivity])
		if err == nil {
			return a.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Activity{}, errs.ErrActivityNotFound
	}

	if isPhotoFKError(err, "activities_photo_id_fkey") {
		return domain.Activity{}, errs.ErrPhotoNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgCheckViolation {
		if pgErr.ConstraintName == "activities_age_range_chk" {
			return domain.Activity{}, errs.Errf(errs.ErrValidation,
				"age range must be within %d..%d and min_age <= max_age, together with the stored values",
				domain.MinChildAge, domain.MaxChildAge)
		}

		return domain.Activity{}, errs.Errf(errs.ErrValidation, "%s", pgErr.ConstraintName)
	}

	l.Error("activityRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// List returns one page of activities matching f and the total number of matches.
func (r *activityRepo) List(ctx context.Context, f domain.ActivityFilter) ([]domain.Activity, int, error) {
	l := logger.FromCtx(ctx, "activityRepo.List")

	var (
		sqlClient   = r.store.sqlClientByCtx(ctx)
		where, args = activityWhere(f)
		countQ      = `SELECT COUNT(*) FROM activities` + where
		listQ       = `SELECT ` + activityColumns + ` FROM activities` + where +
			` ORDER BY created_at DESC, id` +
			` LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2) //nolint:mnd
	)

	var total int
	if err := sqlClient.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err), zap.String("query", countQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, listQ, append(args, f.Limit, f.Offset)...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err), zap.String("query", listQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbActivity])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Activity, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, total, nil
}

// activityWhere builds a parameterized WHERE clause; values never enter the SQL text.
func activityWhere(f domain.ActivityFilter) (string, []any) {
	var (
		conds = []string{"deleted_at IS NULL"}
		args  []any
	)

	if f.PublishedOnly || f.Status == domain.ActivityStatusPublished {
		conds = append(conds, "is_published")
	}

	if f.Status == domain.ActivityStatusDraft {
		conds = append(conds, "NOT is_published")
	}

	if f.Search != "" {
		args = append(args, "%"+likeEscaper.Replace(f.Search)+"%")
		n := strconv.Itoa(len(args))
		conds = append(conds, "(title_uz ILIKE $"+n+" OR title_ru ILIKE $"+n+" OR title_en ILIKE $"+n+")")
	}

	if f.GoalID != "" {
		args = append(args, f.GoalID)
		conds = append(conds, "$"+strconv.Itoa(len(args))+"::uuid = ANY(goal_ids)")
	}

	if f.Age > 0 {
		args = append(args, f.Age)
		n := strconv.Itoa(len(args))
		conds = append(conds, "min_age <= $"+n+" AND max_age >= $"+n)
	}

	if f.MaxMinutes > 0 {
		args = append(args, f.MaxMinutes)
		conds = append(conds, "duration_minutes <= $"+strconv.Itoa(len(args)))
	}

	if f.Difficulty != "" {
		args = append(args, f.Difficulty)
		conds = append(conds, "difficulty = $"+strconv.Itoa(len(args)))
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

// likeEscaper escapes the LIKE wildcards, so a search for "50%" matches the text literally.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// activitySet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func activitySet(upd domain.ActivityUpdate) (string, []any) {
	var (
		sets = []string{setUpdatedAt}
		args []any
	)

	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, column+" = $"+strconv.Itoa(len(args)))
	}

	if upd.TitleUz != nil {
		add("title_uz", *upd.TitleUz)
	}

	if upd.TitleRu != nil {
		add("title_ru", *upd.TitleRu)
	}

	if upd.TitleEn != nil {
		add("title_en", *upd.TitleEn)
	}

	if upd.DescriptionUz != nil {
		add("description_uz", *upd.DescriptionUz)
	}

	if upd.DescriptionRu != nil {
		add("description_ru", *upd.DescriptionRu)
	}

	if upd.DescriptionEn != nil {
		add("description_en", *upd.DescriptionEn)
	}

	if upd.MainIdeaUz != nil {
		add("main_idea_uz", *upd.MainIdeaUz)
	}

	if upd.MainIdeaRu != nil {
		add("main_idea_ru", *upd.MainIdeaRu)
	}

	if upd.MainIdeaEn != nil {
		add("main_idea_en", *upd.MainIdeaEn)
	}

	for _, f := range []struct {
		column string
		value  *string
	}{
		{"easier_uz", upd.EasierUz}, {"easier_ru", upd.EasierRu}, {"easier_en", upd.EasierEn},
		{"harder_uz", upd.HarderUz}, {"harder_ru", upd.HarderRu}, {"harder_en", upd.HarderEn},
		{"difficulty", upd.Difficulty},
	} {
		if f.value != nil {
			add(f.column, *f.value)
		}
	}

	if upd.PhotoID != nil {
		add("photo_id", nullIfEmpty(*upd.PhotoID))
	}

	if upd.GoalIDs != nil {
		add("goal_ids", upd.GoalIDs)
	}

	if upd.MinAge != nil {
		add("min_age", *upd.MinAge)
	}

	if upd.MaxAge != nil {
		add("max_age", *upd.MaxAge)
	}

	if upd.DurationMinutes != nil {
		add("duration_minutes", *upd.DurationMinutes)
	}

	if upd.IsPublished != nil {
		add("is_published", *upd.IsPublished)
	}

	return strings.Join(sets, ", "), args
}

func (a dbActivity) toDomain() domain.Activity {
	return domain.Activity{
		ID:              a.ID,
		TitleUz:         a.TitleUz,
		TitleRu:         a.TitleRu,
		TitleEn:         a.TitleEn,
		DescriptionUz:   a.DescriptionUz,
		DescriptionRu:   a.DescriptionRu,
		DescriptionEn:   a.DescriptionEn,
		MainIdeaUz:      a.MainIdeaUz,
		MainIdeaRu:      a.MainIdeaRu,
		MainIdeaEn:      a.MainIdeaEn,
		EasierUz:        a.EasierUz,
		EasierRu:        a.EasierRu,
		EasierEn:        a.EasierEn,
		HarderUz:        a.HarderUz,
		HarderRu:        a.HarderRu,
		HarderEn:        a.HarderEn,
		Difficulty:      a.Difficulty,
		PhotoID:         a.PhotoID,
		GoalIDs:         nonNil(a.GoalIDs),
		MinAge:          int(a.MinAge),
		MaxAge:          int(a.MaxAge),
		DurationMinutes: int(a.DurationMinutes),
		IsPublished:     a.IsPublished,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
	}
}

// nonNil turns a nil slice into an empty one: pgx sends nil as NULL, and JSON shows it as null.
func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}

	return ids
}

// Similar returns published activities sharing a goal with f.ActivityID, not that activity and
// not in f.ExcludeIDs. With
// f.ChildID they fit f.Age and the ones the child never did (then did longest ago) come first;
// without it they overlap f.MinAge..f.MaxAge. Then more shared goals, then newest first.
func (r *activityRepo) Similar(ctx context.Context, f domain.SimilarFilter) ([]domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.Similar").With(zap.String("id", f.ActivityID))

	minAge, maxAge := f.MinAge, f.MaxAge
	if f.ChildID != "" {
		minAge, maxAge = f.Age, f.Age
	}

	q := `
		SELECT ` + prefixColumns("a", activityColumns) + `
		FROM activities a
		LEFT JOIN (
			SELECT activity_id, MAX(completed_on) AS last_on
			FROM activity_completions WHERE child_id = $1::uuid
			GROUP BY activity_id
		) done ON done.activity_id = a.id
		WHERE a.is_published AND a.deleted_at IS NULL AND a.id <> $2::uuid AND a.goal_ids && $3::uuid[]
			AND a.min_age <= $5 AND a.max_age >= $4 AND NOT (a.id = ANY($7::uuid[]))
		ORDER BY done.last_on NULLS FIRST,
			(SELECT COUNT(*) FROM unnest(a.goal_ids) g WHERE g = ANY($3::uuid[])) DESC,
			a.created_at DESC, a.id
		LIMIT $6`

	var childID *string
	if f.ChildID != "" {
		childID = &f.ChildID
	}

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, childID, f.ActivityID, nonNil(f.GoalIDs), minAge, maxAge, f.Limit, nonNil(f.ExcludeIDs))
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbActivity])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Activity, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// ListByIDs returns the activities among ids that are not deleted, in the order of ids. With
// publishedOnly, unpublished ones are left out too.
func (r *activityRepo) ListByIDs(ctx context.Context, ids []string, publishedOnly bool) ([]domain.Activity, error) {
	l := logger.FromCtx(ctx, "activityRepo.ListByIDs")

	q := `SELECT ` + prefixColumns("a", activityColumns) + `
		FROM activities a
		WHERE a.id = ANY($1::uuid[]) AND a.deleted_at IS NULL AND (a.is_published OR NOT $2)
		ORDER BY array_position($1::uuid[], a.id)`

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, nonNil(ids), publishedOnly)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbActivity])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Activity, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// photoOrNull turns a missing or empty photo id into NULL.
func photoOrNull(id *string) *string {
	if id == nil {
		return nil
	}

	return nullIfEmpty(*id)
}

// isPhotoFKError reports whether err is a foreign key violation of the given photo constraint,
// that is, a photo_id that is not an uploaded media id.
func isPhotoFKError(err error, constraint string) bool {
	var pgErr *pgconn.PgError

	return errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == constraint
}
