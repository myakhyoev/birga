package dbstore

import (
	"context"
	"errors"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// planRepo stores the 7-day plans: child_plans (one row per plan of a child, the latest is the
// current one) and child_plan_days (the goal, activity and done day of each day).
type planRepo struct {
	store sqlClientProvider
}

type dbPlanRow struct {
	dbActivity

	PlanID      string     `db:"plan_id"`
	Number      int32      `db:"number"`
	StartedOn   time.Time  `db:"started_on"`
	Day         int16      `db:"day"`
	DayGoalID   *string    `db:"day_goal_id"`
	CompletedOn *time.Time `db:"completed_on"`
	Usable      bool       `db:"usable"`
}

// LockChild locks the child row until the transaction ends, so two requests cannot change the
// same child's plan at once. A missing or deleted child is ErrChildNotFound.
func (r *planRepo) LockChild(ctx context.Context, childID string) error {
	l := logger.FromCtx(ctx, "planRepo.LockChild").With(zap.String("child_id", childID))

	var id string

	err := r.store.sqlClientByCtx(ctx).QueryRow(ctx,
		`SELECT id FROM children WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, childID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.ErrChildNotFound
	}

	if err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// Current returns the child's latest plan (without statuses) and its days in order. usable[i] is
// false when the activity of day i has since been unpublished or deleted. A child with no plan
// yet gets a zero Plan (empty ID).
func (r *planRepo) Current(ctx context.Context, childID string) (domain.Plan, []domain.PlanItem, []bool, error) {
	l := logger.FromCtx(ctx, "planRepo.Current").With(zap.String("child_id", childID))

	q := `
		SELECT ` + prefixColumns("a", activityColumns) + `,
			p.id AS plan_id, p.number, p.started_on, d.day, d.goal_id AS day_goal_id, d.completed_on,
			(a.is_published AND a.deleted_at IS NULL) AS usable
		FROM (SELECT * FROM child_plans WHERE child_id = $1 ORDER BY number DESC LIMIT 1) p
		JOIN child_plan_days d ON d.plan_id = p.id
		JOIN activities a ON a.id = d.activity_id
		ORDER BY d.day`

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, childID)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Plan{}, nil, nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	dbRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbPlanRow])
	if err != nil {
		l.Error("pgx.CollectRows", zap.Error(err))

		return domain.Plan{}, nil, nil, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	var (
		plan   = domain.Plan{ChildID: childID}
		items  = make([]domain.PlanItem, 0, len(dbRows))
		usable = make([]bool, 0, len(dbRows))
	)

	for i := range dbRows {
		row := dbRows[i]
		plan.ID, plan.Number, plan.StartedOn = row.PlanID, int(row.Number), row.StartedOn

		items = append(items, domain.PlanItem{
			Day:         int(row.Day),
			GoalID:      row.DayGoalID,
			Activity:    row.toDomain(),
			CompletedOn: row.CompletedOn,
		})
		usable = append(usable, row.Usable)
	}

	return plan, items, usable, nil
}

// Create stores a new plan for the child with the given number, started on startedOn, and its
// days, and returns the plan id.
func (r *planRepo) Create(ctx context.Context, childID string, number int, startedOn time.Time, items []domain.PlanItem) (string, error) {
	l := logger.FromCtx(ctx, "planRepo.Create").With(zap.String("child_id", childID), zap.Int("number", number))

	var (
		days        = make([]int16, 0, len(items))
		goalIDs     = make([]*string, 0, len(items))
		activityIDs = make([]string, 0, len(items))
	)

	for _, it := range items {
		days = append(days, int16(it.Day)) //nolint:gosec // 1..7
		goalIDs = append(goalIDs, it.GoalID)
		activityIDs = append(activityIDs, it.Activity.ID)
	}

	q := `
		WITH p AS (
			INSERT INTO child_plans (child_id, number, started_on) VALUES ($1, $2, $3) RETURNING id
		), d AS (
			INSERT INTO child_plan_days (plan_id, day, goal_id, activity_id)
			SELECT p.id, d.day, d.goal_id, d.activity_id
			FROM p, unnest($4::smallint[], $5::uuid[], $6::uuid[]) AS d (day, goal_id, activity_id)
		)
		SELECT id FROM p`

	var id string
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q,
		childID, number, startedOn.Format(time.DateOnly), days, goalIDs, activityIDs).Scan(&id); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return "", errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return id, nil
}

// Pick returns one published activity for a plan day: within the child's age and not in
// ExcludeIDs. Activities serving the day's goal come first, then normal ones (not easy or
// difficult), then those serving more of the child's goals, then the ones the child has not done
// (or did longest ago); ties are broken per child and Seed. No match is ErrNoRecommendation.
func (r *planRepo) Pick(ctx context.Context, p domain.PlanPick) (domain.Activity, error) {
	l := logger.FromCtx(ctx, "planRepo.Pick").With(zap.String("child_id", p.ChildID))

	q := `
		SELECT ` + prefixColumns("a", activityColumns) + `
		FROM activities a
		LEFT JOIN (
			SELECT activity_id, MAX(completed_on) AS last_on
			FROM activity_completions WHERE child_id = $1
			GROUP BY activity_id
		) done ON done.activity_id = a.id
		WHERE a.is_published AND a.deleted_at IS NULL AND a.min_age <= $2 AND a.max_age >= $2
			AND NOT (a.id = ANY($3::uuid[]))
		ORDER BY COALESCE($4::uuid = ANY(a.goal_ids), FALSE) DESC, a.difficulty = 'normal' DESC,
			(SELECT COUNT(*) FROM unnest(a.goal_ids) g WHERE g = ANY($5::uuid[])) DESC,
			done.last_on NULLS FIRST, md5(a.id::text || $1::text || $6::text)
		LIMIT 1`

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q,
		p.ChildID, p.Age, nonNil(p.ExcludeIDs), p.GoalID, nonNil(p.ChildGoalIDs), p.Seed)
	if err == nil {
		var a dbActivity

		a, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbActivity])
		if err == nil {
			return a.toDomain(), nil
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Activity{}, errs.ErrNoRecommendation
	}

	l.Error("planRepo.Pick query failed", zap.Error(err))

	return domain.Activity{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
}

// SetActivity swaps the activity of one day of a plan.
func (r *planRepo) SetActivity(ctx context.Context, planID string, day int, activityID string) error {
	l := logger.FromCtx(ctx, "planRepo.SetActivity").With(zap.String("plan_id", planID), zap.Int("day", day))

	_, err := r.store.sqlClientByCtx(ctx).Exec(ctx,
		`UPDATE child_plan_days SET activity_id = $3 WHERE plan_id = $1 AND day = $2`, planID, day, activityID)
	if err != nil {
		l.Error("sqlClient.Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// MarkDone marks the open day of the child's current plan done on `on`, when activityID is that
// day's activity and no plan day of the child was done on `on` or later yet. It returns the day
// marked, the day of the current plan this activity already did on `on`, or 0 when the completion
// is not a plan day.
func (r *planRepo) MarkDone(ctx context.Context, childID, activityID string, on time.Time) (int, error) {
	l := logger.FromCtx(ctx, "planRepo.MarkDone").With(zap.String("child_id", childID))

	q := `
		WITH p AS (
			SELECT id FROM child_plans WHERE child_id = $1 ORDER BY number DESC LIMIT 1
		), next AS (
			SELECT d.plan_id, d.day FROM child_plan_days d JOIN p ON p.id = d.plan_id
			WHERE d.completed_on IS NULL ORDER BY d.day LIMIT 1
		)
		UPDATE child_plan_days d SET completed_on = $3
		FROM next
		WHERE d.plan_id = next.plan_id AND d.day = next.day AND d.activity_id = $2
			AND NOT EXISTS (
				SELECT 1 FROM child_plan_days x JOIN child_plans xp ON xp.id = x.plan_id
				WHERE xp.child_id = $1 AND x.completed_on >= $3
			)
		RETURNING d.day`

	var day int16

	var (
		sqlClient = r.store.sqlClientByCtx(ctx)
		args      = []any{childID, activityID, on.Format(time.DateOnly)}
	)

	err := sqlClient.QueryRow(ctx, q, args...).Scan(&day)
	if errors.Is(err, pgx.ErrNoRows) {
		err = sqlClient.QueryRow(ctx, `
			SELECT d.day FROM child_plan_days d
			JOIN (SELECT id FROM child_plans WHERE child_id = $1 ORDER BY number DESC LIMIT 1) p ON p.id = d.plan_id
			WHERE d.activity_id = $2 AND d.completed_on = $3
			ORDER BY d.day LIMIT 1`, args...).Scan(&day)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}

	if err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return int(day), nil
}

// DoneDays returns the distinct days on which a plan day of the child was done, newest first, the
// number of plan days done, and the day the child's first plan started (zero with no plan).
func (r *planRepo) DoneDays(ctx context.Context, childID string) ([]time.Time, int, time.Time, error) {
	l := logger.FromCtx(ctx, "planRepo.DoneDays").With(zap.String("child_id", childID))

	sqlClient := r.store.sqlClientByCtx(ctx)

	var first *time.Time
	if err := sqlClient.QueryRow(ctx,
		`SELECT MIN(started_on) FROM child_plans WHERE child_id = $1`, childID).Scan(&first); err != nil {
		l.Error("sqlClient.QueryRow", zap.Error(err))

		return nil, 0, time.Time{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	rows, err := sqlClient.Query(ctx, `
		SELECT d.completed_on, COUNT(*)
		FROM child_plan_days d JOIN child_plans p ON p.id = d.plan_id
		WHERE p.child_id = $1 AND d.completed_on IS NOT NULL
		GROUP BY d.completed_on
		ORDER BY d.completed_on DESC`, childID)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return nil, 0, time.Time{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}
	defer rows.Close()

	var (
		days  []time.Time
		total int
	)

	for rows.Next() {
		var (
			on time.Time
			n  int
		)

		if err := rows.Scan(&on, &n); err != nil {
			l.Error("rows.Scan", zap.Error(err))

			return nil, 0, time.Time{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
		}

		days = append(days, on)
		total += n
	}

	if err := rows.Err(); err != nil {
		l.Error("rows.Err", zap.Error(err))

		return nil, 0, time.Time{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	var firstOn time.Time
	if first != nil {
		firstOn = *first
	}

	return days, total, firstOn, nil
}
