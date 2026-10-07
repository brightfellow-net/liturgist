// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package images turns an uploaded logo into the PNG that is stored (15 §2,
// L-2): the real format is read from the bytes, the size is checked before
// the pixels are decoded, the image is scaled down to fit a square and
// written as a new PNG, which drops metadata and anything else hidden in the
// file.
package images

import (
	"bytes"
	"image"
	"image/color"
	_ "image/jpeg" // registers the JPEG decoder
	"image/png"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder
)

// Normalizer implements app.ImageNormalizer.
type Normalizer struct{}

var _ app.ImageNormalizer = Normalizer{}

var (
	errType = &domain.InvalidInputError{Field: "image", Message: "Use a PNG, JPEG or WebP image."}
	errSize = &domain.InvalidInputError{Field: "image", Message: "The image is too large."}
)

// Normalize returns data as a PNG that fits in domain.LogoFitSide, keeping the
// aspect ratio and the transparency. An image that is already small is not
// enlarged.
func (Normalizer) Normalize(data []byte) (app.NormalizedImage, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg" && format != "webp") {
		return app.NormalizedImage{}, errType
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return app.NormalizedImage{}, errType
	}
	// Checked on the header, before the pixels are decoded: a small file can
	// describe an enormous image.
	if cfg.Width > domain.LogoMaxSide || cfg.Height > domain.LogoMaxSide || cfg.Width*cfg.Height > domain.LogoMaxPixels {
		return app.NormalizedImage{}, errSize
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return app.NormalizedImage{}, errType
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > domain.LogoFitSide || h > domain.LogoFitSide {
		if w >= h {
			h, w = max(1, h*domain.LogoFitSide/w), domain.LogoFitSide
		} else {
			w, h = max(1, w*domain.LogoFitSide/h), domain.LogoFitSide
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if format == "jpeg" {
		// A JPEG has no transparency; paint it on white so that the result does not depend on the decoder.
		draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, dst); err != nil {
		return app.NormalizedImage{}, err
	}
	return app.NormalizedImage{PNG: out.Bytes(), Width: w, Height: h}, nil
}
