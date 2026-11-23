package store

const (
	sourceKey = "source"
	dateKey   = "date"
	tagsKey   = "tags"
)

// IntakeNote returns the contents of a new intake note named name: the
// properties every intake note carries (uid, title, kind, source, date and
// a tags list, empty when there are none) followed by body. date is
// YYYY-MM-DD. An unset kind writes no kind property.
func IntakeNote(name string, kind Kind, source, date, body string, tags []string) ([]byte, error) {
	return intakeNote(name, kind, source, date, body, tags, nil, nil)
}

// IntakeNoteWithPeople is IntakeNote for a note that also names the people it
// is about, as a people property. An empty list writes no people property.
func IntakeNoteWithPeople(name string, kind Kind, source, date, body string, tags, people []string) ([]byte, error) {
	return intakeNote(name, kind, source, date, body, tags, nil, people)
}

// ExtractedNote is IntakeNote for a note made from lines of another note:
// it has no tags and, when related is not empty, a related property holding
// those wikilinks, such as [[Origin]].
func ExtractedNote(name string, kind Kind, source, date, body string, related []string) ([]byte, error) {
	return intakeNote(name, kind, source, date, body, nil, related, nil)
}

func intakeNote(name string, kind Kind, source, date, body string, tags, related, people []string) ([]byte, error) {
	uid, err := NewUID()
	if err != nil {
		return nil, err
	}
	fm := NewFrontmatter()
	fm.SetString(uidKey, uid)
	fm.SetString(titleKey, noteTitle(name))
	if kind != KindUnset {
		fm.SetString(kindKey, kind.String())
	}
	fm.SetString(sourceKey, source)
	fm.SetDate(dateKey, date)
	fm.SetStringList(tagsKey, tags)
	if len(people) > 0 {
		fm.SetStringList(peopleKey, people)
	}
	if len(related) > 0 {
		fm.SetStringList("related", related)
	}
	return assembleNote(fm, []byte(body), false)
}
