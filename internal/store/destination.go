package store

import (
	"path"
	"strings"
	"time"
)

const (
	tabInbox    = "inbox"
	tabArchives = "archives"
	dailyDir    = "_daily"
)

// tabRoots are the folders PARA shows as tabs.
var tabRoots = []string{tabInbox, "projects", "areas", "resources", tabArchives}

// isDailyName reports whether name is YYYY-MM-DD.md, ignoring the case of
// the extension.
func isDailyName(name string) bool {
	if !isNote(name) {
		return false
	}
	_, err := time.Parse(dateLayout, strings.TrimSuffix(name, name[len(name)-len(noteExt):]))
	return err == nil
}

// DailyDate returns the date of a dated daily note: a note named
// YYYY-MM-DD.md directly inside inbox/_daily or archives/_daily. It reports
// false for anything else.
func DailyDate(rel string) (string, bool) {
	dir := path.Dir(rel)
	if dir != path.Join(tabInbox, dailyDir) && dir != path.Join(tabArchives, dailyDir) {
		return "", false
	}
	name := path.Base(rel)
	if !isDailyName(name) {
		return "", false
	}
	return name[:len(name)-len(noteExt)], true
}

// IsUndigestedDaily reports whether rel is a dated daily note still in
// inbox/_daily, waiting to be digested and archived.
func IsUndigestedDaily(rel string) bool {
	_, ok := DailyDate(rel)
	return ok && path.Dir(rel) == path.Join(tabInbox, dailyDir)
}

// DailyOverdue reports whether the dated daily note at rel is still in
// inbox/_daily although its date is retentionDays or more calendar days
// before today: it should have been digested and archived by now. A
// retention of zero or less never marks anything.
func DailyOverdue(rel string, retentionDays int, today time.Time) bool {
	if retentionDays <= 0 || !IsUndigestedDaily(rel) {
		return false
	}
	date, _ := DailyDate(rel)
	age, ok := daysBefore(date, today)
	return ok && age >= retentionDays
}

const hoursPerDay = 24
