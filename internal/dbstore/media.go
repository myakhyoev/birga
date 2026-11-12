package dbstore

import (
	"context"
	"encoding/json"
	"errors"
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
	ID             string    `db:"id"`
	ObjectKey      string    `db:"object_key"`
	ContentType    string    `db:"content_type"`
	SizeBytes      int64     `db:"size_bytes"`
	CreatedAt      time.Time `db:"created_at"`
	Kind           *string   `db:"kind"`
	CropX          *float64  `db:"crop_x"`
	CropY          *float64  `db:"crop_y"`
	CropW          *float64  `db:"crop_w"`
	CropH          *float64  `db:"crop_h"`
	Variants       []byte    `db:"variants"`
	VariantsStatus string    `db:"variants_status"`
}

const mediaColumns = `id, object_key, content_type, size_bytes, created_at, kind, crop_x, crop_y, crop_w, crop_h,
	variants, variants_status`

// Create stores a media row. A non-empty m.ID is used as the primary key, so the id can be part
// of the object key that was uploaded before the row exists. An image with a kind is queued for
// the variant worker (variants_status pending).
func (r *mediaRepo) Create(ctx context.Context, m domain.Media) (domain.Media, error) {
	l := logger.FromCtx(ctx, "mediaRepo.Create")

	var kind *string
	if m.Kind != "" {
		k := string(m.Kind)
		kind = &k
	}

	var cx, cy, cw, ch *float64
	if m.Crop != nil {
		cx, cy, cw, ch = &m.Crop.X, &m.Crop.Y, &m.Crop.W, &m.Crop.H
	}

	status := domain.VariantsNone
	if kind != nil {
		status = domain.VariantsPending
	}

	q := `
		INSERT INTO media (id, object_key, content_type, size_bytes, kind, crop_x, crop_y, crop_w, crop_h, variants_status)
		VALUES (COALESCE($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING ` + mediaColumns

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, nullIfEmpty(m.ID), m.Key, m.ContentType, m.Size,
		kind, cx, cy, cw, ch, status)
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

// Get returns the media row with this id, or ErrMediaNotFound.
func (r *mediaRepo) Get(ctx context.Context, id string) (domain.Media, error) {
	l := logger.FromCtx(ctx, "mediaRepo.Get")

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, `SELECT `+mediaColumns+` FROM media WHERE id = $1`, id)
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Media{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	m, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbMedia])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Media{}, errs.ErrMediaNotFound
	}

	if err != nil {
		l.Error("pgx.CollectExactlyOneRow", zap.Error(err))

		return domain.Media{}, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return m.toDomain(), nil
}

// ClaimPending takes the oldest pending row that is not being worked on (or whose worker stopped
// more than staleAfter ago) and has been tried fewer than maxAttempts times, counts the attempt and
// returns it. ok is false when there is nothing to do. Two workers never get the same row.
func (r *mediaRepo) ClaimPending(ctx context.Context, maxAttempts int, staleAfter time.Duration) (domain.Media, bool, error) {
	l := logger.FromCtx(ctx, "mediaRepo.ClaimPending")

	q := `
		UPDATE media SET variants_attempts = variants_attempts + 1, variants_claimed_at = NOW()
		WHERE id = (
			SELECT id FROM media
			WHERE variants_status = 'pending' AND variants_attempts < $1
			  AND (variants_claimed_at IS NULL OR variants_claimed_at <= NOW() - make_interval(secs => $2))
			ORDER BY created_at, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + mediaColumns

	rows, err := r.store.sqlClientByCtx(ctx).Query(ctx, q, maxAttempts, staleAfter.Seconds())
	if err != nil {
		l.Error("sqlClient.Query", zap.Error(err))

		return domain.Media{}, false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	m, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[dbMedia])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Media{}, false, nil
	}

	if err != nil {
		l.Error("pgx.CollectExactlyOneRow", zap.Error(err))

		return domain.Media{}, false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return m.toDomain(), true, nil
}

// SaveVariants records the variant keys of a row and marks it done.
func (r *mediaRepo) SaveVariants(ctx context.Context, id string, variants map[string]string) error {
	data, err := json.Marshal(variants)
	if err != nil {
		return errs.Wrap(err)
	}

	_, err = r.store.sqlClientByCtx(ctx).Exec(ctx, `
		UPDATE media SET variants = $2, variants_status = 'done', variants_error = NULL, variants_claimed_at = NULL
		WHERE id = $1`, id, data)
	if err != nil {
		logger.FromCtx(ctx, "mediaRepo.SaveVariants").Error("Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

// FailVariants records why making the variants failed. With final, or after maxAttempts tries, the
// row is marked failed and never retried; otherwise it stays pending and is retried once the claim
// goes stale.
func (r *mediaRepo) FailVariants(ctx context.Context, id, reason string, final bool, maxAttempts int) error {
	_, err := r.store.sqlClientByCtx(ctx).Exec(ctx, `
		UPDATE media SET variants_error = $2,
			variants_status = CASE WHEN $3 OR variants_attempts >= $4 THEN 'failed' ELSE 'pending' END
		WHERE id = $1`, id, reason, final, maxAttempts)
	if err != nil {
		logger.FromCtx(ctx, "mediaRepo.FailVariants").Error("Exec", zap.Error(err))

		return errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return nil
}

func (m dbMedia) toDomain() domain.Media {
	d := domain.Media{
		ID:             m.ID,
		Key:            m.ObjectKey,
		ContentType:    m.ContentType,
		Size:           m.SizeBytes,
		CreatedAt:      m.CreatedAt,
		VariantsStatus: m.VariantsStatus,
	}

	if m.Kind != nil {
		d.Kind = domain.MediaKind(*m.Kind)
	}

	if m.CropX != nil && m.CropY != nil && m.CropW != nil && m.CropH != nil {
		d.Crop = &domain.MediaCrop{X: *m.CropX, Y: *m.CropY, W: *m.CropW, H: *m.CropH}
	}

	if len(m.Variants) > 0 {
		_ = json.Unmarshal(m.Variants, &d.Variants)
	}

	return d
}
