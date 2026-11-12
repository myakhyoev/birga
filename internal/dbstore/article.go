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

// Soft-deleted articles (deleted_at set) are invisible to every read. Reads and daily picks of
// the app see published articles only; the use cases decide which reads those are.
type articleRepo struct {
	store sqlClientProvider
}

type dbArticle struct {
	ID          string    `db:"id"`
	TitleUz     string    `db:"title_uz"`
	TitleRu     string    `db:"title_ru"`
	TitleEn     string    `db:"title_en"`
	BodyUz      string    `db:"article_body_uz"`
	BodyRu      string    `db:"article_body_ru"`
	BodyEn      string    `db:"article_body_en"`
	TipUz       string    `db:"tip_uz"`
	TipRu       string    `db:"tip_ru"`
	TipEn       string    `db:"tip_en"`
	PhotoID     *string   `db:"photo_id"`
	ReadMinutes int16     `db:"expected_read_time"`
	MinAge      int16     `db:"min_age"`
	MaxAge      int16     `db:"max_age"`
	GoalID      string    `db:"goal_id"`
	TopicID     *string   `db:"topic_id"`
	Status      string    `db:"status"`
	Source      string    `db:"source"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
	IsRead      bool      `db:"is_read"`
}

// articleColumns selects from "articles a"; is_read is false unless the query joins the reader.
const articleColumns = `
	a.id, a.title_uz, a.title_ru, a.title_en, a.article_body_uz, a.article_body_ru, a.article_body_en,
	a.tip_uz, a.tip_ru, a.tip_en, a.photo_id, a.expected_read_time, a.min_age, a.max_age, a.goal_id,
	a.topic_id, a.status, a.source, a.created_at, a.updated_at`

// readerJoin adds is_read for the user in parameter $n.
func readerJoin(n int) (cols, join string) {
	p := "$" + strconv.Itoa(n)

	return `, (r.user_id IS NOT NULL) AS is_read`,
		` LEFT JOIN article_reads r ON r.article_id = a.id AND r.user_id = ` + p + `::uuid`
}

const noReader = `, FALSE AS is_read`

func (r *articleRepo) Create(ctx context.Context, a domain.Article) (domain.Article, error) {
	l := logger.FromCtx(ctx, "articleRepo.Create")

	q := `
		INSERT INTO articles AS a (
			title_uz, title_ru, title_en, article_body_uz, article_body_ru, article_body_en,
			tip_uz, tip_ru, tip_en, photo_id, expected_read_time, min_age, max_age, goal_id, status, topic_id, source
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, COALESCE(NULLIF($17, ''), 'admin'))
		RETURNING ` + articleColumns + noReader

	var photoID *string
	if a.PhotoID != nil {
		photoID = nullIfEmpty(*a.PhotoID)
	}

	return r.one(ctx, l, q,
		a.TitleUz, a.TitleRu, a.TitleEn, a.BodyUz, a.BodyRu, a.BodyEn,
		a.TipUz, a.TipRu, a.TipEn, photoID, a.ReadMinutes, a.MinAge, a.MaxAge, a.GoalID, a.Status, topicOrNull(a.TopicID), a.Source,
	)
}

// Get returns the article in any status. A non-empty readerID fills IsRead for that user.
func (r *articleRepo) Get(ctx context.Context, id, readerID string) (domain.Article, error) {
	l := logger.FromCtx(ctx, "articleRepo.Get").With(zap.String("id", id))

	if readerID == "" {
		return r.one(ctx, l, `SELECT `+articleColumns+noReader+` FROM articles a WHERE a.id = $1 AND a.deleted_at IS NULL`, id)
	}

	cols, join := readerJoin(2) //nolint:mnd

	return r.one(ctx, l, `SELECT `+articleColumns+cols+` FROM articles a`+join+
		` WHERE a.id = $1 AND a.deleted_at IS NULL`, id, readerID)
}

// Update applies the non-nil fields of upd. A change that breaks a CHECK constraint (for example
// min_age above the stored max_age) is ErrValidation.
func (r *articleRepo) Update(ctx context.Context, id string, upd domain.ArticleUpdate) (domain.Article, error) {
	l := logger.FromCtx(ctx, "articleRepo.Update").With(zap.String("id", id))

	var (
		sets, args = articleSet(upd)
		q          = `UPDATE articles AS a SET ` + sets +
			` WHERE a.id = $` + strconv.Itoa(len(args)+1) + ` AND a.deleted_at IS NULL RETURNING ` + articleColumns + noReader
	)

	return r.one(ctx, l, q, append(args, id)...)
}

// Delete soft-deletes the article.
func (r *articleRepo) Delete(ctx context.Context, id string) error {
	l := logger.FromCtx(ctx, "articleRepo.Delete").With(zap.String("id", id))

	q := `UPDATE articles SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q, id)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if tag.RowsAffected() == 0 {
		return errs.ErrArticleNotFound
	}

	return nil
}

// List returns one page of articles matching f and the total number of matches, newest first.
func (r *articleRepo) List(ctx context.Context, f domain.ArticleFilter) ([]domain.Article, int, error) {
	l := logger.FromCtx(ctx, "articleRepo.List")

	var (
		sqlClient   = r.store.sqlClientByCtx(ctx)
		where, args = articleWhere(f)
		countQ      = `SELECT COUNT(*) FROM articles a` + where
		cols, join  = noReader, ""
	)

	if f.ReaderID != "" {
		args = append(args, f.ReaderID)
		cols, join = readerJoin(len(args))
	}

	listQ := `SELECT ` + articleColumns + cols + ` FROM articles a` + join + where +
		` ORDER BY a.created_at DESC, a.id` +
		` LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2) //nolint:mnd

	var total int

	countArgs := args
	if f.ReaderID != "" {
		countArgs = args[:len(args)-1]
	}

	if err := sqlClient.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err), zap.String("query", countQ))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out, err := r.many(ctx, l, listQ, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// MarkRead records that the user read the published article; repeating it keeps the first time.
func (r *articleRepo) MarkRead(ctx context.Context, userID, articleID string) error {
	l := logger.FromCtx(ctx, "articleRepo.MarkRead").With(zap.String("article_id", articleID))

	q := `
		INSERT INTO article_reads (user_id, article_id)
		SELECT $1, a.id FROM articles a WHERE a.id = $2 AND a.status = 'published' AND a.deleted_at IS NULL
		ON CONFLICT (user_id, article_id) DO NOTHING
		RETURNING article_id`

	var id string

	err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, userID, articleID).Scan(&id)
	if err == nil {
		return nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	// No row: either the article is not published, or it was read before.
	a, err := r.Get(ctx, articleID, "")
	if err != nil {
		return err
	}

	if a.Status != domain.ArticleStatusPublished {
		return errs.ErrArticleNotFound
	}

	var exists bool
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM article_reads WHERE user_id = $1 AND article_id = $2)`, userID, articleID,
	).Scan(&exists); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	if !exists {
		return errs.ErrArticleNotFound
	}

	return nil
}

func (r *articleRepo) daily() dailyPicks {
	return dailyPicks{
		store: r.store, content: "articles", daily: "user_daily_articles", idColumn: "article_id", reads: "article_reads",
	}
}

// DailyArticleID returns the article stored for the user's day, if it is still published.
func (r *articleRepo) DailyArticleID(ctx context.Context, userID string, day time.Time) (string, bool, error) {
	return r.daily().storedID(ctx, userID, day)
}

// SetDailyArticle stores articleID as the user's article for day, replacing an earlier pick.
func (r *articleRepo) SetDailyArticle(ctx context.Context, userID string, day time.Time, articleID string) error {
	return r.daily().set(ctx, userID, day, articleID)
}

// WasDaily reports whether the article was the user's daily article on day or an earlier day.
func (r *articleRepo) WasDaily(ctx context.Context, userID, articleID string, day time.Time) (bool, error) {
	return r.daily().wasDaily(ctx, userID, articleID, day)
}

// DailyIDs returns which of ids were the user's daily article on day or earlier.
func (r *articleRepo) DailyIDs(ctx context.Context, userID string, ids []string, day time.Time) (map[string]bool, error) {
	return r.daily().dailyIDs(ctx, userID, ids, day)
}

// LockDaily serializes daily article picks of one user until the surrounding transaction ends.
func (r *articleRepo) LockDaily(ctx context.Context, userID string) error {
	return r.daily().lock(ctx, userID)
}

// PickDaily chooses the next daily article for the user, leaving out the ids in exclude; see
// dailyPicks.pick for the order. Read articles count as shown. ok is false when no published
// article is left.
func (r *articleRepo) PickDaily(ctx context.Context, userID string, exclude []string) (string, bool, error) {
	return r.daily().pick(ctx, userID, exclude)
}

// DailyHistory returns one page of the user's daily articles up to and including day, newest
// first, and their total. Articles deleted since are left out; unpublished ones stay, as the
// user already saw them.
func (r *articleRepo) DailyHistory(ctx context.Context, userID string, day time.Time, limit, offset int,
) ([]domain.DailyArticle, int, error) {
	l := logger.FromCtx(ctx, "articleRepo.DailyHistory")

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		from      = ` FROM user_daily_articles d JOIN articles a ON a.id = d.article_id AND a.deleted_at IS NULL
			WHERE d.user_id = $1 AND d.day <= $2`
		cols, join = readerJoin(1)
		total      int
	)

	if err := sqlClient.QueryRow(ctx, `SELECT COUNT(*)`+from, userID, day).Scan(&total); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	q := `SELECT d.day, ` + articleColumns + cols +
		` FROM user_daily_articles d JOIN articles a ON a.id = d.article_id AND a.deleted_at IS NULL` + join +
		` WHERE d.user_id = $1 AND d.day <= $2 ORDER BY d.day DESC LIMIT $3 OFFSET $4`

	rows, err := sqlClient.Query(ctx, q, userID, day, limit, offset)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	type dbDaily struct {
		Day time.Time `db:"day"`
		dbArticle
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbDaily])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.DailyArticle, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, domain.DailyArticle{Day: dbRows[i].Day, Article: dbRows[i].toDomain()})
	}

	return out, total, nil
}

func (r *articleRepo) many(ctx context.Context, l *zap.Logger, q string, args ...any) ([]domain.Article, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err), zap.String("query", q))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbArticle])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	out := make([]domain.Article, 0, len(dbRows))
	for i := range dbRows {
		out = append(out, dbRows[i].toDomain())
	}

	return out, nil
}

// one runs q, which returns at most one article row (SELECT, or a write with RETURNING). No row
// means ErrArticleNotFound; an unknown photo_id is ErrPhotoNotFound; an unknown goal_id is
// ErrUnknownGoals; a failed CHECK means ErrValidation.
func (r *articleRepo) one(ctx context.Context, l *zap.Logger, q string, args ...any) (domain.Article, error) {
	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, args...)
	if err == nil {
		var a dbArticle

		a, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbArticle])
		if err == nil {
			return a.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Article{}, errs.ErrArticleNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch {
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "articles_photo_id_fkey":
			return domain.Article{}, errs.ErrPhotoNotFound
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "articles_goal_id_fkey":
			return domain.Article{}, errs.ErrUnknownGoals
		case pgErr.Code == pgForeignKeyViolation && pgErr.ConstraintName == "articles_topic_id_fkey":
			return domain.Article{}, errs.ErrUnknownTopic
		case pgErr.Code == pgCheckViolation && pgErr.ConstraintName == "articles_age_range_chk":
			return domain.Article{}, errs.Errf(errs.ErrValidation,
				"age range must be within %d..%d and min_age <= max_age, together with the stored values",
				domain.MinChildAge, domain.MaxChildAge)
		case pgErr.Code == pgCheckViolation:
			return domain.Article{}, errs.Errf(errs.ErrValidation, "%s", pgErr.ConstraintName)
		}
	}

	l.Error("articleRepo query failed", zap.Error(err), zap.String("query", q))

	return domain.Article{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// articleWhere builds a parameterized WHERE clause; values never enter the SQL text.
func articleWhere(f domain.ArticleFilter) (string, []any) {
	var (
		conds = []string{"a.deleted_at IS NULL"}
		args  []any
	)

	add := func(cond func(n string) string, v any) {
		args = append(args, v)
		conds = append(conds, cond("$"+strconv.Itoa(len(args))))
	}

	if f.Status != "" {
		add(func(n string) string { return "a.status = " + n }, f.Status)
	}

	if f.Source != "" {
		add(func(n string) string { return "a.source = " + n }, f.Source)
	}

	if f.Search != "" {
		add(func(n string) string {
			return "(a.title_uz ILIKE " + n + " OR a.title_ru ILIKE " + n + " OR a.title_en ILIKE " + n + ")"
		}, "%"+likeEscaper.Replace(f.Search)+"%")
	}

	if f.GoalID != "" {
		add(func(n string) string { return "a.goal_id = " + n + "::uuid" }, f.GoalID)
	}

	if f.TopicID != "" {
		add(func(n string) string { return "a.topic_id = " + n + "::uuid" }, f.TopicID)
	}

	if f.MaxMinutes > 0 {
		add(func(n string) string { return "a.expected_read_time <= " + n }, f.MaxMinutes)
	}

	if f.Age > 0 {
		add(func(n string) string { return "a.min_age <= " + n + " AND a.max_age >= " + n }, f.Age)
	}

	return " WHERE " + strings.Join(conds, " AND "), args
}

// articleSet builds a parameterized SET clause from fixed fragments; values never enter the SQL text.
func articleSet(upd domain.ArticleUpdate) (string, []any) {
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
		{"article_body_uz", upd.BodyUz}, {"article_body_ru", upd.BodyRu}, {"article_body_en", upd.BodyEn},
		{"tip_uz", upd.TipUz}, {"tip_ru", upd.TipRu}, {"tip_en", upd.TipEn},
		{colGoalID, upd.GoalID}, {colStatus, upd.Status}, {colSource, upd.Source},
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

	for _, f := range []struct {
		column string
		value  *int
	}{{"expected_read_time", upd.ReadMinutes}, {"min_age", upd.MinAge}, {"max_age", upd.MaxAge}} {
		if f.value != nil {
			add(f.column, *f.value)
		}
	}

	return strings.Join(sets, ", "), args
}

func (a dbArticle) toDomain() domain.Article {
	return domain.Article{
		ID:          a.ID,
		TitleUz:     a.TitleUz,
		TitleRu:     a.TitleRu,
		TitleEn:     a.TitleEn,
		BodyUz:      a.BodyUz,
		BodyRu:      a.BodyRu,
		BodyEn:      a.BodyEn,
		TipUz:       a.TipUz,
		TipRu:       a.TipRu,
		TipEn:       a.TipEn,
		PhotoID:     a.PhotoID,
		ReadMinutes: int(a.ReadMinutes),
		MinAge:      int(a.MinAge),
		MaxAge:      int(a.MaxAge),
		GoalID:      a.GoalID,
		TopicID:     a.TopicID,
		Status:      a.Status,
		Source:      a.Source,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
		IsRead:      a.IsRead,
	}
}
