package store

import "strings"

const (
	// PNGExt, SVGExt and PDFExt are the extensions of the attachment types para
	// stores. A PDF is not an image, but it is listed, bundled, renamed and
	// deleted the same way.
	PNGExt = ".png"
	SVGExt = ".svg"
	PDFExt = ".pdf"
)

// imageExts lists the stored image extensions, in lower case.
var imageExts = []string{PNGExt, SVGExt, PDFExt}

// ImageExt returns the extension of an image file name in lower case (".png",
// ".svg" or ".pdf"), or "" when name is not an image file name. A name that is only
// the extension is not an image.
func ImageExt(name string) string {
	for _, ext := range imageExts {
		if len(name) > len(ext) && strings.EqualFold(name[len(name)-len(ext):], ext) {
			return ext
		}
	}
	return ""
}

// ImageStem returns an image file name without its extension, in whatever case
// the extension has. A name that is not an image file name is returned as it
// is.
func ImageStem(name string) string {
	return name[:len(name)-len(ImageExt(name))]
}

// isImage reports whether name is a PNG or SVG image file name, in any case.
func isImage(name string) bool { return ImageExt(name) != "" }

// IsImage reports whether name is a PNG or SVG image file name, in any case.
// They are the only image formats para stores.
func IsImage(name string) bool { return isImage(name) }

// ImageDisplayNames returns the name to show for each of names, which are the
// file names of one listing (one folder, or one bundle). A PNG shows its stem
// only. An SVG or PDF shows its stem only, unless another image in the listing has the
// same stem, compared without regard to case, and then it shows the stem and
// ".svg" (or ".pdf") so that the two rows differ. A name that is not an image passes
// through untouched and does not count as a collision.
func ImageDisplayNames(names []string) []string {
	counts := make(map[string]int, len(names))
	for _, name := range names {
		if isImage(name) {
			counts[strings.ToLower(ImageStem(name))]++
		}
	}
	shown := make([]string, len(names))
	for i, name := range names {
		shown[i] = name
		if !isImage(name) {
			continue
		}
		shown[i] = ImageStem(name)
		if ext := ImageExt(name); ext != PNGExt && counts[strings.ToLower(shown[i])] > 1 {
			shown[i] += ext
		}
	}
	return shown
}
