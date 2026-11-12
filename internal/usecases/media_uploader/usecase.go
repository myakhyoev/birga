package mediauploader

import (
	"context"
	"net/http"
	"time"

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
	Get(ctx context.Context, id string) (domain.Media, error)
}

// objectStorage stores files (S3).
type objectStorage interface {
	Put(ctx context.Context, key, contentType string, data []byte) error
	Delete(ctx context.Context, key string) error
	URL(key string) string
	Presign(ctx context.Context, key string, ttl time.Duration) (string, error)
	Get(ctx context.Context, key string) ([]byte, error)
}

// UseCase validates an image, uploads it to object storage and records it in the media table.
type UseCase struct {
	l         logger.Logger
	repo      mediaRepo
	storage   objectStorage
	maxSize   int64
	keyPrefix string
	linkTTL   time.Duration
	// uploaded is called after an image is stored, to wake the variant worker; may be nil.
	uploaded func()
}

// New creates a new media uploader use case. maxSize is the largest accepted file in bytes;
// keyPrefix is put in front of every object key; linkTTL is how long a Link works.
func New(l logger.Logger, repo mediaRepo, storage objectStorage, maxSize int64, keyPrefix string, linkTTL time.Duration) *UseCase {
	return &UseCase{
		l:         l,
		repo:      repo,
		storage:   storage,
		maxSize:   maxSize,
		keyPrefix: keyPrefix,
		linkTTL:   linkTTL,
	}
}

// OnUpload sets fn to be called after every image upload (the variant worker's wake-up).
func (uc *UseCase) OnUpload(fn func()) {
	uc.uploaded = fn
}

// MaxSize is the largest accepted file in bytes.
func (uc *UseCase) MaxSize() int64 {
	return uc.maxSize
}

// Execute checks the file (size, and a JPEG, PNG or WebP image judged by its content, not by
// what the client claims; with AllowVideo also an MP4 or WebM video), uploads it under a new
// random key and stores the media row.
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
	if !ok && up.AllowVideo {
		ext, ok = domain.VideoExtension(contentType)
	}

	switch {
	case !ok && up.AllowVideo:
		return domain.Media{}, errs.ErrMediaUnsupportedTypeOrVideo
	case !ok:
		return domain.Media{}, errs.ErrMediaUnsupportedType
	}

	kind, crop := up.Kind, up.Crop
	if domain.IsVideo(contentType) {
		// Videos are kept as uploaded; their banner is a separate image (posts.photo_id).
		kind, crop = "", nil
	}

	if err := checkKindCrop(kind, crop); err != nil {
		return domain.Media{}, err
	}

	id := uuid.NewString()
	key := uc.keyPrefix + keyFolder + id + ext

	if err := uc.storage.Put(ctx, key, contentType, up.Data); err != nil {
		return domain.Media{}, err
	}

	m, err := uc.repo.Create(ctx, domain.Media{ID: id, Key: key, ContentType: contentType, Size: size, Kind: kind, Crop: crop})
	if err != nil {
		// Do not leave an object nobody can reference.
		if delErr := uc.storage.Delete(context.WithoutCancel(ctx), key); delErr != nil {
			uc.l.Error("orphan object left in storage", zap.String("key", key), zap.Error(delErr))
		}

		return domain.Media{}, err
	}

	m.URL = uc.storage.URL(m.Key)

	if kind != "" && uc.uploaded != nil {
		uc.uploaded()
	}

	return m, nil
}

// checkKindCrop accepts an empty or known kind, and a crop only with a kind.
func checkKindCrop(kind domain.MediaKind, crop *domain.MediaCrop) error {
	if kind != "" {
		if _, ok := domain.ParseMediaKind(string(kind)); !ok {
			return errs.Errf(errs.ErrValidation, "kind must be profile or content")
		}
	}

	if crop == nil {
		return nil
	}

	if kind == "" {
		return errs.Errf(errs.ErrValidation, "crop needs a kind (profile or content)")
	}

	if !crop.Valid() {
		return errs.Errf(errs.ErrValidation, "crop must lie inside the photo: x, y >= 0, w, h > 0, x+w <= 1, y+h <= 1")
	}

	return nil
}

// Recrop makes a new media row from the original photo of id with another kind and frame, and
// queues its variants. Media ids never change their picture (clients cache by id), so a new frame
// is a new id; the caller then points the content at it. Videos cannot be recropped.
func (uc *UseCase) Recrop(ctx context.Context, id string, kind domain.MediaKind, crop *domain.MediaCrop) (domain.Media, error) {
	if uuid.Validate(id) != nil {
		return domain.Media{}, errs.ErrMediaNotFound
	}

	if kind == "" {
		return domain.Media{}, errs.Errf(errs.ErrValidation, "kind is required: profile or content")
	}

	src, err := uc.repo.Get(ctx, id)
	if err != nil {
		return domain.Media{}, err
	}

	if domain.IsVideo(src.ContentType) {
		return domain.Media{}, errs.Errf(errs.ErrValidation, "a video has no frame; upload a cover picture instead")
	}

	data, err := uc.storage.Get(ctx, src.Key)
	if err != nil {
		return domain.Media{}, err
	}

	return uc.Execute(ctx, domain.MediaUpload{Data: data, Kind: kind, Crop: crop})
}

// Link returns a signed link to the media with this id that works for linkTTL; the bucket is
// private, so this is how clients read a photo. size picks a variant (main, thumb, small); when that
// variant is not made (yet), or size is "" or original, the link is to the original. served says
// which one the link opens. An id that is not a UUID or not uploaded: not found.
func (uc *UseCase) Link(ctx context.Context, id, size string) (link, served string, err error) {
	if uuid.Validate(id) != nil {
		return "", "", errs.ErrMediaNotFound
	}

	m, err := uc.repo.Get(ctx, id)
	if err != nil {
		return "", "", err
	}

	key, served := m.VariantKey(size)

	link, err = uc.storage.Presign(ctx, key, uc.linkTTL)
	if err != nil {
		return "", "", err
	}

	return link, served, nil
}
