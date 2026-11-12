// Package imagevariant makes resized copies of uploaded photos: it decodes a JPEG, PNG or WebP
// (turning phone JPEGs upright from their EXIF orientation), cuts the frame a person chose (or the
// centre), scales it with a Catmull-Rom filter and encodes a JPEG.
package imagevariant

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // register the PNG decoder
	"math"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register the WebP decoder
)

// MaxPixels is the largest image Decode accepts (width x height), so one upload cannot use
// gigabytes of memory. 50 megapixels covers every phone camera.
const MaxPixels = 50_000_000

// JPEGQuality is the quality of the encoded variants.
const JPEGQuality = 85

// ErrTooLarge means the image has more than MaxPixels pixels.
var ErrTooLarge = errors.New("image is larger than 50 megapixels")

// Rect is a part of an image in fractions of its upright width and height: X and Y are the top-left
// corner, W and H the size, all between 0 and 1.
type Rect struct {
	X, Y, W, H float64
}

// Valid reports whether r lies inside the image and is not empty.
func (r Rect) Valid() bool {
	const eps = 1e-6

	return r.X >= 0 && r.Y >= 0 && r.W > 0 && r.H > 0 && r.X+r.W <= 1+eps && r.Y+r.H <= 1+eps
}

// Decode reads a JPEG, PNG or WebP image and turns a JPEG upright by its EXIF orientation.
func Decode(data []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read image header: %w", err)
	}

	if int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return nil, ErrTooLarge
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	if format == "jpeg" {
		img = orient(img, exifOrientation(data))
	}

	return img, nil
}

// Spec is one variant: its size in pixels. Upscale false lets a smaller source give a smaller
// picture of the same shape instead of a blown-up one.
type Spec struct {
	Width, Height int
	Upscale       bool
}

// Frame is the part of img a variant shows, in pixels: the chosen crop (or the whole image), then
// the largest centred part of it with the variant's shape (width/height).
func Frame(b image.Rectangle, crop *Rect, aspect float64) image.Rectangle {
	r := b
	if crop != nil && crop.Valid() {
		w, h := float64(b.Dx()), float64(b.Dy())
		r = image.Rect(
			b.Min.X+int(math.Round(crop.X*w)), b.Min.Y+int(math.Round(crop.Y*h)),
			b.Min.X+int(math.Round((crop.X+crop.W)*w)), b.Min.Y+int(math.Round((crop.Y+crop.H)*h)),
		).Intersect(b)

		if r.Empty() {
			r = b
		}
	}

	w, h := float64(r.Dx()), float64(r.Dy())
	if w/h > aspect { // too wide: trim the sides
		nw := int(math.Round(h * aspect))
		x := r.Min.X + (r.Dx()-nw)/2 //nolint:mnd // centred: half of the rest on each side

		return image.Rect(x, r.Min.Y, x+max(nw, 1), r.Max.Y)
	}

	nh := int(math.Round(w / aspect)) // too tall: trim top and bottom
	y := r.Min.Y + (r.Dy()-nh)/2      //nolint:mnd // centred

	return image.Rect(r.Min.X, y, r.Max.X, y+max(nh, 1))
}

// Render cuts the variant's frame out of img and scales it to the spec's size.
func Render(img image.Image, crop *Rect, s Spec) image.Image {
	aspect := float64(s.Width) / float64(s.Height)
	fr := Frame(img.Bounds(), crop, aspect)

	w, h := s.Width, s.Height
	if !s.Upscale && fr.Dx() < w {
		w = max(fr.Dx(), 1)
		h = max(int(math.Round(float64(w)/aspect)), 1)
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// JPEG has no transparency: transparent PNG parts become white, not black.
	xdraw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, fr, xdraw.Over, nil)

	return dst
}

// EncodeJPEG encodes img as a JPEG at JPEGQuality.
func EncodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return buf.Bytes(), nil
}
