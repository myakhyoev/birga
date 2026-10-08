package dbstore

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
)

// Integration tests run against a real, migrated database:
//
//	TEST_POSTGRES_URL=postgres://birga:birga@localhost:5432/birga?sslmode=disable make test-integration
//
// Every test runs inside a transaction that is rolled back, so data never persists.
func newTestStore(t *testing.T) *DBStore {
	t.Helper()

	url := os.Getenv("TEST_POSTGRES_URL")
	if url == "" || testing.Short() {
		t.Skip("TEST_POSTGRES_URL not set")
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}

	t.Cleanup(pool.Close)

	return New(pool)
}

var errRollback = errors.New("rollback")

func inRollbackTx(t *testing.T, s *DBStore, fn func(ctx context.Context)) {
	t.Helper()

	err := s.InTx(context.Background(), func(ctx context.Context) error {
		fn(ctx)

		return errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("InTx: %v", err)
	}
}

func activity(goalID string, minAge, maxAge int, published bool) domain.Activity {
	return domain.Activity{
		TitleUz: "t", TitleRu: "т", DescriptionUz: "d", DescriptionRu: "д",
		GoalIDs: []string{goalID}, MinAge: minAge, MaxAge: maxAge, DurationMinutes: 10, IsPublished: published,
	}
}

// testGoal creates a goal named name in all three languages and returns its id.
func testGoal(t *testing.T, s *DBStore, ctx context.Context, name string) string {
	t.Helper()

	g, err := s.Goal().Create(ctx, domain.Goal{NameUz: name + " uz", NameRu: name + " ru", NameEn: name + " en"})
	if err != nil {
		t.Fatalf("Create goal %s: %v", name, err)
	}

	return g.ID
}

func TestActivityRepo_CreateGet(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		motor := testGoal(t, s, ctx, "motor test")

		created, err := s.Activity().Create(ctx, activity(motor, 2, 4, true))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}

		if created.ID == "" || created.CreatedAt.IsZero() {
			t.Fatalf("defaults not returned: %+v", created)
		}

		got, err := s.Activity().Get(ctx, created.ID)
		if err != nil || got.ID != created.ID || got.MaxAge != 4 || len(got.GoalIDs) != 1 || got.GoalIDs[0] != motor {
			t.Fatalf("Get: %+v, %v", got, err)
		}

		if _, err := s.Activity().Get(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, errs.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})
}

func TestActivityRepo_List(t *testing.T) {
	s := newTestStore(t)

	inRollbackTx(t, s, func(ctx context.Context) {
		// Hide rows that may already exist in the dev database.
		if _, err := s.sqlClientByCtx(ctx).Exec(ctx, `DELETE FROM activities`); err != nil {
			t.Fatalf("cleanup: %v", err)
		}

		motor, social := testGoal(t, s, ctx, "motor test"), testGoal(t, s, ctx, "social test")

		both := activity(social, 2, 6, true)
		both.GoalIDs = append(both.GoalIDs, motor)

		for _, a := range []domain.Activity{
			activity(motor, 2, 3, true),
			activity(motor, 4, 6, true),
			activity(social, 2, 6, true),
			activity(motor, 2, 6, false),
			both,
		} {
			if _, err := s.Activity().Create(ctx, a); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}

		cases := []struct {
			name  string
			f     domain.ActivityFilter
			total int
		}{
			{"all", domain.ActivityFilter{Limit: 10}, 5},
			{"published", domain.ActivityFilter{PublishedOnly: true, Limit: 10}, 4},
			{"goal+published", domain.ActivityFilter{GoalID: motor, PublishedOnly: true, Limit: 10}, 3},
			{"age 5", domain.ActivityFilter{Age: 5, PublishedOnly: true, Limit: 10}, 3},
			{"paged", domain.ActivityFilter{Limit: 1, Offset: 1}, 5},
		}

		for _, tc := range cases {
			items, total, err := s.Activity().List(ctx, tc.f)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}

			wantItems := min(tc.total-tc.f.Offset, tc.f.Limit)
			if total != tc.total || len(items) != wantItems {
				t.Fatalf("%s: total=%d items=%d, want total=%d items=%d", tc.name, total, len(items), tc.total, wantItems)
			}
		}
	})
}

func TestPing(t *testing.T) {
	if err := newTestStore(t).Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
