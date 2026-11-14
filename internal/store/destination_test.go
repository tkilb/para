package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMoveDestination(t *testing.T) {
	tests := []struct {
		name, rel, tab, want string
	}{
		{"to a tab root", "inbox/a.md", "projects", "projects/a.md"},
		{"from a subfolder lands at the root", "areas/deep/er/a.md", "resources", "resources/a.md"},
		{"to the inbox", "projects/a.md", "inbox", "inbox/a.md"},
		{"to the archives", "projects/a.md", "archives", "archives/a.md"},
		{"a daily goes to archives/_daily", "inbox/_daily/2026-10-30.md", "archives", "archives/_daily/2026-10-30.md"},
		{"a daily elsewhere is plain", "inbox/_daily/2026-10-30.md", "projects", "projects/2026-10-30.md"},
		{"a daily-looking name outside inbox/_daily is plain", "projects/2026-10-30.md", "archives", "archives/2026-10-30.md"},
		{"a non-date note in inbox/_daily is plain", "inbox/_daily/notes.md", "archives", "archives/notes.md"},
		{"daily extension case", "inbox/_daily/2026-10-30.MD", "archives", "archives/_daily/2026-10-30.MD"},
		{"a bad date is not a daily", "inbox/_daily/2026-13-45.md", "archives", "archives/2026-13-45.md"},
		{"unknown tab", "inbox/a.md", "elsewhere", ""},
		{"empty", "", "projects", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MoveDestination(tt.rel, tt.tab))
		})
	}
}

func TestDailyDate(t *testing.T) {
	tests := []struct {
		rel  string
		want string
		ok   bool
	}{
		{"inbox/_daily/2026-10-30.md", "2026-10-30", true},
		{"archives/_daily/2026-10-30.md", "2026-10-30", true},
		{"inbox/_daily/2026-10-30.MD", "2026-10-30", true},
		{"inbox/_daily/notes.md", "", false},
		{"inbox/_daily/2026-13-45.md", "", false},
		{"inbox/2026-10-30.md", "", false},
		{"projects/daily/2026-10-30.md", "", false},
		{"inbox/_daily/sub/2026-10-30.md", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			got, ok := DailyDate(tt.rel)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDailyOverdue(t *testing.T) {
	today := time.Date(2026, 10, 30, 15, 45, 0, 0, time.Local)
	tests := []struct {
		name string
		rel  string
		days int
		want bool
	}{
		{"exactly the retention period old", "inbox/_daily/2026-09-30.md", 30, true},
		{"one day short", "inbox/_daily/2026-10-01.md", 30, false},
		{"older", "inbox/_daily/2026-01-15.md", 30, true},
		{"today", "inbox/_daily/2026-10-30.md", 30, false},
		{"a future date", "inbox/_daily/2026-12-01.md", 30, false},
		{"seven day retention", "inbox/_daily/2026-10-23.md", 7, true},
		{"one day retention", "inbox/_daily/2026-10-29.md", 1, true},
		{"already archived is not overdue", "archives/_daily/2025-01-01.md", 30, false},
		{"not a daily name", "inbox/_daily/notes.md", 30, false},
		{"not in the daily folder", "inbox/2025-01-01.md", 30, false},
		{"zero retention never marks", "inbox/_daily/2020-01-01.md", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DailyOverdue(tt.rel, tt.days, today))
		})
	}
	// Calendar days, not 24-hour spans: a late-evening "today" does not tip a
	// note over early.
	late := time.Date(2026, 10, 30, 23, 59, 0, 0, time.Local)
	assert.True(t, DailyOverdue("inbox/_daily/2026-09-30.md", 30, late))
	early := time.Date(2026, 10, 30, 0, 1, 0, 0, time.Local)
	assert.True(t, DailyOverdue("inbox/_daily/2026-09-30.md", 30, early))
	assert.False(t, DailyOverdue("inbox/_daily/2026-10-01.md", 30, early))
}

func TestIsUndigestedDaily(t *testing.T) {
	assert.True(t, IsUndigestedDaily("inbox/_daily/2026-10-30.md"))
	assert.False(t, IsUndigestedDaily("archives/_daily/2026-10-30.md"))
	assert.False(t, IsUndigestedDaily("inbox/_daily/notes.md"))
}
