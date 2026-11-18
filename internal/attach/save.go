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
	pngExt       = ".png"
	pasteTimeout = 15 * time.Second
	maxNameTries = 1000
)

// Source supplies the image on the clipboard.
type Source interface {
	Image(ctx context.Context) ([]byte, error)
}

// Result describes a saved image.
type Result struct {
	// Rel is the new file, relative to the notebook root.
	Rel string
	// Action undoes (and redoes) the save.
	Action undo.Action
}

// save writes the PNG in data to dir as "stem.png", with a number added to the
// stem when that name is taken.
func save(s store.Store, dir, stem string, data []byte) (Result, error) {
	rel, err := freeName(s, dir, stem)
	if err != nil {
		return Result{}, err
	}
	act, err := s.Create(rel, data)
	if err != nil {
		return Result{}, err
	}
	return Result{Rel: rel, Action: act}, nil
}

// freeName returns the first unused "stem.png" name in dir, then "stem 2.png"
// and so on.
func freeName(s store.Store, dir, stem string) (string, error) {
	for n := 1; n <= maxNameTries; n++ {
		name := stem
		if n > 1 {
			name = fmt.Sprintf("%s %d", stem, n)
		}
		rel := filepath.ToSlash(filepath.Join(dir, name+pngExt))
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
