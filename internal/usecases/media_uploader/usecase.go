package mediauploader

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// keyFolder groups uploaded media inside the bucket (after the configured key prefix).
const keyFolder = "media/"

type mediaRepo interface {
	Create(ctx context.Context, m domain.Media) (domain.Media, error)
}

// objectStorage stores files (S3).
type objectStorage interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

// UseCase validates an image, uploads it to object storage and records it in the media table.
type UseCase struct {
	l         logger.Logger
	repo      mediaRepo
	storage   objectStorage
	maxSize   int64
	keyPrefix string
}

// New creates a new media uploader use case. maxSize is the largest accepted file in bytes;
// keyPrefix is put in front of every object key.
func New(l logger.Logger, repo mediaRepo, storage objectStorage, maxSize int64, keyPrefix string) *UseCase {
	return &UseCase{
		l:         l,
		repo:      repo,
		storage:   storage,
		maxSize:   maxSize,
		keyPrefix: keyPrefix,
	}
}

// MaxSize is the largest accepted file in bytes.
func (uc *UseCase) MaxSize() int64 {
	return uc.maxSize
}

// Execute checks the file (size, and a JPEG, PNG or WebP image judged by its content, not by
// what the client claims), uploads it under a new random key and stores the media row.
func (uc *UseCase) Execute(ctx context.Context, up domain.MediaUpload) (domain.Media, error) {
	size := int64(len(up.Data))

	switch {
	case size == 0:
		return domain.Media{}, errs.ErrMediaEmpty
	case size > uc.maxSize:
		return domain.Media{}, errs.Errf(errs.ErrValidation, "file is %d bytes, the limit is %d bytes", size, uc.maxSize)
	}

	contentType := http.DetectContentType(up.Data)

	ext, ok := domain.ImageExtension(contentType)
	if !ok {
		return domain.Media{}, errs.ErrMediaUnsupportedType
	}

	id := uuid.NewString()
	key := uc.keyPrefix + keyFolder + id + ext

	if err := uc.storage.Put(ctx, key, contentType, up.Data); err != nil {
		return domain.Media{}, err
	}

	m, err := uc.repo.Create(ctx, domain.Media{ID: id, Key: key, ContentType: contentType, Size: size})
	if err != nil {
		// Do not leave an object nobody can reference.
		if delErr := uc.storage.Delete(context.WithoutCancel(ctx), key); delErr != nil {
			uc.l.Error("orphan object left in storage", zap.String("key", key), zap.Error(delErr))
		}

		return domain.Media{}, err
	}

	m.URL = uc.storage.URL(m.Key)

	return m, nil
}
