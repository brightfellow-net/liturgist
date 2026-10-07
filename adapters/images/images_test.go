// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package images_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/images"
	"github.com/brightfellow-net/liturgist/domain"
)

func testdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = 200, 255
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withHeaderSize rewrites the size in a PNG's header (and its checksum), so
// that a tiny file claims to be a huge image.
func withHeaderSize(t *testing.T, data []byte, w, h uint32) []byte {
	t.Helper()
	out := bytes.Clone(data)
	// signature (8) + length (4) + "IHDR" (4), then width and height.
	binary.BigEndian.PutUint32(out[16:], w)
	binary.BigEndian.PutUint32(out[20:], h)
	binary.BigEndian.PutUint32(out[29:], crc32.ChecksumIEEE(out[12:29]))
	return out
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("the result is not a PNG: %v", err)
	}
	return img
}

func alphaAt(img image.Image, x, y int) uint8 {
	return color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A
}

// TC-614: every format comes out as a PNG that fits the square, with the aspect ratio.
func TestNormalizeSizes(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 900, 300)), nil); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name       string
		data       []byte
		fitW, fitH int
	}{
		{"wide png", pngOf(t, 2000, 1000), 512, 256},
		{"tall png", pngOf(t, 1000, 2000), 256, 512},
		{"square png", pngOf(t, 512, 512), 512, 512},
		{"small png is not enlarged", pngOf(t, 100, 50), 100, 50},
		{"one pixel high", pngOf(t, 4096, 1), 512, 1},
		{"jpeg (170.67 rounds down)", jpg.Bytes(), 512, 170},
		{"webp", testdata(t, "logo.webp"), 512, 256},
		{"the largest allowed size", pngOf(t, 4000, 4000), 512, 512},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := images.Normalizer{}.Normalize(c.data)
			if err != nil {
				t.Fatal(err)
			}
			n := decodePNG(t, got.PNG)
			if got.Width != c.fitW || got.Height != c.fitH || n.Bounds().Dx() != c.fitW || n.Bounds().Dy() != c.fitH {
				t.Errorf("got %d x %d (reported %d x %d), want %d x %d", n.Bounds().Dx(), n.Bounds().Dy(), got.Width, got.Height, c.fitW, c.fitH)
			}
		})
	}
}

// TC-614: transparency is kept, in PNG and in WebP.
func TestNormalizeKeepsTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 20; y < 40; y++ {
		for x := 0; x < 40; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"png": b.Bytes(), "webp": testdata(t, "logo.webp")} {
		got, err := images.Normalizer{}.Normalize(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		n := decodePNG(t, got.PNG)
		if a := alphaAt(n, 2, 2); a != 0 {
			t.Errorf("%s: the transparent corner has alpha %d", name, a)
		}
		if a := alphaAt(n, n.Bounds().Dx()-2, n.Bounds().Dy()-2); a != 255 {
			t.Errorf("%s: the opaque corner has alpha %d", name, a)
		}
	}
}

// TC-615: what is refused, with the message the sender sees.
func TestNormalizeRefuses(t *testing.T) {
	var g bytes.Buffer
	if err := gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	good := pngOf(t, 64, 64)
	const typeMsg, sizeMsg = "Use a PNG, JPEG or WebP image.", "The image is too large."
	for _, c := range []struct {
		name string
		data []byte
		msg  string
	}{
		{"text", []byte("hello"), typeMsg},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), typeMsg},
		{"gif", g.Bytes(), typeMsg},
		{"truncated png", good[:len(good)/2], typeMsg},
		{"animated webp", testdata(t, "anim.webp"), typeMsg},
		{"4097 px wide", withHeaderSize(t, good, 4097, 1), sizeMsg},
		{"4097 px high", withHeaderSize(t, good, 1, 4097), sizeMsg},
		{"over 16 million pixels", withHeaderSize(t, good, 4001, 4000), sizeMsg},
		{"a 100 000 x 100 000 header", withHeaderSize(t, good, 100000, 100000), sizeMsg},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := images.Normalizer{}.Normalize(c.data)
			var in *domain.InvalidInputError
			if !errors.As(err, &in) || in.Field != "image" || in.Message != c.msg {
				t.Errorf("got %v, want invalid input %q", err, c.msg)
			}
		})
	}
}

// TC-616: camera data does not survive.
func TestNormalizeDropsMetadata(t *testing.T) {
	src := testdata(t, "photo.jpg")
	if !bytes.Contains(src, []byte("SecretCamera")) || !bytes.Contains(src, []byte("Exif")) {
		t.Fatal("the fixture has no EXIF data")
	}
	got, err := images.Normalizer{}.Normalize(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"SecretCamera", "Exif", "GPS", "eXIf"} {
		if strings.Contains(string(got.PNG), s) {
			t.Errorf("the result still contains %q", s)
		}
	}
	// Only the chunks a plain PNG needs.
	for _, chunk := range []string{"tEXt", "iTXt", "zTXt", "eXIf"} {
		if bytes.Contains(got.PNG, []byte(chunk)) {
			t.Errorf("the result has a %s chunk", chunk)
		}
	}
}
