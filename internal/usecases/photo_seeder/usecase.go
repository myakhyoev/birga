// Package photoseeder attaches the starter pictures in seeds/photos to the starter content: each
// activity, article and video post listed there that has no picture yet gets one. Runs on every
// deploy; content that already has a picture (seeded before, or chosen by an admin) is left alone.
package photoseeder

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"gitlab.com/loyihalar/birga/backend/internal/domain"
	"gitlab.com/loyihalar/birga/backend/internal/errs"
	"gitlab.com/loyihalar/birga/backend/pkg/logger"
)

// SourcesFile lists the seed photos inside the seed photo folder.
const SourcesFile = "sources.tsv"

type targetRepo interface {
	NeedsPhoto(ctx context.Context, kind, key string) (bool, error)
	SetPhoto(ctx context.Context, kind, key, mediaID string) (int, error)
}

type uploader interface {
	Execute(ctx context.Context, up domain.MediaUpload) (domain.Media, error)
}

type UseCase struct {
	l        logger.Logger
	repo     targetRepo
	uploader uploader
}

func New(l logger.Logger, repo targetRepo, up uploader) *UseCase {
	return &UseCase{l: l, repo: repo, uploader: up}
}

// Result counts what a run did.
type Result struct {
	Listed   int // photos in the list
	Attached int // photos uploaded and attached
	Missing  int // listed, but the JPEG is not in the folder yet
}

// Execute reads dir/sources.tsv and attaches every listed photo whose content still has none.
func (uc *UseCase) Execute(ctx context.Context, dir string) (Result, error) {
	f, err := os.Open(filepath.Join(dir, SourcesFile))
	if err != nil {
		return Result{}, errs.Errf(errs.ErrInternal, "open the seed photo list: %s", err.Error())
	}
	defer f.Close()

	photos, err := ParseSources(f)
	if err != nil {
		return Result{}, err
	}

	l := logger.WithContext(uc.l, ctx)
	res := Result{Listed: len(photos)}

	for _, p := range photos {
		need, err := uc.repo.NeedsPhoto(ctx, p.Kind, p.Key)
		if err != nil {
			return res, err
		}

		if !need {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, filepath.Clean("/"+p.File)))
		if os.IsNotExist(err) {
			l.Warn("seed photo file missing", zap.String("file", p.File))

			res.Missing++

			continue
		}

		if err != nil {
			return res, errs.Errf(errs.ErrInternal, "read %s: %s", p.File, err.Error())
		}

		m, err := uc.uploader.Execute(ctx, domain.MediaUpload{Data: data, Kind: domain.MediaKindContent})
		if err != nil {
			return res, err
		}

		n, err := uc.repo.SetPhoto(ctx, p.Kind, p.Key, m.ID)
		if err != nil {
			return res, err
		}

		l.Info("seed photo attached", zap.String("kind", p.Kind), zap.String("file", p.File), zap.Int("rows", n))

		res.Attached++
	}

	return res, nil
}

// ParseSources reads the tab-separated list: kind, key, file, source URL. Blank lines and lines
// starting with # are skipped.
func ParseSources(r io.Reader) ([]domain.SeedPhoto, error) {
	var (
		out  []domain.SeedPhoto
		sc   = bufio.NewScanner(r)
		line int
	)

	for sc.Scan() {
		line++

		text := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "#") {
			continue
		}

		cols := strings.Split(text, "\t")
		if len(cols) < 3 || cols[0] == "" || cols[1] == "" || cols[2] == "" { //nolint:mnd
			return nil, errs.Errf(errs.ErrValidation, "%s line %d: want kind, key and file separated by tabs", SourcesFile, line)
		}

		switch cols[0] {
		case domain.SeedPhotoActivity, domain.SeedPhotoArticle, domain.SeedPhotoPost:
		default:
			return nil, errs.Errf(errs.ErrValidation, "%s line %d: unknown kind %q", SourcesFile, line, cols[0])
		}

		out = append(out, domain.SeedPhoto{Kind: cols[0], Key: cols[1], File: cols[2]})
	}

	if err := sc.Err(); err != nil {
		return nil, errs.Errf(errs.ErrInternal, "read %s: %s", SourcesFile, err.Error())
	}

	return out, nil
}
