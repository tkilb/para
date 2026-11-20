package imgfmt

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

const (
	// maxSVGDepth is the deepest element nesting accepted in an SVG, the root
	// counting as one level.
	maxSVGDepth = 256

	pngExt = ".png"
	svgExt = ".svg"

	svgRoot = "svg"
	utf8BOM = "\xef\xbb\xbf"
)

// ErrUnsafe reports an SVG that holds a script, an embedded page, an event
// handler, a link to anywhere but the file itself, a style sheet or DTD, an
// element outside the SVG namespace, or nesting that is too deep. An SVG is stored as it came, so it is refused
// rather than cleaned.
var ErrUnsafe = errors.New("the SVG image is not safe to store")

// Prepare returns data in the form para stores it, and the extension of that
// form. An SVG is returned as it came with ".svg", once checked. Any other
// image goes through ToPNG and returns ".png". It returns ErrUnsupported,
// ErrCorrupt, ErrTooLarge or ErrUnsafe, and never a partial result.
func Prepare(data []byte) ([]byte, string, error) {
	if int64(len(data)) > limits.maxBytes {
		return nil, "", ErrTooLarge
	}
	if !looksLikeXML(data) {
		out, err := ToPNG(data)
		if err != nil {
			return nil, "", err
		}
		return out, pngExt, nil
	}
	if err := checkSVG(data); err != nil {
		return nil, "", err
	}
	return data, svgExt, nil
}

// looksLikeXML reports whether the first byte that is not white space, after
// an optional byte order mark, opens a tag. No raster format starts that way.
func looksLikeXML(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte(utf8BOM))
	data = bytes.TrimLeft(data, " \t\r\n")
	return len(data) > 0 && data[0] == '<'
}

// checkSVG walks every token of data. It returns ErrUnsupported when the root
// element is not an svg in the SVG namespace (or the encoding is not UTF-8),
// ErrCorrupt when the XML is broken, and ErrUnsafe for anything the policy
// refuses.
func checkSVG(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte(utf8BOM))))
	dec.Strict = true
	var w svgWalk
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			if !w.seenRoot {
				return ErrUnsupported
			}
			return nil
		}
		if err != nil {
			return brokenXML(data, w.seenRoot)
		}
		if err := w.token(tok); err != nil {
			return err
		}
	}
}

// svgWalk tracks where checkSVG is in the document.
type svgWalk struct {
	depth    int
	seenRoot bool
	closed   bool
	styleAt  int // depth of the open style element, or 0
}

// token applies the policy to one token.
func (w *svgWalk) token(tok xml.Token) error {
	switch t := tok.(type) {
	case xml.StartElement:
		return w.start(t)
	case xml.EndElement:
		if w.depth == w.styleAt {
			w.styleAt = 0
		}
		w.depth--
		w.closed = w.depth == 0
	case xml.CharData:
		if w.closed && len(bytes.TrimSpace(t)) > 0 {
			return ErrCorrupt
		}
		if w.styleAt > 0 {
			return checkCSS(string(t))
		}
	case xml.ProcInst:
		if t.Target != xmlDeclTarget {
			return ErrUnsafe
		}
	case xml.Directive:
		return checkDirective(t)
	}
	return nil
}

// start applies the policy to a start element and tracks depth.
func (w *svgWalk) start(el xml.StartElement) error {
	if w.closed || (w.depth == 0 && w.seenRoot) {
		return ErrCorrupt
	}
	if !w.seenRoot && (el.Name.Local != svgRoot || el.Name.Space != svgNS) {
		return ErrUnsupported
	}
	w.seenRoot = true
	w.depth++
	if w.depth > maxSVGDepth {
		return ErrUnsafe
	}
	if strings.EqualFold(el.Name.Local, "style") && w.styleAt == 0 {
		w.styleAt = w.depth
	}
	return checkElement(el)
}

// brokenXML picks the error for XML that failed to parse: damaged when it is
// an svg, unsupported when it declares an encoding other than UTF-8 or never
// got as far as a root element.
func brokenXML(data []byte, seenRoot bool) error {
	if !seenRoot && declaresOtherEncoding(data) {
		return ErrUnsupported
	}
	if seenRoot || bytes.Contains(data, []byte("<"+svgRoot)) {
		return ErrCorrupt
	}
	return ErrUnsupported
}

// declaresOtherEncoding reports whether data opens with an XML declaration
// whose encoding is not UTF-8.
func declaresOtherEncoding(data []byte) bool {
	d := bytes.TrimLeft(bytes.TrimPrefix(data, []byte(utf8BOM)), " \t\r\n")
	end := bytes.Index(d, []byte("?>"))
	if !bytes.HasPrefix(d, []byte("<?xml")) || end < 0 {
		return false
	}
	m := encodingDecl.FindSubmatch(d[:end])
	return m != nil && !strings.EqualFold(string(m[1]), "utf-8")
}
