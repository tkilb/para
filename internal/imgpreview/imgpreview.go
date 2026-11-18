// Package imgpreview draws a PNG as a scaled preview in a terminal: with the
// Kitty or iTerm2 image protocol where the terminal has one, and with colored
// half-block characters everywhere else. It builds strings only; it never
// writes to the terminal itself.
package imgpreview

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"strings"

	"golang.org/x/image/draw"

	"github.com/tkilb/para/internal/imgfmt"
)

// Protocol is how an image is drawn.
type Protocol int

const (
	// Blocks draws colored half-block characters. It works in any terminal
	// with true color, including inside tmux.
	Blocks Protocol = iota
	// Kitty draws with the Kitty graphics protocol (Kitty, Ghostty).
	Kitty
	// ITerm2 draws with the iTerm2 inline image protocol.
	ITerm2
)

const (
	// cellWidth and cellHeight are the assumed size of a terminal cell in
	// pixels. Terminals scale the image into the cells asked for, so a wrong
	// guess only changes how sharp the image is, and its shape slightly.
	cellWidth  = 8
	cellHeight = 16
)

// ErrUnreadable reports a file that is not a readable image. It never says
// why or where, because it is shown in the status bar.
var ErrUnreadable = errors.New("cannot read the image")

// Picture is a decoded PNG ready to draw at any size.
type Picture struct {
	img    image.Image
	data   []byte // the PNG as read
	width  int
	height int
}

// Open reads and decodes the PNG at path. The file is checked like any other
// image para accepts, so a huge or damaged file is refused before decoding.
func Open(path string) (Picture, error) {
	f, err := os.Open(path)
	if err != nil {
		return Picture{}, ErrUnreadable
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, imgfmt.MaxBytes+1))
	if err != nil {
		return Picture{}, ErrUnreadable
	}
	data, err := imgfmt.ToPNG(raw)
	if err != nil {
		return Picture{}, ErrUnreadable
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return Picture{}, ErrUnreadable
	}
	b := img.Bounds()
	return Picture{img: img, data: data, width: b.Dx(), height: b.Dy()}, nil
}

// Caption describes the picture: "640 x 480 px, 12.3 KB".
func (p Picture) Caption() string {
	return fmt.Sprintf("%d x %d px, %s", p.width, p.height, humanSize(len(p.data)))
}

func humanSize(n int) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// Render draws the picture to fit cols x rows terminal cells, keeping its
// shape and never enlarging it. It returns one string per row used, which may
// be fewer than rows, and nothing when the box has no room.
func (p Picture) Render(proto Protocol, cols, rows int) []string {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	switch proto {
	case Kitty:
		return p.placed(cols, rows, kittySequence)
	case ITerm2:
		return p.placed(cols, rows, iterm2Sequence)
	}
	return p.blocks(cols, rows)
}

// fit returns w x h reduced to fit inside maxW x maxH with the same shape.
// It never enlarges.
func fit(w, h, maxW, maxH int) (int, int) {
	switch {
	case w <= maxW && h <= maxH:
		return w, h
	case w*maxH > h*maxW: // limited by width
		return maxW, max(h*maxW/w, 1)
	}
	return max(w*maxH/h, 1), maxH
}

// scaled returns the picture resized to w x h.
func (p Picture) scaled(w, h int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), p.img, p.img.Bounds(), draw.Src, nil)
	return dst
}

// scaledPNG returns the picture as PNG bytes of at most maxW x maxH pixels,
// with the size it came out at. A picture that already fits is not touched.
func (p Picture) scaledPNG(maxW, maxH int) ([]byte, int, int) {
	w, h := fit(p.width, p.height, maxW, maxH)
	if w == p.width && h == p.height {
		return p.data, w, h
	}
	var b bytes.Buffer
	if err := png.Encode(&b, p.scaled(w, h)); err != nil {
		return p.data, p.width, p.height
	}
	return b.Bytes(), w, h
}

// placed draws the picture with a terminal image protocol. The sequence goes
// on the first row and the rows under it stay empty, so no text lands on the
// image.
func (p Picture) placed(cols, rows int, seq func(data []byte, c, r int) string) []string {
	data, w, h := p.scaledPNG(cols*cellWidth, rows*cellHeight)
	c := min(max((w+cellWidth-1)/cellWidth, 1), cols)
	r := min(max((h+cellHeight-1)/cellHeight, 1), rows)
	lines := make([]string, r)
	lines[0] = seq(data, c, r)
	return lines
}

// Cleanup returns what to write before anything else on the row an image
// began on, to remove an image the terminal still holds. It is empty for
// protocols whose images vanish when their cells are drawn over.
func Cleanup(proto Protocol) string {
	if proto == Kitty {
		return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", kittyID)
	}
	return ""
}

// transparentBelow is the alpha under which a pixel is left to the terminal's
// own background.
const transparentBelow = 32

// blocks draws the picture with half-block characters: each cell is two
// pixels, the top as the foreground of "▀" and the bottom as its background.
func (p Picture) blocks(cols, rows int) []string {
	w, h := fit(p.width, p.height, cols, rows*2)
	img := p.scaled(w, h)
	lines := make([]string, 0, (h+1)/2)
	for y := 0; y < h; y += 2 {
		var b strings.Builder
		for x := 0; x < w; x++ {
			writeCell(&b, img, x, y)
		}
		b.WriteString("\x1b[0m")
		lines = append(lines, b.String())
	}
	return lines
}

// writeCell draws the pixels at (x, y) and (x, y+1) as one character.
func writeCell(b *strings.Builder, img *image.NRGBA, x, y int) {
	top := img.NRGBAAt(x, y)
	bottom := img.NRGBAAt(x, y+1) // outside the image reads as transparent
	topSeen, bottomSeen := top.A >= transparentBelow, bottom.A >= transparentBelow
	switch {
	case topSeen && bottomSeen:
		fmt.Fprintf(b, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
	case topSeen:
		fmt.Fprintf(b, "\x1b[38;2;%d;%d;%dm\x1b[49m▀", top.R, top.G, top.B)
	case bottomSeen:
		fmt.Fprintf(b, "\x1b[38;2;%d;%d;%dm\x1b[49m▄", bottom.R, bottom.G, bottom.B)
	default:
		b.WriteString("\x1b[49m ")
	}
}
