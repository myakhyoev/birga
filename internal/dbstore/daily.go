package dbstore

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// dailyPicks keeps "one item per user per local day" for a content table (articles, posts). The
// table and column names are fixed fragments set in code; values never enter the SQL text.
type dailyPicks struct {
	store sqlClientProvider
	// content is the content table: id, goal_id (or topicColumn), min_age, max_age, status,
	// created_at, deleted_at.
	content string
	// topicColumn, when set, is the column the days rotate through instead of goal_id, and the
	// children's goals are not matched (tips have topics, not goals).
	topicColumn string
	// daily is the per-day table: user_id, day, <idColumn>, created_at.
	daily    string
	idColumn string
	// reads, when set, is a table (user_id, <idColumn>) of items the user finished; they count as shown.
	reads string
}

const publishedLive = `status = 'published' AND deleted_at IS NULL`

// lock serializes the daily picks of one user until the surrounding transaction ends.
func (d dailyPicks) lock(ctx context.Context, userID string) error {
	if _, err := d.store.sqlClientByCtx(ctx).Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1 || ':' || $2::text))`, d.daily, userID); err != nil {
		logger.FromCtx(ctx, "dailyPicks.lock").Error("sqlClient.Exec", zap.Error(err), zap.String("table", d.daily))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// storedID returns the item stored for the user's day, if it is still published.
func (d dailyPicks) storedID(ctx context.Context, userID string, day time.Time) (string, bool, error) {
	q := `SELECT d.` + d.idColumn + ` FROM ` + d.daily + ` d
		JOIN ` + d.content + ` c ON c.id = d.` + d.idColumn + ` AND c.` + publishedLive + `
		WHERE d.user_id = $1 AND d.day = $2`

	return d.oneID(ctx, "dailyPicks.storedID", q, userID, day)
}

// set stores id as the user's item for day, replacing an earlier pick.
func (d dailyPicks) set(ctx context.Context, userID string, day time.Time, id string) error {
	q := `INSERT INTO ` + d.daily + ` (user_id, day, ` + d.idColumn + `) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, day) DO UPDATE SET ` + d.idColumn + ` = EXCLUDED.` + d.idColumn + `, created_at = NOW()`

	if _, err := d.store.sqlClientByCtx(ctx).Exec(ctx, q, userID, day, id); err != nil {
		logger.FromCtx(ctx, "dailyPicks.set").Error("sqlClient.Exec", zap.Error(err), zap.String("table", d.daily))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// pick chooses the next daily item for the user among published items, leaving out the ids in
// exclude. Preference, in order: the goal is one of the goals of the user's children (unless
// topicColumn is set); the age
// range fits one of the children; not shown as a daily item before (nor finished, when reads is
// set); the item shown longest ago; the goal shown longest ago; the oldest item. ok is false when
// no published item is left.
func (d dailyPicks) pick(ctx context.Context, userID string, exclude []string) (string, bool, error) {
	if exclude == nil {
		exclude = []string{}
	}

	group, goalOrder := colGoalID, `(c.goal_id IN (SELECT goal_id FROM kid_goals)) DESC,`
	if d.topicColumn != "" {
		group, goalOrder = d.topicColumn, ``
	}

	seen, readsJoin := `s.item_id IS NOT NULL`, ``
	if d.reads != "" {
		seen = `(s.item_id IS NOT NULL OR rd.` + d.idColumn + ` IS NOT NULL)`
		readsJoin = ` LEFT JOIN ` + d.reads + ` rd ON rd.` + d.idColumn + ` = c.id AND rd.user_id = $1`
	}

	q := `
		WITH kids AS (
			SELECT ch.age, ch.goal_ids FROM children ch
			JOIN user_children uc ON uc.child_id = ch.id
			WHERE uc.user_id = $1 AND ch.deleted_at IS NULL
		),
		kid_goals AS (SELECT DISTINCT UNNEST(goal_ids) AS goal_id FROM kids),
		shown AS (
			SELECT ` + d.idColumn + ` AS item_id, MAX(day) AS last_day FROM ` + d.daily + `
			WHERE user_id = $1 GROUP BY ` + d.idColumn + `
		),
		goal_shown AS (
			SELECT x.` + group + ` AS group_id, MAX(dd.day) AS last_day
			FROM ` + d.daily + ` dd JOIN ` + d.content + ` x ON x.id = dd.` + d.idColumn + `
			WHERE dd.user_id = $1 GROUP BY x.` + group + `
		)
		SELECT c.id FROM ` + d.content + ` c
		LEFT JOIN shown s ON s.item_id = c.id
		LEFT JOIN goal_shown gs ON gs.group_id = c.` + group + readsJoin + `
		WHERE c.` + publishedLive + ` AND c.id <> ALL($2::uuid[])
		ORDER BY
			` + goalOrder + `
			EXISTS (SELECT 1 FROM kids k WHERE k.age BETWEEN c.min_age AND c.max_age) DESC,
			` + seen + `,
			s.last_day NULLS FIRST,
			gs.last_day NULLS FIRST,
			c.created_at, c.id
		LIMIT 1`

	return d.oneID(ctx, "dailyPicks.pick", q, userID, exclude)
}

func (d dailyPicks) oneID(ctx context.Context, name, q string, args ...any) (string, bool, error) {
	var id string

	err := d.store.sqlClientByCtx(ctx).QueryRow(ctx, q, args...).Scan(&id)
	switch {
	case err == nil:
		return id, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return "", false, nil
	default:
		logger.FromCtx(ctx, name).Error("sqlClient.QueryRow", zap.Error(err), zap.String("table", d.daily))

		return "", false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}
}

// wasDaily reports whether id was the user's item on day or an earlier day.
func (d dailyPicks) wasDaily(ctx context.Context, userID, id string, day time.Time) (bool, error) {
	var ok bool

	q := `SELECT EXISTS (SELECT 1 FROM ` + d.daily + ` WHERE user_id = $1 AND ` + d.idColumn + ` = $2 AND day <= $3)`
	if err := d.store.sqlClientByCtx(ctx).QueryRow(ctx, q, userID, id, day).Scan(&ok); err != nil {
		logger.FromCtx(ctx, "dailyPicks.wasDaily").Error("sqlClient.QueryRow", zap.Error(err), zap.String("table", d.daily))

		return false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return ok, nil
}

// dailyIDs returns which of ids were the user's item on day or an earlier day.
func (d dailyPicks) dailyIDs(ctx context.Context, userID string, ids []string, day time.Time) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	q := `SELECT DISTINCT ` + d.idColumn + `::text FROM ` + d.daily + `
		WHERE user_id = $1 AND ` + d.idColumn + ` = ANY($2::uuid[]) AND day <= $3`

	rows, err := d.store.sqlClientByCtx(ctx).Query(ctx, q, userID, ids, day)
	if err != nil {
		logger.FromCtx(ctx, "dailyPicks.dailyIDs").Error("sqlClient.Query", zap.Error(err), zap.String("table", d.daily))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	shown, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		logger.FromCtx(ctx, "dailyPicks.dailyIDs").Error("pgx.CollectRows", zap.Error(err))

		return nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	for _, id := range shown {
		out[id] = true
	}

	return out, nil
}
