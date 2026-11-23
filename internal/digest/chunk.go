package digest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// SinglePassBytes is the largest source sent to the agent in one pass, and
// the size parts are cut to when a source is larger. About 400 KiB is roughly
// 100 000 tokens: more than an hour of meeting talk.
const SinglePassBytes = 400 << 10

// Split cuts text into parts of at most limit bytes that join back into text
// exactly. It cuts after the last blank line that fits, which is where a
// speaker's turn or a message ends, else after the last line break, else at a
// character boundary. A cut is not made in the first quarter of a part when
// a later kind of break is available. A limit of zero or less, or text that
// fits, gives one part.
func Split(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}
	var parts []string
	rest := text
	for len(rest) > limit {
		cut := cutPoint(rest, limit)
		parts = append(parts, rest[:cut])
		rest = rest[cut:]
		if strings.TrimSpace(rest) == "" {
			parts[len(parts)-1] += rest
			return parts
		}
	}
	return append(parts, rest)
}

// cutPoint is the length of the part to cut from rest, which is longer than
// limit.
func cutPoint(rest string, limit int) int {
	window := rest[:limit]
	minCut := limit / 4
	if cut := lastBlankLine(window); cut > minCut {
		return cut
	}
	if i := strings.LastIndexByte(window, '\n'); i+1 > minCut {
		return i + 1
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(rest[cut]) {
		cut--
	}
	if cut == 0 {
		_, cut = utf8.DecodeRuneInString(rest)
	}
	return cut
}

// lastBlankLine returns the end of the last blank line in window, or 0.
func lastBlankLine(window string) int {
	end := 0
	for _, sep := range []string{"\n\n", "\r\n\r\n"} {
		if i := strings.LastIndex(window, sep); i >= 0 && i+len(sep) > end {
			end = i + len(sep)
		}
	}
	return end
}

// Merger merges the partial results of one split source into one result.
type Merger interface {
	Merge(ctx context.Context, parts []Digest) (Digest, error)
}

// Chunked is a Digester for sources of any size: a source that fits in one
// pass goes straight to Part, and a larger one is split, each part is digested
// in order, and Merge combines the results.
//
// A failed part fails the import, because the result would be incomplete.
// A part with nothing usable (small talk only) is skipped. A failed merge is
// not a failure: the notes of the parts are kept side by side and the result
// is marked Unmerged. Cancelling stops everything.
type Chunked struct {
	Part  Digester
	Merge Merger
	// Limit is the pass size in bytes; zero means SinglePassBytes.
	Limit int
	// OnPart, when set, is called with the parts done and the total: before
	// the first part and after each one. It is not called for one pass.
	OnPart func(done, total int)
}

// Digest implements Digester.
func (c Chunked) Digest(ctx context.Context, source string) (Digest, error) {
	limit := c.Limit
	if limit <= 0 {
		limit = SinglePassBytes
	}
	parts := Split(source, limit)
	if len(parts) == 1 {
		return c.Part.Digest(ctx, source)
	}
	results, err := c.digestParts(ctx, parts)
	switch {
	case err != nil:
		return Digest{}, err
	case len(results) == 0:
		return Digest{}, ErrEmpty
	case len(results) == 1:
		return results[0], nil
	}
	merged, err := c.Merge.Merge(ctx, results)
	if err == nil {
		return merged, nil
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return Digest{}, err
	}
	kept := concat(results)
	kept.Unmerged = true
	return kept, nil
}

func (c Chunked) digestParts(ctx context.Context, parts []string) ([]Digest, error) {
	var results []Digest
	c.progress(0, len(parts))
	for i, p := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, err := c.Part.Digest(ctx, p)
		switch {
		case errors.Is(err, ErrEmpty):
		case err != nil:
			return nil, fmt.Errorf("part %d of %d: %w", i+1, len(parts), err)
		default:
			results = append(results, d)
		}
		c.progress(i+1, len(parts))
	}
	return results, nil
}

func (c Chunked) progress(done, total int) {
	if c.OnPart != nil {
		c.OnPart(done, total)
	}
}

// concat joins partial results as they are: every note and action in order,
// with the first topic and source there is.
func concat(parts []Digest) Digest {
	var out Digest
	for _, p := range parts {
		if out.Topic == "" {
			out.Topic = p.Topic
		}
		if out.Source == "" {
			out.Source = p.Source
		}
		out.Notes = append(out.Notes, p.Notes...)
		out.Actions = append(out.Actions, p.Actions...)
		out.Facts = append(out.Facts, p.Facts...)
		out.Conclusions = append(out.Conclusions, p.Conclusions...)
		out.People = append(out.People, p.People...)
	}
	out.People = cleanPeople(out.People)
	return out
}

type partJSON struct {
	Topic       string     `json:"topic"`
	Source      string     `json:"source"`
	Notes       []noteJSON `json:"notes"`
	Actions     []string   `json:"actions"`
	Facts       []string   `json:"facts"`
	Conclusions []string   `json:"conclusions"`
	People      []string   `json:"people"`
}

type noteJSON struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

// MarshalParts encodes partial results as a JSON list in the shape of the
// schema, which is the input of the merge prompt. Empty lists are lists, not
// null.
func MarshalParts(parts []Digest) ([]byte, error) {
	out := make([]partJSON, 0, len(parts))
	for _, p := range parts {
		pj := partJSON{Topic: p.Topic, Source: p.Source, Notes: []noteJSON{}, Actions: append([]string{}, p.Actions...), Facts: append([]string{}, p.Facts...), Conclusions: append([]string{}, p.Conclusions...), People: append([]string{}, p.People...)}
		for _, n := range p.Notes {
			pj.Notes = append(pj.Notes, noteJSON{Title: n.Title, Body: n.Body, Tags: append([]string{}, n.Tags...)})
		}
		out = append(out, pj)
	}
	return json.Marshal(out)
}

// Merge implements Merger: it sends the partial results to the agent as JSON
// on standard input. Use a CLI whose Prompt is the merge prompt.
func (c CLI) Merge(ctx context.Context, parts []Digest) (Digest, error) {
	data, err := MarshalParts(parts)
	if err != nil {
		return Digest{}, fmt.Errorf("%w: encode the parts", ErrBackendFailed)
	}
	return c.Digest(ctx, string(data))
}
