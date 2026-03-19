package provider

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log/slog"

	"github.com/rwcarlsen/goexif/exif"
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
	x, err := exif.Decode(bytes.NewReader(rawJPEG))
	if err != nil {
		slog.Debug("exif decode failed, skipping orientation", slog.String("error", err.Error()))
		return img
	}

	tag, err := x.Get(exif.Orientation)
	if err != nil {
		return img // no orientation tag
	}

	orient, err := tag.Int(0)
	if err != nil {
		return img
	}

	return orientImage(img, orient)
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

