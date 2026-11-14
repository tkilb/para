package para

import "github.com/tkilb/para/internal/undo"

// Effect is what a key press asks the root model to do besides updating the
// panel: record an undo action, show a message, or ask for confirmation.
type Effect struct {
	// Action, when set, is pushed onto the undo history.
	Action *undo.Action
	// Info, Warn and Err are notices shown as popups. Routine successes set
	// none of them: the list already shows what happened.
	Info string
	Warn string
	Err  error
	// Edit, when set, is the root-relative path of the note to open in the
	// editor.
	Edit string
	// Rename, when set, asks the root model to rename an item; it owns the
	// link index that rewriting references needs.
	Rename *RenameRequest
	// SetKind, when set, asks the root model to pick a kind for a note.
	SetKind *KindRequest
	// Mark, when set, is the root-relative path of the note whose mark to
	// toggle; the root model owns the uids and their persistence.
	Mark string
	// Confirm, when set, asks the user before anything changes.
	Confirm *Confirm
	// Prompt, when set, asks the user to type a line of text.
	Prompt *Prompt
}

// ConfirmOp names what happens when a Confirm is accepted.
type ConfirmOp int

const (
	// OpOverwritePaste pastes over items that already exist.
	OpOverwritePaste ConfirmOp = iota + 1
	// OpTrash moves the targets to the trash.
	OpTrash
	// OpDelete permanently deletes the targets.
	OpDelete
)

// Confirm is a yes/no question. The root model shows Prompt and, on yes,
// calls Model.ApplyConfirm with Op.
type Confirm struct {
	Prompt string
	Op     ConfirmOp
	// Danger draws the question in red, for actions that remove items.
	Danger bool
}

// InputOp names what happens with the text a Prompt collects.
type InputOp int

const (
	// OpCreate creates a note, or a folder with a trailing slash.
	OpCreate InputOp = iota + 1
	// OpRename renames the hovered item to the text.
	OpRename
	// OpSearch jumps to the first entry whose name contains the text.
	OpSearch
	// OpClearSearch discards the active search.
	OpClearSearch
)

// Prompt asks for one line of text and, on enter, calls Model.ApplyInput
// with Op and the text. The root model shows it as an input popup titled
// Title with Label above the field, or, when Inline is set, as a line at the
// bottom of the screen that takes the place of the key hints.
type Prompt struct {
	Title       string
	Label       string
	Placeholder string
	// Initial is the text the field starts with.
	Initial string
	Op      InputOp
	Inline  bool
	// CancelOp, when set, is applied (with empty text) on esc instead of just
	// closing the prompt, so esc also undoes what the prompt set.
	CancelOp InputOp
}
