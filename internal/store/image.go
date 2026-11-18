package store

import "strings"

const imageExt = ".png"

// isImage reports whether name is a PNG image file name, in any case.
func isImage(name string) bool {
	return len(name) > len(imageExt) && strings.EqualFold(name[len(name)-len(imageExt):], imageExt)
}

// IsImage reports whether name is a PNG image file name, in any case. It is
// the only image format para stores.
func IsImage(name string) bool { return isImage(name) }
