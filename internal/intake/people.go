package intake

import (
	"strings"

	"github.com/tkilb/para/internal/textsafe"
)

// ParseNames splits text typed into a people box into names. Names are
// separated by commas, since a name has spaces. Each is kept as typed, except
// that control characters and runs of white space become one space; empty
// names and duplicates (ignoring case) are dropped and the order is kept.
func ParseNames(text string) []string {
	var names []string
	seen := map[string]bool{}
	for _, part := range strings.Split(text, ",") {
		name := strings.Join(strings.Fields(textsafe.SingleLine(part)), " ")
		if key := strings.ToLower(name); name != "" && !seen[key] {
			seen[key] = true
			names = append(names, name)
		}
	}
	return names
}
