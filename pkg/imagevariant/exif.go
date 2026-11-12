package imagevariant

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
)

// exifOrientation returns the EXIF orientation (1-8) of a JPEG, or 1 when it has none. Phones save
// photos unrotated and record how to turn them in this tag; Go's decoder ignores it.
func exifOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}

	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 1
		}

		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // start of scan or end of image: no more metadata
			return 1
		}

		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}

		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}

		i += 2 + size
	}

	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}

	var bo binary.ByteOrder

	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}

	ifd := int(bo.Uint32(t[4:]))
	if ifd < 8 || ifd+2 > len(t) {
		return 1
	}

	n := int(bo.Uint16(t[ifd:]))
	for k := range n {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}

		if bo.Uint16(t[e:]) == 0x0112 { // Orientation, a SHORT
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}

			return 1
		}
	}

	return 1
}

// orient turns img upright for an EXIF orientation.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	src := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)

	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}

	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))

	for y := range h {
		for x := range w {
			var dx, dy int

			switch o {
			case 2: // mirrored
				dx, dy = w-1-x, y
			case 3: // turned 180
				dx, dy = w-1-x, h-1-y
			case 4: // upside down mirrored
				dx, dy = x, h-1-y
			case 5: // transposed
				dx, dy = y, x
			case 6: // turn 90 clockwise
				dx, dy = h-1-y, x
			case 7: // transversed
				dx, dy = h-1-y, w-1-x
			case 8: // turn 90 counter-clockwise
				dx, dy = y, w-1-x
			}

			si := src.PixOffset(x, y)
			di := dst.PixOffset(dx, dy)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}

	return dst
}
