package provider

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"bytes"
	"testing"
)

func makeJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, nil)
	return buf.Bytes()
}

func makePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestPreprocessImageSmallJPEG(t *testing.T) {
	data := makeJPEG(100, 80)
	got, err := PreprocessImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MimeType != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %q", got.MimeType)
	}
	if got.Base64 == "" {
		t.Error("expected non-empty base64")
	}

	// Decode and verify dimensions unchanged.
	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	img, _, _ := image.Decode(bytes.NewReader(decoded))
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 80 {
		t.Errorf("expected 100x80, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestPreprocessImageLargeJPEGResized(t *testing.T) {
	data := makeJPEG(4000, 2000)
	got, err := PreprocessImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	img, _, _ := image.Decode(bytes.NewReader(decoded))
	if img.Bounds().Dx() != 2000 {
		t.Errorf("expected width 2000, got %d", img.Bounds().Dx())
	}
	if img.Bounds().Dy() != 1000 {
		t.Errorf("expected height 1000, got %d", img.Bounds().Dy())
	}
}

func TestPreprocessImagePNG(t *testing.T) {
	data := makePNG(200, 150)
	got, err := PreprocessImage(data, "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MimeType != "image/jpeg" {
		t.Errorf("expected image/jpeg output, got %q", got.MimeType)
	}
}

func TestPreprocessImageWEBPPassthrough(t *testing.T) {
	// Fake WEBP data — just ensure it passes through without decode.
	fakeData := []byte("RIFF\x00\x00\x00\x00WEBP")
	got, err := PreprocessImage(fakeData, "image/webp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MimeType != "image/webp" {
		t.Errorf("expected image/webp, got %q", got.MimeType)
	}
	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	if !bytes.Equal(decoded, fakeData) {
		t.Error("expected webp data to pass through unchanged")
	}
}

func TestPreprocessImageInvalidData(t *testing.T) {
	_, err := PreprocessImage([]byte("not an image"), "image/jpeg")
	if err == nil {
		t.Fatal("expected error for invalid image data")
	}
}

func TestPreprocessImageTallResized(t *testing.T) {
	data := makeJPEG(1000, 3000)
	got, err := PreprocessImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	img, _, _ := image.Decode(bytes.NewReader(decoded))
	if img.Bounds().Dy() != 2000 {
		t.Errorf("expected height 2000, got %d", img.Bounds().Dy())
	}
	// Width should scale proportionally: 1000 * 2000 / 3000 = 666
	expectedW := 1000 * 2000 / 3000
	if img.Bounds().Dx() != expectedW {
		t.Errorf("expected width %d, got %d", expectedW, img.Bounds().Dx())
	}
}
