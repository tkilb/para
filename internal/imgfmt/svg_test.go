package imgfmt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	svgOpen  = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 10 10">`
	svgClose = `</svg>`
	svgPlain = svgOpen + `<rect width="10" height="10" fill="red"/>` + svgClose
)

func svgWith(inner string) []byte { return []byte(svgOpen + inner + svgClose) }

func TestPrepareKeepsAValidSVGByteForByte(t *testing.T) {
	bom := "\xef\xbb\xbf"
	tests := []struct {
		name string
		in   string
	}{
		{"plain", svgPlain},
		{"byte order mark", bom + svgPlain},
		{"leading space and newline", "\n  " + svgPlain + "\n"},
		{"xml declaration in lower case encoding", `<?xml version="1.0" encoding="utf-8"?>` + svgPlain},
		{"xml declaration", `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + svgPlain},
		{"comment first", "<!-- logo -->" + svgPlain},
		{"bare doctype", `<!DOCTYPE svg>` + svgPlain},
		{"gradient with url fill", string(svgWith(`<defs><linearGradient id="grad"><stop offset="0" stop-color="red"/></linearGradient></defs><rect fill="url(#grad)" style="stroke:url('#grad')"/>`))},
		{"style with plain css", string(svgWith(`<style>rect { fill: blue; stroke: url(#grad) }</style>`))},
		{"style in a cdata section", string(svgWith(`<style><![CDATA[rect { fill: blue }]]></style>`))},
		{"png image", string(svgWith(`<image href="data:image/png;base64,AAAA"/>`))},
		{"css data image", string(svgWith(`<style>rect { fill: url("data:image/webp;base64,AAAA") }</style>`))},
		{"set on opacity", string(svgWith(`<rect><set attributeName="opacity" to="0"/></rect>`))},
		{"animate on fill", string(svgWith(`<rect><animate attributeName="fill" to="red"/></rect>`))},
		{"xml lang", string(svgWith(`<text xml:lang="en">a</text>`))},
		{"fragment href", string(svgWith(`<defs><g id="a"/></defs><use href="#a"/><use xlink:href="#a"/>`))},
		{"data image href", string(svgWith(`<image href="data:image/png;base64,AAAA"/>`))},
		{"data image href in other case", string(svgWith(`<image xlink:href=" DATA:IMAGE/png;base64,AAAA"/>`))},
		{"style element", string(svgWith(`<style>rect { fill: blue }</style>`))},
		{"an attribute that only contains on", string(svgWith(`<rect width="1" height="1" data-on="x" font-size="2"/>`))},
		{"a namespace declaration named on", string(svgWith(`<g xmlns:one="urn:x"/>`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, ext, err := Prepare([]byte(tt.in))
			require.NoError(t, err)
			assert.Equal(t, ".svg", ext)
			assert.Equal(t, tt.in, string(out), "an SVG is stored as it came")
		})
	}
}

func TestPrepareConvertsRasterToPNG(t *testing.T) {
	in := encodeJPEG(t, sample(16, 12))
	out, ext, err := Prepare(in)
	require.NoError(t, err)
	assert.Equal(t, ".png", ext)
	want, err := ToPNG(in)
	require.NoError(t, err)
	assert.Equal(t, want, out)

	png := encodePNG(t, sample(8, 8))
	out, ext, err = Prepare(png)
	require.NoError(t, err)
	assert.Equal(t, ".png", ext)
	assert.Equal(t, png, out, "a PNG is stored as it came")
}

func TestPrepareRejectsUnsafeSVG(t *testing.T) {
	tooDeep := svgOpen + strings.Repeat("<g>", maxSVGDepth) + strings.Repeat("</g>", maxSVGDepth) + svgClose
	tests := []struct {
		name string
		in   string
	}{
		{"script element", string(svgWith(`<script>alert(1)</script>`))},
		{"script in another case", string(svgWith(`<SCRIPT>alert(1)</SCRIPT>`))},
		{"script with a prefix", string(svgWith(`<s:script xmlns:s="http://www.w3.org/2000/svg"/>`))},
		{"foreignObject", string(svgWith(`<foreignObject><div/></foreignObject>`))},
		{"foreignObject in another case", string(svgWith(`<foreignobject/>`))},
		{"onload on the root", `<svg xmlns="http://www.w3.org/2000/svg" onload="x()"></svg>`},
		{"onclick", string(svgWith(`<rect onclick="x()"/>`))},
		{"event handler in upper case", string(svgWith(`<rect ONMOUSEOVER="x()"/>`))},
		{"javascript href", string(svgWith(`<a href="javascript:alert(1)"><rect/></a>`))},
		{"javascript xlink href", string(svgWith(`<a xlink:href="javascript:alert(1)"><rect/></a>`))},
		{"external href", string(svgWith(`<image href="https://example.com/a.png"/>`))},
		{"file href", string(svgWith(`<image href="file:///etc/passwd"/>`))},
		{"relative href", string(svgWith(`<image href="other.svg"/>`))},
		{"empty href", string(svgWith(`<use href=""/>`))},
		{"non image data href", string(svgWith(`<image href="data:text/html;base64,AAAA"/>`))},
		{"entity declaration", `<!DOCTYPE svg [<!ENTITY a "aaaa">]>` + svgPlain},
		{"entity declaration in lower case", `<!DOCTYPE svg [<!entity a "aaaa">]>` + svgPlain},
		{"xml stylesheet", `<?xml-stylesheet type="text/xsl" href="#s"?><svg xmlns="http://www.w3.org/2000/svg"/>`},
		{"other processing instruction", `<?php echo 1 ?>` + svgPlain},
		{"xslt element", string(svgWith(`<x:stylesheet xmlns:x="http://www.w3.org/1999/XSL/Transform"/>`))},
		{"attlist in the doctype", `<!DOCTYPE svg [<!ATTLIST svg onload CDATA "alert(1)">]><svg xmlns="http://www.w3.org/2000/svg"/>`},
		{"element declaration", `<!DOCTYPE svg [<!ELEMENT svg ANY>]>` + svgPlain},
		{"public doctype", `<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">` + svgPlain},
		{"system doctype", `<!DOCTYPE svg SYSTEM "http://evil/x.dtd">` + svgPlain},
		{"html element in svg", `<svg xmlns="http://www.w3.org/2000/svg"><iframe xmlns="http://www.w3.org/1999/xhtml" src="javascript:alert(1)"/></svg>`},
		{"html child in svg", string(svgWith(`<html:iframe xmlns:html="http://www.w3.org/1999/xhtml"/>`))},
		{"set href", `<svg xmlns="http://www.w3.org/2000/svg"><a href="#"><set attributeName="href" to="javascript:alert(1)"/></a></svg>`},
		{"set xlink href", string(svgWith(`<a href="#"><set attributeName="xlink:href" to="javascript:alert(1)"/></a>`))},
		{"animate href in upper case", string(svgWith(`<a href="#"><animate attributeName="HREF" values="javascript:alert(1)"/></a>`))},
		{"animateTransform handler", string(svgWith(`<animateTransform attributeName="onclick"/>`))},
		{"animateMotion handler", string(svgWith(`<animateMotion attributeName=" onload "/>`))},
		{"css import", `<svg xmlns="http://www.w3.org/2000/svg"><style>@import url(http://evil/t.css);</style></svg>`},
		{"css escape hiding a url", string(svgWith(`<style>a{fill:\75rl(http://evil/x)}</style>`))},
		{"css import in upper case", string(svgWith(`<style>@IMPORT "x.css";</style>`))},
		{"css url in cdata", `<svg xmlns="http://www.w3.org/2000/svg"><style><![CDATA[rect{background:url(http://evil/p)}]]></style></svg>`},
		{"css url under a style child", string(svgWith(`<style><b/>rect{fill:url(http://evil/p)}</style>`))},
		{"css url with quotes and space", string(svgWith(`<style>rect{fill:URL( ' http://evil/p ' )}</style>`))},
		{"css url in a style attribute", `<svg xmlns="http://www.w3.org/2000/svg"><rect style="fill:url(http://evil/x)"/></svg>`},
		{"css url in a svg data image", string(svgWith(`<style>rect{fill:url(data:image/svg+xml,x)}</style>`))},
		{"xml base", `<svg xmlns="http://www.w3.org/2000/svg" xml:base="http://evil/"/>`},
		{"svg data href", `<svg xmlns="http://www.w3.org/2000/svg"><use href="data:image/svg+xml,&lt;svg/&gt;#x"/></svg>`},
		{"svg data xlink href", string(svgWith(`<image xlink:href="data:image/svg+xml;base64,AAAA"/>`))},
		{"too deep", tooDeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, ext, err := Prepare([]byte(tt.in))
			assert.ErrorIs(t, err, ErrUnsafe)
			assert.Nil(t, out)
			assert.Empty(t, ext)
		})
	}
}

func TestPrepareAcceptsDepthAtTheCap(t *testing.T) {
	// The root counts as one level.
	in := svgOpen + strings.Repeat("<g>", maxSVGDepth-1) + strings.Repeat("</g>", maxSVGDepth-1) + svgClose
	_, ext, err := Prepare([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, ".svg", ext)
}

func TestPrepareRejectsBrokenOrForeignInput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrUnsupported},
		{"plain text", "not an image at all", ErrUnsupported},
		{"html", "<html><body/></html>", ErrUnsupported},
		{"svg without a namespace", `<svg><rect/></svg>`, ErrUnsupported},
		{"svg in another namespace", `<svg xmlns="http://www.w3.org/1999/xhtml"><iframe src="javascript:alert(1)"/></svg>`, ErrUnsupported},
		{"latin 1 encoding", `<?xml version="1.0" encoding="ISO-8859-1"?>` + svgPlain, ErrUnsupported},
		{"utf 16 declaration", `<?xml version="1.0" encoding='utf-16'?>` + svgPlain, ErrUnsupported},
		{"xml with another root", `<?xml version="1.0"?><note/>`, ErrUnsupported},
		{"an svg word in text", "svg", ErrUnsupported},
		{"truncated svg", svgOpen + `<rect width="1"`, ErrCorrupt},
		{"unclosed root", svgOpen + `<rect/>`, ErrCorrupt},
		{"mismatched tags", svgOpen + `<g></rect>` + svgClose, ErrCorrupt},
		{"truncated before the root ends", `<?xml version="1.0"?><svg xmlns="x"`, ErrCorrupt},
		{"undefined entity", string(svgWith(`<text>&nbsp;</text>`)), ErrCorrupt},
		{"second root", svgPlain + svgPlain, ErrCorrupt},
		{"text after the root", svgPlain + "junk", ErrCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, ext, err := Prepare([]byte(tt.in))
			assert.ErrorIs(t, err, tt.want)
			assert.Nil(t, out)
			assert.Empty(t, ext)
		})
	}
}

func TestPrepareRejectsAnSVGOverTheByteLimit(t *testing.T) {
	in := []byte(svgPlain)
	old := limits
	t.Cleanup(func() { limits = old })
	limits.maxBytes = int64(len(in) - 1)
	out, _, err := Prepare(in)
	assert.ErrorIs(t, err, ErrTooLarge)
	assert.Nil(t, out)
}

func TestPrepareErrorsNeverShowTheContent(t *testing.T) {
	secret := "/Users/someone/secret.svg"
	_, _, err := Prepare(svgWith(`<image href="` + secret + `"/>`))
	require.ErrorIs(t, err, ErrUnsafe)
	assert.NotContains(t, err.Error(), secret)
	_, _, err = Prepare([]byte(svgOpen + `<` + secret))
	require.ErrorIs(t, err, ErrCorrupt)
	assert.NotContains(t, err.Error(), secret)
}

func TestToPNGStillRejectsSVG(t *testing.T) {
	out, err := ToPNG([]byte(svgPlain))
	assert.ErrorIs(t, err, ErrUnsupported)
	assert.Nil(t, out)
}

func TestErrUnsupportedNamesEveryFormat(t *testing.T) {
	assert.Equal(t, "not a PNG, JPEG, WebP, AVIF or SVG image", ErrUnsupported.Error())
}

func TestPrepareTellsAMissingNamespaceApart(t *testing.T) {
	_, _, err := Prepare([]byte(`<svg><rect/></svg>`))
	assert.ErrorIs(t, err, ErrNoNamespace)
	assert.ErrorIs(t, err, ErrUnsupported, "callers that test for ErrUnsupported still match")
	for _, in := range []string{"<html><body/></html>", `<?xml version="1.0"?><note/>`} {
		_, _, err := Prepare([]byte(in))
		assert.NotErrorIs(t, err, ErrNoNamespace, in)
	}
}
