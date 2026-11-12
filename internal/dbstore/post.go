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

// Soft-deleted posts (deleted_at set) are invisible to every read. Which posts the app may see is
// decided by the use cases.
type postRepo struct {
	store sqlClientProvider
}

type dbPost struct {
	ID           string    `db:"id"`
	TitleUz      string    `db:"title_uz"`
	TitleRu      string    `db:"title_ru"`
	TitleEn      string    `db:"title_en"`
	VideoID      string    `db:"video_id"`
	PhotoID      *string   `db:"photo_id"`
	GoalID       string    `db:"goal_id"`
	TopicID      *string   `db:"topic_id"`
	MinAge       int16     `db:"min_age"`
	MaxAge       int16     `db:"max_age"`
	Duration     float64   `db:"duration"`
	SuggestionUz string    `db:"suggestion_uz"`
	SuggestionRu string    `db:"suggestion_ru"`
	SuggestionEn string    `db:"suggestion_en"`
	QuestionsUz  string    `db:"questions_uz"`
	QuestionsRu  string    `db:"questions_ru"`
	QuestionsEn  string    `db:"questions_en"`
	Status       string    `db:"status"`
	Source       string    `db:"source"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

// postColumns selects from "posts p"; duration is read as float8.
const postColumns = `
	p.id, p.title_uz, p.title_ru, p.title_en, p.video_id, p.photo_id, p.goal_id, p.topic_id, p.min_age, p.max_age,
	p.duration::float8 AS duration, p.suggestion_uz, p.suggestion_ru, p.suggestion_en,
	p.questions_uz, p.questions_ru, p.questions_en, p.status, p.source, p.created_at, p.updated_at`

func (r *postRepo) Create(ctx context.Context, p domain.Post) (domain.Post, error) {
	l := logger.FromCtx(ctx, "postRepo.Create")

	q := `
		INSERT INTO posts AS p (
			title_uz, title_ru, title_en, video_id, goal_id, min_age, max_age, duration,
			suggestion_uz, suggestion_ru, suggestion_en, questions_uz, questions_ru, questions_en, status, topic_id, photo_id, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, COALESCE(NULLIF($18, ''), 'admin'))
		RETURNING ` + postColumns

	return r.one(ctx, l, q,
		p.TitleUz, p.TitleRu, p.TitleEn, p.VideoID, p.GoalID, p.MinAge, p.MaxAge, p.Duration,
		p.SuggestionUz, p.SuggestionRu, p.SuggestionEn, p.QuestionsUz, p.QuestionsRu, p.QuestionsEn, p.Status, topicOrNull(p.TopicID), photoOrNull(p.PhotoID), p.Source,
	)
}

// Get returns the post in any status.
func (r *postRepo) Get(ctx context.Context, id string) (domain.Post, error) {
	l := logger.FromCtx(ctx, "postRepo.Get").With(zap.String("id", id))

	return r.one(ctx, l, `SELECT `+postColumns+` FROM posts p WHERE p.id = $1 AND p.deleted_at IS NULL`, id)
}

// Update applies the non-nil fields of upd. A change that breaks a CHECK constraint is ErrValidation.
func (r *postRepo) Update(ctx context.Context, id string, upd domain.PostUpdate) (domain.Post, error) {
	l := logger.FromCtx(ctx, "postRepo.Update").With(zap.String("id", id))

	var (
		sets, args = postSet(upd)
		q          = `UPDATE posts AS p SET ` + sets +
			` WHERE p.id = $` + strconv.Itoa(len(args)+1) + ` AND p.deleted_at IS NULL RETURNING ` + postColumns
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the post.
func (r *postRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "postRepo.Delete").With(zap.String("id", id))

	q := `UPDATE posts SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrPostNotFound
	}

	return nil
}

// List returns one page of posts matching f and the total number of matches, newest first.
func (r *postRepo) List(ctx context.Context, f domain.PostFilter) ([]domain.Post, int, error) {
	l := logger.FromCtx(ctx, "postRepo.List")

	var (
		sqlClient   = r.store.sqlClientByCtx(ctx)
		where, args = postWhere(f)
		countQ      = `SELECT COUNT(*) FROM posts p` + where
		listQ       = `SELECT ` + postColumns + ` FROM posts p` + where +
			` ORDER BY p.created_at DESC, p.id` +
			` LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2) //nolint:mnd
		total int
	)

	if err := sqlClient.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err), zap.String("query", countQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out, err := r.many(ctx, l, listQ, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// WasDaily reports whether the post was the user's daily video on day or before.
func (r *postRepo) WasDaily(ctx context.Context, userID, postID string, day time.Time) (bool, error) {
	return r.daily().wasDaily(ctx, userID, postID, day)
}

// DailyIDs returns which of ids were the user's daily video on day or earlier.
func (r *postRepo) DailyIDs(ctx context.Context, userID string, ids []string, day time.Time) (map[string]bool, error) {
	return r.daily().dailyIDs(ctx, userID, ids, day)
}

func (r *postRepo) daily() dailyPicks {
	return dailyPicks{store: r.store, content: "posts", daily: "user_daily_posts", idColumn: "post_id"}
}

// DailyPostID returns the post stored for the user's day, if it is still published.
func (r *postRepo) DailyPostID(ctx context.Context, userID string, day time.Time) (string, bool, error) {
	return r.daily().storedID(ctx, userID, day)
}

// SetDailyPost stores postID as the user's video for day, replacing an earlier pick.
func (r *postRepo) SetDailyPost(ctx context.Context, userID string, day time.Time, postID string) error {
	return r.daily().set(ctx, userID, day, postID)
}

// LockDaily serializes daily video picks of one user until the surrounding transaction ends.
func (r *postRepo) LockDaily(ctx context.Context, userID string) error {
	return r.daily().lock(ctx, userID)
}

// PickDaily chooses the next daily video for the user, leaving out the ids in exclude; see
// dailyPicks.pick for the order. ok is false when no published post is left.
func (r *postRepo) PickDaily(ctx context.Context, userID string, exclude []string) (string, bool, error) {
	return r.daily().pick(ctx, userID, exclude)
}

// DailyHistory returns one page of the user's daily videos up to and including day, newest
// first, and their total. Posts deleted since are left out.
func (r *postRepo) DailyHistory(ctx context.Context, userID string, day time.Time, limit, offset int,
) ([]domain.DailyPost, int, error) {
	l := logger.FromCtx(ctx, "postRepo.DailyHistory")

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		from      = ` FROM user_daily_posts d JOIN posts p ON p.id = d.post_id AND p.deleted_at IS NULL
			WHERE d.user_id = $1 AND d.day <= $2`
		total int
	)

	if err := sqlClient.QueryRow(ctx, `SELECT COUNT(*)`+from, userID, day).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, `SELECT d.day, `+postColumns+from+` ORDER BY d.day DESC LIMIT $3 OFFSET $4`,
		userID, day, limit, offset)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	type dbDaily struct {
		Day time.Time `db:"day"`
		dbPost
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbDaily])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.DailyPost, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, domain.DailyPost{Day: dbRows[i].Day, Post: dbRows[i].toDomain()})
	}

	return out, total, nil
}

func (r *postRepo) many(ctx context.Context, l *zap.Logger, q string, args ...any) ([]domain.Post, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err), zap.String("query", q))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbPost])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Post, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// one runs q, which returns at most one post row. No row means ErrPostNotFound; an unknown
// video_id is ErrVideoNotFound; an unknown goal_id is ErrUnknownGoals; a failed CHECK means ErrValidation.
func (r *postRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Post, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var p dbPost

		p, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbPost])
		if err == nil {
			return p.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Post{}, errs.ErrPostNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "posts_video_id_fkey":
			return domain.Post{}, errs.ErrVideoNotFound
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "posts_photo_id_fkey":
			return domain.Post{}, errs.ErrPhotoNotFound
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "posts_topic_id_fkey":
			return domain.Post{}, errs.ErrUnknownTopic
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "posts_goal_id_fkey":
			return domain.Post{}, errs.ErrUnknownGoals
		case pgErr.Code == pgCheckViolation && pgErr.ConstraintName == "posts_age_range_chk":
			return domain.Post{}, errs.Errf(errs.ErrValidation,
				"age range must be within %d..%d and min_age <= max_age, together with the stored values",
				domain.MinChildAge, domain.MaxChildAge)
		case pgErr.Code == pgCheckViolation:
			return domain.Post{}, errs.Errf(errs.ErrValidation, "%s", pgErr.ConstraintName)
		}
	}

	l.Error("postRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Post{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// postWhere builds a parameterized WHERE clause; values never enter the SQL text.
func postWhere(f domain.PostFilter) (string, []any) {
	var (
		conds = []string{"p.deleted_at IS NULL"}
		args  []any
	)

	add := func(cond func(n string) string, v any) {
		args = append(args, v)
		conds = append(conds, cond("$"+strconv.Itoa(len(args))))
	}

	if f.Status != "" {
		add(func(n string) string { return "p.status = " + n }, f.Status)
	}

	if f.Source != "" {
		add(func(n string) string { return "p.source = " + n }, f.Source)
	}

	if f.Search != "" {
		add(func(n string) string {
			return "(p.title_uz ILIKE " + n + " OR p.title_ru ILIKE " + n + " OR p.title_en ILIKE " + n + ")"
		}, "%"+likeEscaper.Replace(f.Search)+"%")
	}

	if f.GoalID != "" {
		add(func(n string) string { return "p.goal_id = " + n + "::uuid" }, f.GoalID)
	}

	if f.TopicID != "" {
		add(func(n string) string { return "p.topic_id = " + n + "::uuid" }, f.TopicID)
	}

	if f.MaxMinutes > 0 {
		add(func(n string) string { return "p.duration <= " + n }, f.MaxMinutes)
	}

	if f.Age > 0 {
		add(func(n string) string { return "p.min_age <= " + n + " AND p.max_age >= " + n }, f.Age)
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

// postSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func postSet(upd domain.PostUpdate) (string, []any) {
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
		{"title_uz", upd.TitleUz}, {"title_ru", upd.TitleRu}, {"title_en", upd.TitleEn},
		{"video_id", upd.VideoID}, {"goal_id", upd.GoalID},
		{"suggestion_uz", upd.SuggestionUz}, {"suggestion_ru", upd.SuggestionRu}, {"suggestion_en", upd.SuggestionEn},
		{"questions_uz", upd.QuestionsUz}, {"questions_ru", upd.QuestionsRu}, {"questions_en", upd.QuestionsEn},
		{"status", upd.Status}, {"source", upd.Source},
	} {
		if f.value != nil {
			add(f.column, *f.value)
		}
	}

	if upd.PhotoID != nil {
		add("photo_id", nullIfEmpty(*upd.PhotoID))
	}

	if upd.TopicID != nil {
		add("topic_id", nullIfEmpty(*upd.TopicID))
	}

	if upd.MinAge != nil {
		add("min_age", *upd.MinAge)
	}

	if upd.MaxAge != nil {
		add("max_age", *upd.MaxAge)
	}

	if upd.Duration != nil {
		add("duration", *upd.Duration)
	}

	return strings.Join(sets, ", "), args
}

func (p dbPost) toDomain() domain.Post {
	return domain.Post{
		ID:           p.ID,
		TitleUz:      p.TitleUz,
		TitleRu:      p.TitleRu,
		TitleEn:      p.TitleEn,
		VideoID:      p.VideoID,
		PhotoID:      p.PhotoID,
		GoalID:       p.GoalID,
		TopicID:      p.TopicID,
		MinAge:       int(p.MinAge),
		MaxAge:       int(p.MaxAge),
		Duration:     p.Duration,
		SuggestionUz: p.SuggestionUz,
		SuggestionRu: p.SuggestionRu,
		SuggestionEn: p.SuggestionEn,
		QuestionsUz:  p.QuestionsUz,
		QuestionsRu:  p.QuestionsRu,
		QuestionsEn:  p.QuestionsEn,
		Status:       p.Status,
		Source:       p.Source,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}
