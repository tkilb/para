package store

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"time"
)

// ErrPurgeIncomplete is returned when some expired dailies could not be
// removed. Like the other store errors it never echoes a path.
var ErrPurgeIncomplete = errors.New("some old daily notes could not be removed")

// PurgeDailies permanently deletes the archived daily notes
// (archives/_daily/YYYY-MM-DD.md) whose date is retentionDays or more calendar
// days before today, and returns how many it removed. It never touches
// another folder or file name, never follows a symlink and is not undoable.
// A retention of zero or less removes nothing, and a missing folder is not
// an error. The folder must resolve inside the notebook root.
func (m *Markdown) PurgeDailies(retentionDays int, today time.Time) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	dir, err := ResolveInRoot(m.root, path.Join(tabArchives, dailyDir))
	if err != nil {
		return 0, err
	}
	if !isPlainDir(m.root, dir) {
		return 0, ErrEscapesRoot
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, ErrUnreadable
	}

	removed, failed := 0, false
	for _, e := range entries {
		if !e.Type().IsRegular() || !isDailyName(e.Name()) {
			continue
		}
		age, ok := daysBefore(e.Name()[:len(dateLayout)], today)
		if !ok || age < retentionDays {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			failed = true
			continue
		}
		removed++
	}
	if failed {
		return removed, ErrPurgeIncomplete
	}
	return removed, nil
}

// daysBefore returns how many whole calendar days the YYYY-MM-DD date is
// before today (negative for a future date).
func daysBefore(date string, today time.Time) (int, bool) {
	day, err := time.Parse(dateLayout, date)
	if err != nil {
		return 0, false
	}
	y, m, d := today.Date()
	todayDay := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return int(todayDay.Sub(day).Hours() / hoursPerDay), true
}

// isPlainDir reports whether dir, as resolved, is still archives/_daily under
// the real notebook root and a real directory: a symlink (at archives or at
// daily) that points at another folder inside the notebook stays within the
// root but is not the folder to purge. A folder that does not exist passes.
func isPlainDir(root, dir string) bool {
	abs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	if dir != filepath.Join(real, tabArchives, dailyDir) {
		return false
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return err == nil && info.IsDir()
}
