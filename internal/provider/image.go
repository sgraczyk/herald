package provider

import (
	"bytes"
	"encoding/base64"
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

