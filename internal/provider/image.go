package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
)

const (
	maxImageDimension = 2000
	maxBase64Size     = 4 << 20 // 4 MB
)

// PreprocessImage decodes, resizes if needed, and base64-encodes an image.
// JPEG and PNG are decoded and resized via stdlib + x/image/draw.
// WEBP is passed through without resize (stdlib cannot decode it).
func PreprocessImage(data []byte, mimeType string) (ImageData, error) {
	if mimeType == "image/webp" {
		encoded := base64.StdEncoding.EncodeToString(data)
		if len(encoded) > maxBase64Size {
			return ImageData{}, fmt.Errorf("webp image exceeds %d bytes after encoding", maxBase64Size)
		}
		return ImageData{Base64: encoded, MimeType: mimeType}, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ImageData{}, fmt.Errorf("decode image: %w", err)
	}

	// Apply EXIF orientation correction for JPEG images before resize.
	if mimeType == "image/jpeg" {
		img = applyEXIFOrientation(data, img)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	if w > maxImageDimension || h > maxImageDimension {
		img = resizeImage(img, w, h)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return ImageData{}, fmt.Errorf("encode jpeg: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	if len(encoded) > maxBase64Size {
		return ImageData{}, fmt.Errorf("image exceeds %d bytes after resize and encoding", maxBase64Size)
	}

	return ImageData{Base64: encoded, MimeType: "image/jpeg"}, nil
}

// resizeImage scales the image so the largest dimension is maxImageDimension,
// preserving aspect ratio.
func resizeImage(img image.Image, w, h int) image.Image {
	var newW, newH int
	if w >= h {
		newW = maxImageDimension
		newH = h * maxImageDimension / w
	} else {
		newH = maxImageDimension
		newW = w * maxImageDimension / h
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// applyEXIFOrientation reads the EXIF orientation tag from raw JPEG data and
// transforms the decoded image accordingly. If the EXIF data cannot be read
// or has no orientation tag, the image is returned unchanged.
func applyEXIFOrientation(rawJPEG []byte, img image.Image) image.Image {
	orient := readJPEGOrientation(rawJPEG)
	if orient <= 1 || orient > 8 {
		return img
	}
	return orientImage(img, orient)
}

// readJPEGOrientation extracts the EXIF orientation tag (0x0112) from raw
// JPEG data. Returns 0 if the tag is absent or cannot be parsed.
func readJPEGOrientation(data []byte) int {
	// JPEG must start with SOI marker.
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0
	}

	off := 2
	for off+4 <= len(data) {
		if data[off] != 0xFF {
			return 0
		}
		marker := data[off+1]
		segLen := int(data[off+2])<<8 | int(data[off+3])
		if segLen < 2 {
			return 0
		}

		if marker == 0xE1 { // APP1
			if off+2+segLen > len(data) {
				return 0
			}
			return parseEXIFOrientation(data[off+4 : off+2+segLen])
		}

		off += 2 + segLen

		// Stop at SOS or past end.
		if marker == 0xDA {
			break
		}
	}
	return 0
}

// parseEXIFOrientation parses a raw APP1 segment payload (after the length
// field) looking for the EXIF orientation tag.
func parseEXIFOrientation(seg []byte) int {
	// Must start with "Exif\x00\x00".
	if len(seg) < 14 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}

	tiff := seg[6:]

	var bo binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}

	if bo.Uint16(tiff[2:4]) != 0x002A {
		return 0
	}

	ifdOff := int(bo.Uint32(tiff[4:8]))
	if ifdOff+2 > len(tiff) {
		return 0
	}

	count := int(bo.Uint16(tiff[ifdOff : ifdOff+2]))
	entryOff := ifdOff + 2

	for i := 0; i < count; i++ {
		eOff := entryOff + i*12
		if eOff+12 > len(tiff) {
			break
		}
		tag := bo.Uint16(tiff[eOff : eOff+2])
		if tag == 0x0112 { // Orientation
			return int(bo.Uint16(tiff[eOff+8 : eOff+10]))
		}
	}

	return 0
}

// orientImage applies one of the 8 EXIF orientation transforms.
//
//	1: normal
//	2: flip horizontal
//	3: rotate 180
//	4: flip vertical
//	5: transpose (flip horizontal + rotate 270 CW)
//	6: rotate 90 CW
//	7: transverse (flip horizontal + rotate 90 CW)
//	8: rotate 270 CW
func orientImage(img image.Image, orient int) image.Image {
	switch orient {
	case 1:
		return img
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return flipH(rotate270(img))
	case 6:
		return rotate90(img)
	case 7:
		return flipH(rotate90(img))
	case 8:
		return rotate270(img)
	default:
		return img
	}
}

func rotate90(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.Y-1-y, x, img.At(x, y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.X-1-x, b.Max.Y-1-y, img.At(x, y))
		}
	}
	return dst
}

func rotate270(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(y, b.Max.X-1-x, img.At(x, y))
		}
	}
	return dst
}

func flipH(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(b.Max.X-1-x, y, img.At(x, y))
		}
	}
	return dst
}

func flipV(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, b.Max.Y-1-y, img.At(x, y))
		}
	}
	return dst
}

