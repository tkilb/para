package digest

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tkilb/para/internal/textsafe"
)

// cleanPeople returns the distinct people, up to MaxPeople, each as first name
// and last initial ("Tyler K."). A name with no last word to take an initial
// from is dropped, never guessed, so every entry has the same form.
func cleanPeople(raw []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range raw {
		if len(out) == MaxPeople {
			break
		}
		name, ok := personName(r)
		if !ok || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

// personName shortens a full name: its first word, then the first letter of
// its last word, with a period. "Tyler Kilburn", "Tyler K" and "tyler k." are
// all "Tyler K.".
func personName(raw string) (string, bool) {
	words := strings.Fields(textsafe.SingleLine(raw))
	if len(words) < 2 {
		return "", false
	}
	first := capitalize(words[0])
	last, _ := utf8.DecodeRuneInString(strings.TrimRight(words[len(words)-1], "."))
	if first == "" || last == utf8.RuneError || !unicode.IsLetter(last) {
		return "", false
	}
	return first + " " + string(unicode.ToUpper(last)) + ".", true
}

// capitalize upper-cases the first letter of word and lower-cases the rest
// only when word is all one case, so "McDonald" stays as it is.
func capitalize(word string) string {
	if word == "" {
		return ""
	}
	if word != strings.ToLower(word) && word != strings.ToUpper(word) {
		return word
	}
	r, size := utf8.DecodeRuneInString(word)
	return string(unicode.ToUpper(r)) + strings.ToLower(word[size:])
}
