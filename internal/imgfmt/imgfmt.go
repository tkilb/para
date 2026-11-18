// Package imgfmt turns an image in a supported format into PNG, the only image
// format para stores. It validates the bytes first, so a hostile or damaged
// file is refused before it is decoded or written.
package imgfmt

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"

	// These register the formats image.Decode accepts. Only the four in
	// supported are allowed through.
	_ "github.com/gen2brain/avif"
	_ "golang.org/x/image/webp"
	_ "image/jpeg"
)

const (
	defaultMaxBytes  = 32 << 20 // the largest input accepted
	defaultMaxPixels = 50 << 20 // about 50 megapixels
)

var (
	// ErrUnsupported reports bytes that are not a PNG, JPEG, WebP or AVIF image.
	ErrUnsupported = errors.New("not a PNG, JPEG, WebP or AVIF image")
	// ErrCorrupt reports an image of a supported format that cannot be decoded.
	ErrCorrupt = errors.New("the image is damaged")
	// ErrTooLarge reports an image with too many bytes or pixels.
	ErrTooLarge = errors.New("the image is too large")
)

// limits holds the size caps. It is a variable only so tests can lower it.
var limits = struct {
	maxBytes  int64
	maxPixels int64
}{defaultMaxBytes, defaultMaxPixels}

// supported lists the formats image.Decode names that para accepts.
var supported = map[string]bool{"png": true, "jpeg": true, "webp": true, "avif": true}

// ToPNG returns data as PNG. A PNG is returned as it came, once checked. A
// JPEG, WebP or AVIF is decoded and encoded as PNG. It returns ErrUnsupported,
// ErrCorrupt or ErrTooLarge, and never a partial result.
func ToPNG(data []byte) ([]byte, error) {
	if int64(len(data)) > limits.maxBytes {
		return nil, ErrTooLarge
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil && !errors.Is(err, image.ErrFormat) {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if err != nil || !supported[format] {
		return nil, ErrUnsupported
	}
	if int64(cfg.Width)*int64(cfg.Height) > limits.maxPixels {
		return nil, ErrTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if format == "png" {
		return data, nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("encode PNG: %w", err)
	}
	return out.Bytes(), nil
}
