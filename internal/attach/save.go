package attach

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/tkilb/para/internal/intake"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

const (
	nameLayout   = "2006-01-02 15-04-05"
	pasteTimeout = 15 * time.Second
	maxNameTries = 1000
)

// Source supplies the clipboard: an image, or text that may be an SVG.
type Source interface {
	Image(ctx context.Context) ([]byte, error)
	Text(ctx context.Context) (string, error)
}

// Result describes a saved image.
type Result struct {
	// Rel is the new file, relative to the notebook root.
	Rel string
	// Action undoes (and redoes) the save.
	Action undo.Action
}

// save writes the image in data to dir as "stem" and ext (".png" or ".svg"), with a number added to the
// stem when that name is taken.
func save(s store.Store, dir, stem, ext string, data []byte) (Result, error) {
	rel, err := freeName(s, dir, stem, ext)
	if err != nil {
		return Result{}, err
	}
	act, err := s.Create(rel, data)
	if err != nil {
		return Result{}, err
	}
	return Result{Rel: rel, Action: act}, nil
}

// freeName returns the first unused "stem" plus ext name in dir, then "stem 2"
// plus ext and so on.
func freeName(s store.Store, dir, stem, ext string) (string, error) {
	for n := 1; n <= maxNameTries; n++ {
		name := stem
		if n > 1 {
			name = fmt.Sprintf("%s %d", stem, n)
		}
		rel := filepath.ToSlash(filepath.Join(dir, name+ext))
		taken, err := s.Exists(rel)
		if err != nil {
			return "", err
		}
		if !taken {
			return rel, nil
		}
	}
	return "", intake.ErrNoFreeName
}
