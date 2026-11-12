package imagevariant

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestFrame(t *testing.T) {
	b := image.Rect(0, 0, 1536, 1024)

	tests := []struct {
		name   string
		crop   *Rect
		aspect float64
		want   image.Rectangle
	}{
		{"centre 2:1 of 3:2", nil, 2, image.Rect(0, 128, 1536, 896)},
		{"centre square", nil, 1, image.Rect(256, 0, 1280, 1024)},
		{"chosen 2:1 frame at the top", &Rect{0, 0, 1, 0.75}, 2, image.Rect(0, 0, 1536, 768)},
		{"square inside a chosen frame", &Rect{0, 0, 1, 0.75}, 1, image.Rect(384, 0, 1152, 768)},
		{"invalid crop is ignored", &Rect{0.5, 0, 0.8, 1}, 2, image.Rect(0, 128, 1536, 896)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Frame(b, tt.crop, tt.aspect); got != tt.want {
				t.Fatalf("Frame = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderSizes(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))

	if got := Render(img, nil, Spec{Width: 400, Height: 200}).Bounds(); got.Dx() != 400 || got.Dy() != 200 {
		t.Fatalf("thumb = %v", got)
	}

	// 1200x600 main from a 600 wide photo: no upscale, same shape.
	if got := Render(img, nil, Spec{Width: 1200, Height: 600}).Bounds(); got.Dx() != 600 || got.Dy() != 300 {
		t.Fatalf("main = %v", got)
	}

	if got := Render(img, nil, Spec{Width: 1200, Height: 600, Upscale: true}).Bounds(); got.Dx() != 1200 {
		t.Fatalf("upscaled main = %v", got)
	}
}

func TestDecodePNGAndEncode(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 20)) // fully transparent
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	img, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	out, err := EncodeJPEG(Render(img, nil, Spec{Width: 20, Height: 10}))
	if err != nil {
		t.Fatal(err)
	}

	back, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}

	if r, g, b, _ := back.At(5, 5).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Fatalf("transparent pixel should turn white, got %d %d %d", r>>8, g>>8, b>>8)
	}
}

// jpegWithOrientation encodes img and inserts an EXIF APP1 segment with this orientation.
func jpegWithOrientation(t *testing.T, img image.Image, o uint16) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}

	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	entry := make([]byte, 2+12+4)
	binary.BigEndian.PutUint16(entry[0:], 1)      // one entry
	binary.BigEndian.PutUint16(entry[2:], 0x0112) // Orientation
	binary.BigEndian.PutUint16(entry[4:], 3)      // SHORT
	binary.BigEndian.PutUint32(entry[6:], 1)      // count
	binary.BigEndian.PutUint16(entry[10:], o)
	tiff = append(tiff, entry...)

	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(len(seg)+2))
	app1 = append(app1, seg...)

	data := buf.Bytes()

	return append(append(append([]byte{}, data[:2]...), app1...), data[2:]...)
}

func TestDecodeTurnsPhoneJPEGUpright(t *testing.T) {
	// 64x32, left half red, right half blue.
	src := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := range 32 {
		for x := range 64 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 32 {
				c = color.RGBA{B: 255, A: 255}
			}

			src.Set(x, y, c)
		}
	}

	data := jpegWithOrientation(t, src, 6) // display turned 90 degrees clockwise
	if o := exifOrientation(data); o != 6 {
		t.Fatalf("orientation = %d, want 6", o)
	}

	img, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}

	if b := img.Bounds(); b.Dx() != 32 || b.Dy() != 64 {
		t.Fatalf("upright size = %v, want 32x64", b)
	}

	// Turned clockwise, the left (red) half is now on top.
	if r, _, b, _ := img.At(16, 8).RGBA(); r>>8 < 200 || b>>8 > 60 {
		t.Fatalf("top should be red, got r=%d b=%d", r>>8, b>>8)
	}
}

func TestExifOrientationWithoutExif(t *testing.T) {
	if o := exifOrientation([]byte("not a jpeg")); o != 1 {
		t.Fatalf("got %d", o)
	}
}
