package dbstore

import (
	"context"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// seedPhotoRepo finds content without a picture and attaches starter pictures to it. An activity is
// matched by its Uzbek title, with the ‘ ’ ʻ apostrophes read as '; articles and posts by id.
type seedPhotoRepo struct {
	store sqlClientProvider
}

// seedPhotoTargets holds, per kind, the fixed WHERE clause that picks the rows of key ($1) still
// without a picture. Values never enter the SQL text.
var seedPhotoTargets = map[string]struct{ table, match string }{
	domain.SeedPhotoActivity: {"activities", `TRANSLATE(title_uz, '‘’ʻ', '''''''') = TRANSLATE($1, '‘’ʻ', '''''''')`},
	domain.SeedPhotoArticle:  {"articles", `id::text = $1`},
	domain.SeedPhotoPost:     {"posts", `id::text = $1`},
}

// NeedsPhoto reports whether some live row of this kind and key has no picture.
func (r *seedPhotoRepo) NeedsPhoto(ctx context.Context, kind, key string) (bool, error) {
	t, ok := seedPhotoTargets[kind]
	if !ok {
		return false, errs.Errf(errs.ErrValidation, "unknown seed photo kind %q", kind)
	}

	var n int

	q := `SELECT COUNT(*) FROM ` + t.table + ` WHERE ` + t.match + ` AND photo_id IS NULL AND deleted_at IS NULL`
	if err := r.store.sqlClientByCtx(ctx).QueryRow(ctx, q, key).Scan(&n); err != nil {
		logger.FromCtx(ctx, "seedPhotoRepo.NeedsPhoto").Error("QueryRow", zap.Error(err))

		return false, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return n > 0, nil
}

// SetPhoto sets mediaID as the picture of every live row of this kind and key that has none, and
// returns how many rows changed. A picture an admin chose is never replaced.
func (r *seedPhotoRepo) SetPhoto(ctx context.Context, kind, key, mediaID string) (int, error) {
	t, ok := seedPhotoTargets[kind]
	if !ok {
		return 0, errs.Errf(errs.ErrValidation, "unknown seed photo kind %q", kind)
	}

	q := `UPDATE ` + t.table + ` SET photo_id = $2, updated_at = NOW() WHERE ` + t.match +
		` AND photo_id IS NULL AND deleted_at IS NULL`

	tag, err := r.store.sqlClientByCtx(ctx).Exec(ctx, q, key, mediaID)
	if err != nil {
		logger.FromCtx(ctx, "seedPhotoRepo.SetPhoto").Error("Exec", zap.Error(err))

		return 0, errs.Errf(errs.ErrInternal, "%s", err.Error())
	}

	return int(tag.RowsAffected()), nil
}
