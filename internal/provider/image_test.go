package provider

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
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

// buildMinimalEXIF creates a minimal EXIF block (Exif header + TIFF with one
// IFD entry) containing only the orientation tag.
func buildMinimalEXIF(orientation int) []byte {
	var buf bytes.Buffer

	// Exif header.
	buf.WriteString("Exif\x00\x00")

	// TIFF header: little-endian.
	buf.Write([]byte("II"))                                          // byte order
	binary.Write(&buf, binary.LittleEndian, uint16(0x002A))         // magic
	binary.Write(&buf, binary.LittleEndian, uint32(8))              // offset to IFD0

	// IFD0 at offset 8 from TIFF start.
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // 1 entry

	// Entry: Orientation (tag 0x0112), type SHORT (3), count 1, value.
	binary.Write(&buf, binary.LittleEndian, uint16(0x0112))      // tag
	binary.Write(&buf, binary.LittleEndian, uint16(3))            // type: SHORT
	binary.Write(&buf, binary.LittleEndian, uint32(1))            // count
	binary.Write(&buf, binary.LittleEndian, uint16(orientation))  // value
	binary.Write(&buf, binary.LittleEndian, uint16(0))            // padding

	// Next IFD offset = 0 (no more IFDs).
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	return buf.Bytes()
}

// makeJPEGWithEXIF creates a JPEG with a minimal EXIF APP1 segment containing
// the given orientation value. The image is 20x10 (wider than tall).
func makeJPEGWithEXIF(orientation int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var jpegBuf bytes.Buffer
	jpeg.Encode(&jpegBuf, img, nil)
	raw := jpegBuf.Bytes()

	exifData := buildMinimalEXIF(orientation)

	// Insert APP1 after the SOI marker (first 2 bytes of JPEG).
	var out bytes.Buffer
	out.Write(raw[:2]) // SOI
	out.Write([]byte{0xFF, 0xE1})
	segLen := uint16(len(exifData) + 2)
	binary.Write(&out, binary.BigEndian, segLen)
	out.Write(exifData)
	out.Write(raw[2:])
	return out.Bytes()
}

func TestPreprocessImageJPEGWithEXIFOrientation6(t *testing.T) {
	// Orientation 6 = rotate 90 CW. A 20x10 image becomes 10x20.
	data := makeJPEGWithEXIF(6)
	got, err := PreprocessImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	img, _, _ := image.Decode(bytes.NewReader(decoded))

	// After rotation: width and height should swap.
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 20 {
		t.Errorf("expected 10x20 after rotation, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestPreprocessImageJPEGNoEXIF(t *testing.T) {
	// A regular JPEG without EXIF should pass through with original dimensions.
	data := makeJPEG(20, 10)
	got, err := PreprocessImage(data, "image/jpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	img, _, _ := image.Decode(bytes.NewReader(decoded))
	if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 10 {
		t.Errorf("expected 20x10, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestPreprocessImagePNGSkipsEXIF(t *testing.T) {
	// PNG should never have EXIF applied — dimensions stay the same.
	data := makePNG(20, 10)
	got, err := PreprocessImage(data, "image/png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, _ := base64.StdEncoding.DecodeString(got.Base64)
	img, _, _ := image.Decode(bytes.NewReader(decoded))
	// PNG gets re-encoded as JPEG, dimensions should be preserved.
	if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 10 {
		t.Errorf("expected 20x10, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}
