package bootstrap

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/config"
	"gitlab.com/loyihalar/birga/backend/internal/dbstore"
	mediauploader "gitlab.com/loyihalar/birga/backend/internal/usecases/media_uploader"
	photoseeder "gitlab.com/loyihalar/birga/backend/internal/usecases/photo_seeder"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// SeedPhotos uploads the starter pictures in dir (seeds/photos) and attaches them to the starter
// activities, articles and video posts that have no picture yet. Without S3 it does nothing.
// The deploy runs it as `birga_backend seed-photos seeds/photos` after the migrations.
func SeedPhotos(ctx context.Context, cfg config.Application, dir string) error {
	logger.New(cfg.LogLevel, config.ApplicationLabel)

	defer func() { _ = logger.Cleanup() }()

	l := logger.FromCtx(ctx, "bootstrap.SeedPhotos")

	s3 := buildS3(l, cfg.S3)
	if s3 == nil {
		fmt.Println("seed photos skipped: S3 is not configured")

		return nil
	}

	pool, closeDB := initDB(l, cfg.Postgres)
	defer closeDB()

	store := dbstore.New(pool)
	up := mediauploader.New(l.Named("usecase.media_uploader"), store.Media(), s3, cfg.Media.MaxSize, cfg.S3.KeyPrefix, cfg.Media.LinkTTL)

	res, err := photoseeder.New(l.Named("usecase.photo_seeder"), store.SeedPhoto(), up).Execute(ctx, dir)
	if err != nil {
		l.Error("seed photos failed", zap.Error(err))

		return err
	}

	fmt.Printf("seed photos: %d listed, %d attached, %d files missing\n", res.Listed, res.Attached, res.Missing)

	return nil
}
