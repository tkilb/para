package imgfmt

import (
	"encoding/xml"
	"regexp"
	"strings"
)

const (
	svgNS  = "http://www.w3.org/2000/svg"
	xsltNS = "http://www.w3.org/1999/XSL/Transform"
	// xmlNS is the namespace of the xml: prefix, which the decoder may also
	// leave as the prefix itself.
	xmlNS = "http://www.w3.org/XML/1998/namespace"

	xmlDeclTarget = "xml"
	bareDoctype   = "doctype svg"
	fragmentHref  = "#"
	importRule    = "@import"
	cssEscape     = "\\" // a CSS escape could hide "url(" from the scan
	urlOpen       = "url("
	urlClose      = ")"
	cssQuotes     = `"'`
	handlerPrefix = "on"
)

var (
	// encodingDecl finds the encoding in an XML declaration.
	encodingDecl = regexp.MustCompile(`(?i)encoding\s*=\s*["']([^"']*)["']`)

	// safeDataPrefixes are the only data URLs an SVG may hold: raster images.
	safeDataPrefixes = []string{"data:image/png", "data:image/jpeg", "data:image/gif", "data:image/webp"}

	// animationElements can set any attribute, an href or handler included.
	animationElements = map[string]bool{"set": true, "animate": true, "animatetransform": true, "animatemotion": true}
)

// checkDirective allows only a bare "<!DOCTYPE svg>": no internal subset, no
// entity or attribute declarations, no external identifier.
func checkDirective(d xml.Directive) error {
	if strings.EqualFold(strings.Join(strings.Fields(string(d)), " "), bareDoctype) {
		return nil
	}
	return ErrUnsafe
}

// checkElement applies the safety policy to one element and its attributes.
func checkElement(el xml.StartElement) error {
	local := strings.ToLower(el.Name.Local)
	if el.Name.Space != svgNS || el.Name.Space == xsltNS || local == "script" || local == "foreignobject" {
		return ErrUnsafe
	}
	for _, a := range el.Attr {
		if err := checkAttr(a, animationElements[local]); err != nil {
			return err
		}
	}
	return nil
}

// checkAttr applies the safety policy to one attribute.
func checkAttr(a xml.Attr, animated bool) error {
	local := strings.ToLower(a.Name.Local)
	switch {
	case a.Name.Space == "xmlns" || (a.Name.Space == "" && local == "xmlns"):
		return nil
	case strings.HasPrefix(local, handlerPrefix):
		return ErrUnsafe
	case local == "base" && (a.Name.Space == xmlNS || a.Name.Space == xmlDeclTarget):
		return ErrUnsafe
	case local == "href" && !safeURL(a.Value):
		return ErrUnsafe
	case animated && local == "attributename" && unsafeTarget(a.Value):
		return ErrUnsafe
	}
	return checkCSS(a.Value)
}

// unsafeTarget reports whether an animation's attributeName names an href or
// an event handler, with or without an "xlink:" prefix.
func unsafeTarget(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	n = n[strings.LastIndex(n, ":")+1:]
	return n == "href" || strings.HasPrefix(n, handlerPrefix)
}

// safeURL reports whether a link stays in the file (a "#" fragment) or holds
// the data of a raster image.
func safeURL(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(v, fragmentHref) {
		return true
	}
	for _, p := range safeDataPrefixes {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// checkCSS refuses an "@import" and any url() that leaves the file.
func checkCSS(css string) error {
	v := strings.ToLower(css)
	if strings.Contains(v, importRule) || strings.Contains(v, cssEscape) {
		return ErrUnsafe
	}
	for {
		i := strings.Index(v, urlOpen)
		if i < 0 {
			return nil
		}
		v = v[i+len(urlOpen):]
		arg := v
		if end := strings.Index(v, urlClose); end >= 0 {
			arg = v[:end]
		}
		if !safeURL(strings.Trim(strings.TrimSpace(arg), cssQuotes)) {
			return ErrUnsafe
		}
	}
}
