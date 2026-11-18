package attach

import (
	"errors"
	"path"

	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/undo"
)

var (
	// ErrNotNote reports a bundle target that is not a note.
	ErrNotNote = errors.New("choose a note to bundle the image with")
	// ErrNotImage reports an item that is not a PNG image.
	ErrNotImage = errors.New("only a PNG image can be bundled")
	// ErrAlreadyBundled reports an image already in the note's sidecar folder.
	ErrAlreadyBundled = errors.New("the image is already bundled with this note")
)

// Bundle puts the image at imageRel into the sidecar folder of the note at
// noteRel (both relative to the notebook root). It copies the image, or moves
// it when move is set. The image keeps its name, with a number added when that
// name is taken in the folder. The whole step is one undo entry.
func Bundle(s store.Store, noteRel, imageRel string, move bool) (Result, error) {
	side := store.SidecarOf(noteRel)
	switch {
	case side == "":
		return Result{}, ErrNotNote
	case !store.IsImage(path.Base(imageRel)):
		return Result{}, ErrNotImage
	}
	if ok, err := s.Exists(noteRel); err != nil {
		return Result{}, err
	} else if !ok {
		return Result{}, store.ErrNotFound
	}
	if path.Dir(path.Clean(imageRel)) == path.Clean(side) {
		return Result{}, ErrAlreadyBundled
	}
	name := path.Base(imageRel)
	rel, err := freeName(s, side, name[:len(name)-len(".png")])
	if err != nil {
		return Result{}, err
	}
	var act undo.Action
	if move {
		act, err = s.Move(imageRel, rel)
	} else {
		act, _, err = s.Copy(imageRel, rel)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Rel: rel, Action: act}, nil
}
