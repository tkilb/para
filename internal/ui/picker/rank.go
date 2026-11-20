package picker

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sahilm/fuzzy"
)

// How well a word matched an item, best last. A contiguous run beats the same
// letters scattered, a match at the start of the name or of one of its words
// beats one in the middle, and a match in the name beats one only in the rest
// of the label (the folders of a path). An item with a Lead or Body is also
// found by them, below every match in the label.
const (
	qBodyHit       = iota // contiguous in the item's Body
	qLeadHit              // contiguous in the item's Lead
	qPathFuzzy            // scattered across the whole label
	qPathExact            // contiguous, but only in the folders
	qNameFuzzy            // scattered across the name
	qContains             // contiguous inside the name
	qWordStart            // contiguous at the start of a word of the name
	qNamePrefix           // contiguous at the start of the name
	qNameIsTheWord        // the whole name
)

// wordMatch is how one word matched one item: a quality, a score that orders
// matches of the same quality (higher is better), and the byte offsets in
// the label of the characters to highlight.
type wordMatch struct {
	quality int
	score   int
	at      []int
}

// name returns the part of the item's label that matters most, and where it
// starts in the label: the item's Name when it is the end of the label,
// else the whole label.
func (it Item) name() (string, int) {
	if it.Name != "" && strings.HasSuffix(it.Label, it.Name) {
		return it.Name, len(it.Label) - len(it.Name)
	}
	return it.Label, 0
}

// contiguous finds word inside target, ignoring case, and returns the best
// quality it has there (qNameIsTheWord down to qContains) and where. It
// reports false when word is not in target as a whole run (or the text
// changes length when lowered, as some scripts do, so offsets cannot be
// trusted).
func contiguous(target, word string) (quality, at int, ok bool) {
	lt, lw := strings.ToLower(target), strings.ToLower(word)
	if len(lt) != len(target) || len(lw) != len(word) || lw == "" {
		return 0, 0, false
	}
	if lt == lw {
		return qNameIsTheWord, 0, true
	}
	best, bestAt := -1, 0
	for from := 0; from < len(lt); {
		i := strings.Index(lt[from:], lw)
		if i < 0 {
			break
		}
		i += from
		q := qContains
		switch {
		case i == 0:
			q = qNamePrefix
		case startsWord(target, i):
			q = qWordStart
		}
		if q > best {
			best, bestAt = q, i
		}
		from = i + 1
	}
	return best, bestAt, best >= 0
}

// startsWord reports whether the character at byte i of s begins a word:
// after anything that is not a letter or digit, or at a lower to upper case
// change such as the C in "meetingCTO".
func startsWord(s string, i int) bool {
	prev, _ := utf8.DecodeLastRuneInString(s[:i])
	cur, _ := utf8.DecodeRuneInString(s[i:])
	if !unicode.IsLetter(prev) && !unicode.IsDigit(prev) {
		return true
	}
	return unicode.IsLower(prev) && unicode.IsUpper(cur)
}

// runeStarts returns the byte offset of each character of s[at:at+n],
// shifted by base.
func runeStarts(s string, at, n, base int) []int {
	var out []int
	for i := at; i < at+n && i < len(s); {
		out = append(out, base+i)
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
	}
	return out
}

// rank matches word against every item in items (indexes into m.items) and
// returns the matches by position in items; an item the word does not fit
// is missing. Scattered matches come from the fuzzy library.
func (m Model) rank(word string, items []int) map[int]wordMatch {
	labels, names := make([]string, len(items)), make([]string, len(items))
	for i, it := range items {
		labels[i] = m.items[it].Label
		names[i], _ = m.items[it].name()
	}
	scatteredInName := map[int]fuzzy.Match{}
	for _, f := range fuzzy.Find(word, names) {
		scatteredInName[f.Index] = f
	}
	out := map[int]wordMatch{}
	for _, f := range fuzzy.Find(word, labels) {
		it := m.items[items[f.Index]]
		name, start := it.name()
		if q, at, ok := contiguous(name, word); ok {
			out[f.Index] = wordMatch{q, -len(name), runeStarts(name, at, len(word), start)}
		} else if nm, ok := scatteredInName[f.Index]; ok {
			at := make([]int, len(nm.MatchedIndexes))
			for i, b := range nm.MatchedIndexes {
				at[i] = start + b
			}
			out[f.Index] = wordMatch{qNameFuzzy, nm.Score, at}
		} else if _, at, ok := contiguous(it.Label, word); ok {
			out[f.Index] = wordMatch{qPathExact, -len(it.Label), runeStarts(it.Label, at, len(word), 0)}
		} else {
			out[f.Index] = wordMatch{qPathFuzzy, f.Score, f.MatchedIndexes}
		}
	}
	for i, it := range items {
		if _, ok := out[i]; ok {
			continue
		}
		it := m.items[it]
		if at := strings.Index(strings.ToLower(it.Lead), strings.ToLower(word)); at >= 0 && word != "" {
			out[i] = wordMatch{qLeadHit, -at, nil}
		} else if at := strings.Index(strings.ToLower(it.Body), strings.ToLower(word)); at >= 0 && word != "" {
			out[i] = wordMatch{qBodyHit, -at, nil}
		}
	}
	return out
}
