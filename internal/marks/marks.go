// Package marks keeps the marked notes of a notebook in a hidden file at its
// root, so the marks travel with the notebook (backups, git sync, a second
// machine). The file lists one note uid per line, which merges cleanly in git.
package marks

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// FileName is the marks file, at the root of the notebook. Its leading dot
	// hides it from Yazi (by default) and from Obsidian.
	FileName = ".para-marks"
	// Max is how many marked notes a notebook keeps.
	Max = 1000
	// maxUIDLen bounds a uid, so a damaged file cannot grow memory use.
	maxUIDLen = 128
	filePerm  = 0o644
)

// Load returns the uids marked in the notebook at root, in file order, and
// whether the marks file exists. Comments (#), blanks, oversized and repeated
// uids are dropped and at most Max are kept.
func Load(root string) (uids []string, exists bool, err error) {
	f, err := os.Open(filepath.Join(root, FileName))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load marks: %w", err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, true, fmt.Errorf("load marks: %w", err)
	}
	return clean(lines), true, nil
}

// Save replaces the marks file of the notebook at root, all or nothing.
func Save(root string, uids []string) error {
	var b strings.Builder
	for _, uid := range clean(uids) {
		b.WriteString(uid)
		b.WriteByte('\n')
	}
	tmp, err := os.CreateTemp(root, FileName+".tmp*")
	if err != nil {
		return fmt.Errorf("save marks: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("save marks: %w", err)
	}
	if err := tmp.Chmod(filePerm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("save marks: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("save marks: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(root, FileName)); err != nil {
		return fmt.Errorf("save marks: %w", err)
	}
	return nil
}

// clean drops blank, oversized, repeated and whitespace-bearing uids, keeps
// the order and stops at Max.
func clean(uids []string) []string {
	seen := make(map[string]bool, len(uids))
	var out []string
	for _, uid := range uids {
		if uid == "" || len(uid) > maxUIDLen || strings.ContainsAny(uid, " \t\r\n") || seen[uid] {
			continue
		}
		seen[uid] = true
		if out = append(out, uid); len(out) == Max {
			break
		}
	}
	return out
}
