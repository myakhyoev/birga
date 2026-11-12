// Package mediavariants is the background worker that makes the resized copies (variants) of
// uploaded photos. Uploads keep the original and queue the row (media.variants_status = pending);
// the worker downloads the original from storage, cuts the chosen frame (or the centre), makes each
// size of the photo's kind as a JPEG next to it (<key prefix>media/<id>/<size>.jpg) and records the
// keys. It runs inside the API process: woken right after an upload and on a timer as a fallback.
package mediavariants

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/pkg/imagevariant"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

const (
	// MaxAttempts is how often one photo is tried before it is marked failed.
	MaxAttempts = 5
	// staleAfter: a claim older than this belongs to a worker that stopped; the row is taken again.
	staleAfter = 5 * time.Minute
	// variantContentType is the type of every variant.
	variantContentType = "image/jpeg"
	// keyFolder matches media_uploader: variants live in a folder named after the media id.
	keyFolder = "media/"
)

type mediaRepo interface {
	ClaimPending(ctx context.Context, maxAttempts int, staleAfter time.Duration) (domain.Media, bool, error)
	SaveVariants(ctx context.Context, id string, variants map[string]string) error
	FailVariants(ctx context.Context, id, reason string, final bool, maxAttempts int) error
}

type objectStorage interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key, contentType string, data []byte) error
}

// UseCase makes the variants of pending photos.
type UseCase struct {
	l         logger.Logger
	repo      mediaRepo
	storage   objectStorage
	keyPrefix string
	wake      chan struct{}
}

func New(l logger.Logger, repo mediaRepo, storage objectStorage, keyPrefix string) *UseCase {
	return &UseCase{l: l, repo: repo, storage: storage, keyPrefix: keyPrefix, wake: make(chan struct{}, 1)}
}

// Wake asks a running worker to look for work now; it never blocks.
func (uc *UseCase) Wake() {
	select {
	case uc.wake <- struct{}{}:
	default:
	}
}

// Run processes pending photos until ctx ends: everything queued, then it sleeps until Wake or
// the next tick of interval.
func (uc *UseCase) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		for ctx.Err() == nil {
			did, err := uc.ProcessOne(ctx)
			if err != nil {
				uc.l.Error("photo variants: claim failed", zap.Error(err))

				break
			}

			if !did {
				break
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-uc.wake:
		}
	}
}

// ProcessOne makes the variants of one pending photo. did is false when nothing was pending.
// A failure of the photo itself is recorded on its row, not returned.
func (uc *UseCase) ProcessOne(ctx context.Context) (did bool, err error) {
	m, ok, err := uc.repo.ClaimPending(ctx, MaxAttempts, staleAfter)
	if err != nil || !ok {
		return false, err
	}

	variants, final, err := uc.make(ctx, m)
	if err != nil {
		uc.l.Warn("photo variants failed", zap.String("media_id", m.ID), zap.Bool("final", final), zap.Error(err))

		if ferr := uc.repo.FailVariants(context.WithoutCancel(ctx), m.ID, err.Error(), final, MaxAttempts); ferr != nil {
			return true, ferr
		}

		return true, nil
	}

	if err := uc.repo.SaveVariants(ctx, m.ID, variants); err != nil {
		return true, err
	}

	uc.l.Info("photo variants made", zap.String("media_id", m.ID), zap.String("kind", string(m.Kind)), zap.Int("count", len(variants)))

	return true, nil
}

// errBadImage marks failures that a retry cannot fix.
var errBadImage = errors.New("bad image")

// make builds and stores every variant of m. final tells whether a failure is worth retrying:
// storage errors are, an unreadable image is not.
func (uc *UseCase) make(ctx context.Context, m domain.Media) (variants map[string]string, final bool, err error) {
	specs := domain.MediaVariantSpecs(m.Kind)
	if len(specs) == 0 {
		return nil, true, fmt.Errorf("%w: unknown kind %q", errBadImage, m.Kind)
	}

	data, err := uc.storage.Get(ctx, m.Key)
	if err != nil {
		return nil, false, err
	}

	img, err := imagevariant.Decode(data)
	if err != nil {
		return nil, true, fmt.Errorf("%w: %w", errBadImage, err)
	}

	var crop *imagevariant.Rect
	if m.Crop != nil {
		crop = &imagevariant.Rect{X: m.Crop.X, Y: m.Crop.Y, W: m.Crop.W, H: m.Crop.H}
	}

	// The kind's frame (first spec's shape) is cut first, so "small", a square, is the centre of
	// the chosen frame rather than of the whole photo.
	frame := imagevariant.Frame(img.Bounds(), crop, domain.MediaAspect(m.Kind))
	fw, fh := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	frameRect := &imagevariant.Rect{
		X: float64(frame.Min.X-img.Bounds().Min.X) / fw, Y: float64(frame.Min.Y-img.Bounds().Min.Y) / fh,
		W: float64(frame.Dx()) / fw, H: float64(frame.Dy()) / fh,
	}

	variants = make(map[string]string, len(specs))

	for _, s := range specs {
		out, err := imagevariant.EncodeJPEG(imagevariant.Render(img, frameRect,
			imagevariant.Spec{Width: s.Width, Height: s.Height, Upscale: s.Upscale}))
		if err != nil {
			return nil, true, fmt.Errorf("%w: %w", errBadImage, err)
		}

		key := uc.variantKey(m, s.Name)
		if err := uc.storage.Put(ctx, key, variantContentType, out); err != nil {
			return nil, false, err
		}

		variants[s.Name] = key
	}

	return variants, false, nil
}

// variantKey is <key prefix>media/<id>/<size>.jpg, next to the original <key prefix>media/<id>.<ext>.
func (uc *UseCase) variantKey(m domain.Media, size string) string {
	// Keep the prefix the original was stored under, even if S3_KEY_PREFIX changed since.
	prefix := uc.keyPrefix
	if i := strings.LastIndex(m.Key, keyFolder); i >= 0 {
		prefix = m.Key[:i]
	}

	return prefix + keyFolder + m.ID + "/" + size + ".jpg"
}
