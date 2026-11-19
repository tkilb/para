package digest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tkilb/para/internal/intake"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/textsafe"
)

const fallbackSource = "adhoc"

type rawDigest struct {
	Topic   string    `json:"topic"`
	Source  string    `json:"source"`
	Notes   []rawNote `json:"notes"`
	Actions []string  `json:"actions"`
	Facts   []string  `json:"facts"`

	Takeaways struct {
		KeyPoints   []string `json:"key_points"`
		Conclusions []string `json:"conclusions"`
		Decisions   []string `json:"decisions"`
	} `json:"takeaways"`
}

type rawNote struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

// Parse checks an agent's answer: exactly one JSON object, then cleans it.
// Unknown fields are ignored so a prompt may ask for more. Titles become safe
// file names, text loses control characters, lists and lengths are cut to the
// limits, and the source is one of sources ("adhoc" when it is not). The
// error never contains any of the answer.
func Parse(data []byte, sources []string) (Digest, error) {
	if len(data) > MaxInputBytes {
		return Digest{}, ErrTooLarge
	}
	var raw rawDigest
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return Digest{}, ErrBadJSON
	}
	if _, err := dec.Token(); err != io.EOF {
		return Digest{}, ErrBadJSON
	}

	d := Digest{
		Topic:   cleanName(raw.Topic),
		Source:  matchSource(raw.Source, sources),
		Notes:   cleanNotes(raw.Notes),
		Actions: cleanActions(raw.Actions),
		Facts:   cleanFacts(raw.Facts),
		Takeaways: cleanTakeaways(Takeaways{
			KeyPoints:   raw.Takeaways.KeyPoints,
			Conclusions: raw.Takeaways.Conclusions,
			Decisions:   raw.Takeaways.Decisions,
		}),
	}
	if len(d.Notes) == 0 && len(d.Actions) == 0 && len(d.Facts) == 0 && d.Takeaways.Empty() {
		return Digest{}, ErrEmpty
	}
	return d, nil
}

// cleanName makes text a safe file name, with no leading underscore: files
// the writer owns, _source.md, _actions.md, _facts.md and _takeaways.md, start with one.
func cleanName(text string) string {
	name := store.CleanTitle(textsafe.SingleLine(text))
	return strings.TrimLeft(name, "_ ")
}

func matchSource(text string, sources []string) string {
	want := strings.TrimSpace(text)
	for _, s := range sources {
		if strings.EqualFold(s, want) && want != "" {
			return s
		}
	}
	return fallbackSource
}

func cleanNotes(raw []rawNote) []Note {
	var notes []Note
	seen := map[string]bool{}
	for _, r := range raw {
		if len(notes) == MaxNotes {
			break
		}
		title := cleanName(r.Title)
		body := cutRunes(strings.TrimSpace(textsafe.Sanitize(r.Body)), MaxBodyRunes)
		if title == "" || body == "" {
			continue
		}
		notes = append(notes, Note{Title: uniqueTitle(title, seen), Body: body, Tags: cleanTags(r.Tags)})
	}
	return notes
}

// uniqueTitle returns title, or title with " 2", " 3" and so on, so that no
// two notes share a name ignoring case, and records the result in seen.
func uniqueTitle(title string, seen map[string]bool) string {
	name := title
	for n := 2; seen[strings.ToLower(name)]; n++ {
		name = fmt.Sprintf("%s %d", title, n)
	}
	seen[strings.ToLower(name)] = true
	return name
}

// cleanTags keeps the valid, distinct tags of a note, up to MaxTags. A tag
// the intake rules reject, or that holds a space or comma, is dropped alone.
func cleanTags(raw []string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, t := range raw {
		if len(tags) == MaxTags {
			break
		}
		name := strings.TrimPrefix(strings.TrimSpace(textsafe.SingleLine(t)), "#")
		if name == "" || strings.ContainsAny(name, ", \t") {
			continue
		}
		parsed, err := intake.ParseTags(name)
		if err != nil || len(parsed) != 1 {
			continue
		}
		if key := strings.ToLower(parsed[0]); !seen[key] {
			seen[key] = true
			tags = append(tags, parsed[0])
		}
	}
	return tags
}

func cleanActions(raw []string) []string {
	return cleanLines(raw, MaxActions, MaxActionRunes)
}

func cleanFacts(raw []string) []string {
	return cleanLines(raw, MaxFacts, MaxFactRunes)
}

func cleanTakeaways(t Takeaways) Takeaways {
	return Takeaways{
		KeyPoints:   cleanLines(t.KeyPoints, MaxTakeaways, MaxTakeawayRunes),
		Conclusions: cleanLines(t.Conclusions, MaxTakeaways, MaxTakeawayRunes),
		Decisions:   cleanLines(t.Decisions, MaxTakeaways, MaxTakeawayRunes),
	}
}

// cleanLines keeps up to max non-empty single-line items, each cut to runes.
func cleanLines(raw []string, max, runes int) []string {
	var out []string
	for _, r := range raw {
		if len(out) == max {
			break
		}
		r = cutRunes(strings.TrimSpace(textsafe.SingleLine(r)), runes)
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}

func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
