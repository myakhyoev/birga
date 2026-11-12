package domain

import "time"

// Media is an uploaded file stored in object storage (S3). URL is built from Key at read time.
type Media struct {
	ID          string
	Key         string
	URL         string
	ContentType string
	Size        int64
	CreatedAt   time.Time

	// Kind decides the variant sizes; empty for videos and for old images not used anywhere.
	Kind MediaKind
	// Crop is the frame chosen at upload; nil means the centre of the photo.
	Crop *MediaCrop
	// Variants maps a size name (main, thumb, small) to its object key, once the worker made them.
	Variants map[string]string
	// VariantsStatus is none, pending, done or failed.
	VariantsStatus string
}

// VariantKey returns the object key to serve for size: the variant when it exists, else the
// original. served is the size actually used ("original" for the fallback).
func (m Media) VariantKey(size string) (key, served string) {
	if size != "" && size != MediaSizeOriginal {
		if k, ok := m.Variants[size]; ok && k != "" {
			return k, size
		}

		// Profile photos have no "small": their 250x250 thumb is small enough.
		if k, ok := m.Variants[MediaSizeThumb]; size == MediaSizeSmall && ok && k != "" {
			return k, MediaSizeThumb
		}
	}

	return m.Key, MediaSizeOriginal
}

// MediaUpload is a file to store, already decoded from the request.
type MediaUpload struct {
	Data []byte
	// AllowVideo also accepts the video types (admin uploads of daily videos).
	AllowVideo bool
	// Kind is where the photo is shown (profile or content); it decides the variant sizes.
	Kind MediaKind
	// Crop is the frame the uploader chose; nil means the centre.
	Crop *MediaCrop
}

// imageExtensions maps the accepted image types, as sniffed from the content, to file extensions.
var imageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// videoExtensions maps the accepted video types, as sniffed from the content, to file extensions.
var videoExtensions = map[string]string{
	"video/mp4":  ".mp4",
	"video/webm": ".webm",
}

// ImageExtension returns the file extension for an accepted image content type.
func ImageExtension(contentType string) (string, bool) {
	ext, ok := imageExtensions[contentType]

	return ext, ok
}

// VideoExtension returns the file extension for an accepted video content type.
func VideoExtension(contentType string) (string, bool) {
	ext, ok := videoExtensions[contentType]

	return ext, ok
}

// IsVideo reports whether contentType is an accepted video type.
func IsVideo(contentType string) bool {
	_, ok := videoExtensions[contentType]

	return ok
}

// MediaKind says where a photo is shown, which decides the sizes the variant worker makes.
type MediaKind string

const (
	// MediaKindProfile is a user's or a child's photo: square variants.
	MediaKindProfile MediaKind = "profile"
	// MediaKindContent is an activity or article picture or a video cover (banner): 2:1 variants.
	MediaKindContent MediaKind = "content"
)

// Variant names, as GET /v1/media/{id}?size= takes them.
const (
	MediaSizeOriginal = "original"
	MediaSizeMain     = "main"
	MediaSizeThumb    = "thumb"
	MediaSizeSmall    = "small"
)

// Variant processing states of a media row.
const (
	// VariantsNone: nothing to make (a video, or an image uploaded before kinds existed and not used anywhere).
	VariantsNone = "none"
	// VariantsPending: waiting for the worker.
	VariantsPending = "pending"
	// VariantsDone: every variant of the kind is in storage.
	VariantsDone = "done"
	// VariantsFailed: the worker gave up (an unreadable image); clients get the original.
	VariantsFailed = "failed"
)

// MediaVariantSpec is one resized copy of a photo.
type MediaVariantSpec struct {
	Name   string
	Width  int
	Height int
	// Upscale: a smaller source is blown up to the exact size (small pictures); otherwise a
	// smaller source gives a smaller picture of the same shape.
	Upscale bool
}

// mediaVariantSpecs are the sizes made for each kind. All variants of a kind except "small" have the
// kind's shape, so the frame an admin picks works for each; "small" is the centre square of that frame.
//
//nolint:mnd // the size table itself
var mediaVariantSpecs = map[MediaKind][]MediaVariantSpec{
	MediaKindProfile: {
		{Name: MediaSizeMain, Width: 800, Height: 800},
		{Name: MediaSizeThumb, Width: 250, Height: 250, Upscale: true},
	},
	MediaKindContent: {
		{Name: MediaSizeMain, Width: 1200, Height: 600},
		{Name: MediaSizeThumb, Width: 400, Height: 200, Upscale: true},
		{Name: MediaSizeSmall, Width: 240, Height: 240, Upscale: true},
	},
}

// MediaVariantSpecs returns the variants made for kind (nil for an unknown kind).
func MediaVariantSpecs(kind MediaKind) []MediaVariantSpec {
	return mediaVariantSpecs[kind]
}

// MediaAspect is the shape (width / height) of the frame of kind: 1 for profile, 2 for content.
func MediaAspect(kind MediaKind) float64 {
	specs := mediaVariantSpecs[kind]
	if len(specs) == 0 {
		return 0
	}

	return float64(specs[0].Width) / float64(specs[0].Height)
}

// ParseMediaKind accepts "profile" or "content".
func ParseMediaKind(s string) (MediaKind, bool) {
	k := MediaKind(s)
	_, ok := mediaVariantSpecs[k]

	return k, ok
}

// IsMediaSize reports whether s is a size GET /v1/media/{id}?size= accepts.
func IsMediaSize(s string) bool {
	switch s {
	case MediaSizeOriginal, MediaSizeMain, MediaSizeThumb, MediaSizeSmall:
		return true
	}

	return false
}

// MediaCrop is the frame a person chose on the upright photo, in fractions (0-1) of its width and height.
type MediaCrop struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Valid reports whether the frame lies inside the photo and is not empty.
func (c MediaCrop) Valid() bool {
	const eps = 1e-6

	return c.X >= 0 && c.Y >= 0 && c.W > 0 && c.H > 0 && c.X+c.W <= 1+eps && c.Y+c.H <= 1+eps
}
