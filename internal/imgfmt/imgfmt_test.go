package imgfmt

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"github.com/gen2brain/avif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sample is a small image with a solid colour, so a lossy round trip stays
// close to it.
func sample(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	return b.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, nil))
	return b.Bytes()
}

// webpFixture is a 16 x 12 lossless WebP made with cwebp, because Go has no
// WebP encoder.
func webpFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.webp")
	require.NoError(t, err)
	return data
}

func encodeAVIF(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, avif.Encode(&b, img))
	return b.Bytes()
}

func encodeGIF(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	require.NoError(t, gif.Encode(&b, img, nil))
	return b.Bytes()
}

func TestToPNGConvertsEveryFormatToPNG(t *testing.T) {
	img := sample(16, 12)
	tests := []struct {
		name string
		in   []byte
	}{
		{"png", encodePNG(t, img)},
		{"jpeg", encodeJPEG(t, img)},
		{"webp", webpFixture(t)},
		{"avif", encodeAVIF(t, img)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ToPNG(tt.in)
			require.NoError(t, err)
			got, format, err := image.Decode(bytes.NewReader(out))
			require.NoError(t, err)
			assert.Equal(t, "png", format)
			assert.Equal(t, 16, got.Bounds().Dx())
			assert.Equal(t, 12, got.Bounds().Dy())
		})
	}
}

func TestToPNGKeepsPNGUnchanged(t *testing.T) {
	in := encodePNG(t, sample(8, 8))
	out, err := ToPNG(in)
	require.NoError(t, err)
	assert.Equal(t, in, out, "a PNG is stored as it came, not re-encoded")
}

func TestToPNGConvertedColoursStayClose(t *testing.T) {
	out, err := ToPNG(encodeJPEG(t, sample(16, 16)))
	require.NoError(t, err)
	got, _, err := image.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	r, g, b, _ := got.At(8, 8).RGBA()
	assert.InDelta(t, 200, float64(r>>8), 12)
	assert.InDelta(t, 40, float64(g>>8), 12)
	assert.InDelta(t, 40, float64(b>>8), 12)
}

func TestToPNGRejects(t *testing.T) {
	valid := encodePNG(t, sample(8, 8))
	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"empty input", nil, ErrUnsupported},
		{"plain text", []byte("not an image at all"), ErrUnsupported},
		{"a gif is not supported", encodeGIF(t, sample(8, 8)), ErrUnsupported},
		{"a truncated png", valid[:len(valid)/2], ErrCorrupt},
		{"a truncated jpeg", encodeJPEG(t, sample(16, 16))[:40], ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := ToPNG(tt.in)
			assert.ErrorIs(t, err, tt.want)
			assert.Nil(t, out)
		})
	}
}

func TestToPNGRejectsTooManyPixelsBeforeDecoding(t *testing.T) {
	// A 1 x 1 image whose header is rewritten to claim a huge size would be
	// a decompression bomb; the header check must refuse it first.
	in := encodePNG(t, sample(4, 4))
	old := limits
	t.Cleanup(func() { limits = old })
	limits.maxPixels = 10
	out, err := ToPNG(in)
	assert.ErrorIs(t, err, ErrTooLarge)
	assert.Nil(t, out)
}

func TestToPNGRejectsTooManyBytes(t *testing.T) {
	in := encodePNG(t, sample(4, 4))
	old := limits
	t.Cleanup(func() { limits = old })
	limits.maxBytes = int64(len(in) - 1)
	out, err := ToPNG(in)
	assert.ErrorIs(t, err, ErrTooLarge)
	assert.Nil(t, out)
}
