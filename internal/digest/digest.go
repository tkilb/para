// Package digest turns one imported source (a transcript, a page, a thread)
// into grouped notes by asking an agent, and checks what the agent returns.
// The source and the agent's answer are untrusted: nothing here executes
// either or uses either as a path.
package digest

import (
	"context"
	"errors"
)

// Limits on what an agent may return. Anything over a limit is cut or
// dropped, never rejected, so one long answer cannot sink an import.
const (
	// MaxInputBytes is the largest agent answer Parse accepts.
	MaxInputBytes = 1 << 20
	// MaxNotes is how many notes are kept.
	MaxNotes = 40
	// MaxBodyRunes is the longest note body kept.
	MaxBodyRunes = 20000
	// MaxTags is how many tags a note keeps.
	MaxTags = 8
	// MaxPeople is how many people are kept.
	MaxPeople = 50
	// MaxActions is how many action items are kept.
	MaxActions = 100
	// MaxActionRunes is the longest action item kept.
	MaxActionRunes = 300
	// MaxFacts is how many facts are kept.
	MaxFacts = 200
	// MaxFactRunes is the longest fact kept.
	MaxFactRunes = 500
	// MaxConclusions is how many conclusions are kept.
	MaxConclusions = 100
	// MaxConclusionRunes is the longest conclusion kept.
	MaxConclusionRunes = 500
)

var (
	// ErrBadJSON reports an answer that is not one JSON object of the expected
	// shape. It carries no part of the answer.
	ErrBadJSON = errors.New("the agent did not return the expected JSON")
	// ErrEmpty reports an answer with no usable notes, action items, facts or conclusions.
	ErrEmpty = errors.New("the agent returned nothing usable")
	// ErrTooLarge reports an answer over MaxInputBytes.
	ErrTooLarge = errors.New("the agent's answer is too large")
)

// Note is one file of related concepts and ideas.
type Note struct {
	Title string
	Body  string
	Tags  []string
}

// Digest is a checked agent answer: safe to write once the folder, kind and
// dates are added by the writer.
type Digest struct {
	// Topic is a short cleaned name for the folder; empty when the agent gave
	// none, and the writer then names the folder after the source.
	Topic string
	// Source is one of the configured sources, "adhoc" when unsure.
	Source  string
	Notes   []Note
	Actions []string
	// Facts are standalone statements the source gives, kept apart from the
	// notes so they can be picked into a notebook one at a time.
	Facts []string
	// Conclusions are what the source settled, decided or found out, with the
	// reason or owner when it gives one. They may repeat the notes and facts.
	Conclusions []string
	// People are those who take part in the source, each as first name and
	// last initial, such as "Tyler K.". They go on every note of the import.
	People []string
	// Unmerged is set when a large source was split and the merge failed, so
	// the notes of the parts are kept side by side. The caller warns.
	Unmerged bool
}

// Digester runs the agent over one source text.
type Digester interface {
	Digest(ctx context.Context, source string) (Digest, error)
}
