package dbstore

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

type mediaRepo struct {
	store sqlClientProvider
}

type dbMedia struct {
	ID          string    `db:"id"`
	ObjectKey   string    `db:"object_key"`
	ContentType string    `db:"content_type"`
	SizeBytes   int64     `db:"size_bytes"`
	CreatedAt   time.Time `db:"created_at"`
}

const mediaColumns = `id, object_key, content_type, size_bytes, created_at`

// Create stores a media row. A non-empty m.ID is used as the primary key, so the id can be part
// of the object key that was uploaded before the row exists.
func (r *mediaRepo) Create(ctx context.Context, m domain.Media) (domain.Media, error) {
	l := logger.FromCtx(ctx, "mediaRepo.Create")

	q := `
		INSERT INTO media (id, object_key, content_type, size_bytes)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4)
		RETURNING ` + mediaColumns

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, nullIfEmpty(m.ID), m.Key, m.ContentType, m.Size)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Media{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	created, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbMedia])
	if err != nil {
		l.Error("pgx.CollectExactlyOneRow", zap.Error(err))

		return domain.Media{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return created.toDomain(), nil
}

func (m dbMedia) toDomain() domain.Media {
	return domain.Media{
		ID:          m.ID,
		Key:         m.ObjectKey,
		ContentType: m.ContentType,
		Size:        m.SizeBytes,
		CreatedAt:   m.CreatedAt,
	}
}
