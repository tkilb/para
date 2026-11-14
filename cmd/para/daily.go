package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tkilb/para/internal/intake"
	"github.com/tkilb/para/internal/store"
	"github.com/tkilb/para/internal/templates"
	"github.com/tkilb/para/internal/ui/popup"
)

// maxTemplateBytes bounds a template file, as a note is bounded.
const maxTemplateBytes = 1 << 20

// loadDailyTemplate reads the open notebook's daily template (see
// templates.DailyPath). It returns nil and no error when there is none.
func (m model) loadDailyTemplate() ([]byte, error) {
	path := templates.DailyPath(m.templatesDir, m.notebookName())
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxTemplateBytes {
		return nil, errors.New("not a regular file, or too large")
	}
	return os.ReadFile(path)
}

// openDaily jumps PARA to the daily note offset days from today (-1
// yesterday, 0 today, 1 tomorrow) and focuses PARA. A day with no note gets
// one in inbox/_daily, made from the daily template when the config names one
// and empty otherwise, which undo removes. A template that cannot be used is
// reported, and an empty note is made instead.
func (m model) openDaily(offset int) model {
	day := time.Now().AddDate(0, 0, offset)
	tmpl, templateErr := m.loadDailyTemplate()
	res, err := intake.DailyFrom(m.store, day, tmpl)
	if errors.Is(err, store.ErrFrontmatterInvalid) {
		templateErr = errors.New("its properties cannot be read")
		res, err = intake.Daily(m.store, day)
	}
	if templateErr != nil && err == nil && !res.Existed {
		m = m.notify(popup.Warning, capitalize(fmt.Sprintf("the daily template could not be used (%s), so the note is empty.", templateErr)))
	}
	if err != nil {
		return m.notify(popup.Error, capitalize(fmt.Sprintf("could not open the daily note: %s.", err)))
	}
	if !res.Existed {
		m.history.Push(res.Action)
	}
	m.para = m.para.Reload()
	para, ok := m.para.Reveal(res.Rel)
	if !ok {
		return m.notify(popup.Warning, "That note is outside the PARA folders.")
	}
	m.para, m.focus = para, panelPARA
	return m.refreshLinks()
}
