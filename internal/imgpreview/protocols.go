package imgpreview

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	// kittyID names the one image para keeps on screen, so placing another
	// replaces it and Cleanup removes it.
	kittyID = 4242
	// kittyChunk is the most base64 text the Kitty protocol allows in one
	// escape sequence.
	kittyChunk = 4096
)

// Detect chooses the protocol for the terminal described by getenv. Inside
// tmux or screen it is always Blocks, because they do not pass image
// sequences through by default. PARA_IMAGE_PROTOCOL (kitty, iterm2 or blocks)
// overrides the guess.
func Detect(getenv func(string) string) Protocol {
	switch strings.ToLower(getenv("PARA_IMAGE_PROTOCOL")) {
	case "kitty":
		return Kitty
	case "iterm2":
		return ITerm2
	case "blocks":
		return Blocks
	}
	if getenv("TMUX") != "" || getenv("STY") != "" {
		return Blocks
	}
	switch {
	case getenv("KITTY_WINDOW_ID") != "", getenv("TERM") == "xterm-kitty",
		getenv("TERM_PROGRAM") == "ghostty", getenv("TERM") == "xterm-ghostty":
		return Kitty
	case getenv("TERM_PROGRAM") == "iTerm.app":
		return ITerm2
	}
	return Blocks
}

// kittySequence places the PNG in data over c x r cells. The cursor does not
// move, so the layout around the image is left alone.
func kittySequence(data []byte, c, r int) string {
	payload := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for i := 0; ; i += kittyChunk {
		end := min(i+kittyChunk, len(payload))
		more := 0
		if end < len(payload) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,t=d,i=%d,c=%d,r=%d,C=1,q=2,m=%d;", kittyID, c, r, more)
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;", more)
		}
		b.WriteString(payload[i:end])
		b.WriteString("\x1b\\")
		if more == 0 {
			return b.String()
		}
	}
}

// iterm2Sequence places the PNG in data over c x r cells without moving the
// cursor.
func iterm2Sequence(data []byte, c, r int) string {
	return fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=1;doNotMoveCursor=1:%s\x07",
		len(data), c, r, base64.StdEncoding.EncodeToString(data))
}
