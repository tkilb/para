package para

import "github.com/tkilb/para/internal/store"

// KindRequest asks to choose a new kind for the note at Rel, which has
// Current now.
type KindRequest struct {
	Rel     string
	Current store.Kind
}

// askKind asks the root model to open the kind picker for the hovered note.
// A folder has no kind, so it does nothing.
func (m Model) askKind() (Model, Effect) {
	t := m.tabs[m.active]
	if t.cursor >= len(t.rows) || t.rows[t.cursor].entry.IsDir {
		return m, Effect{}
	}
	r := t.rows[t.cursor]
	if r.entry.IsImage {
		return m, Effect{Info: "An image has no kind."}
	}
	if _, daily := store.DailyDate(r.entry.Rel); daily {
		return m, Effect{Info: "A daily note has no kind: digest it into notes, then archive it."}
	}
	return m, Effect{SetKind: &KindRequest{Rel: r.entry.Rel, Current: r.kind}}
}
