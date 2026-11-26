package skills

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tkilb/para/internal/store"
)

const (
	// DirName is the hidden folder where skills are stored in a notebook.
	DirName = ".skills"
	ext     = ".md"
	maxSize = 1 << 20 // 1 MB limit per skill file
)

var (
	ErrNotFound   = errors.New("skill not found")
	ErrTooLarge   = errors.New("skill file exceeds size limit")
	ErrNotRegular = errors.New("skill file is not a regular file")
)

// CleanName validates a user-supplied skill name or filename. It rejects empty
// names, directories (trailing or embedded "/"), ".", "..", and NUL bytes, and
// returns the canonical ".md" filename and its stem.
func CleanName(input string) (filename, stem string, err error) {
	name, isDir, err := store.CleanNewName(input)
	if err != nil {
		return "", "", err
	}
	if isDir {
		return "", "", store.ErrBadName
	}
	return name, stemOf(name), nil
}

// RelPath returns the notebook-relative path (.skills/<filename>) for a
// validated skill name.
func RelPath(input string) (string, error) {
	filename, _, err := CleanName(input)
	if err != nil {
		return "", err
	}
	return filepath.Join(DirName, filename), nil
}

// Dir returns the .skills path inside notebookRoot.
func Dir(notebookRoot string) string {
	return filepath.Join(notebookRoot, DirName)
}

// List returns all skills in notebookRoot/.skills, sorted case-insensitively
// by name (with filename as tie-breaker). A skill file with an empty body is
// still listed so it can be edited or deleted in the UI.
func List(notebookRoot string) ([]Skill, error) {
	dir, err := store.ResolveInRoot(notebookRoot, DirName)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skills directory: %w", err)
	}

	var skills []Skill
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(strings.ToLower(name), ext) {
			continue
		}
		path, err := store.ResolveInRoot(notebookRoot, filepath.Join(DirName, name))
		if err != nil {
			continue
		}
		s, err := readSkill(path, name)
		if err != nil && !errors.Is(err, ErrEmptyBody) {
			continue
		}
		skills = append(skills, s)
	}

	sort.SliceStable(skills, func(i, j int) bool {
		li, lj := strings.ToLower(skills[i].Name), strings.ToLower(skills[j].Name)
		if li != lj {
			return li < lj
		}
		return skills[i].Filename < skills[j].Filename
	})

	return skills, nil
}

// Load loads and validates a single skill by filename (or name) from
// notebookRoot/.skills.
func Load(notebookRoot, filename string) (Skill, error) {
	clean, _, err := CleanName(filename)
	if err != nil {
		return Skill{}, err
	}
	path, err := store.ResolveInRoot(notebookRoot, filepath.Join(DirName, clean))
	if err != nil {
		return Skill{}, err
	}
	return readSkill(path, clean)
}

func readSkill(path, filename string) (Skill, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Skill{}, fmt.Errorf("%w: %s", ErrNotFound, filename)
	}
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Skill{}, err
	}
	if !info.Mode().IsRegular() {
		return Skill{}, ErrNotRegular
	}
	if info.Size() > maxSize {
		return Skill{}, ErrTooLarge
	}

	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return Skill{}, fmt.Errorf("read skill file: %w", err)
	}
	if len(data) > maxSize {
		return Skill{}, ErrTooLarge
	}

	s, parseErr := Parse(filename, data)
	s.Path = path
	s.Rel = filepath.Join(DirName, filename)
	if parseErr != nil {
		return s, parseErr
	}
	return s, nil
}

// Save saves a skill to notebookRoot/.skills, creating the directory if needed.
// Returns the full path of the saved file.
func Save(notebookRoot string, s Skill) (string, error) {
	rawName := s.Filename
	if rawName == "" {
		rawName = s.Name
	}
	filename, _, err := CleanName(rawName)
	if err != nil {
		return "", err
	}

	dir, err := store.ResolveInRoot(notebookRoot, DirName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create skills dir: %w", err)
	}

	path, err := store.ResolveInRoot(notebookRoot, filepath.Join(DirName, filename))
	if err != nil {
		return "", err
	}
	s.Filename = filename
	data := Format(s)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write skill file: %w", err)
	}

	return path, nil
}

// Delete removes a skill file from notebookRoot/.skills.
func Delete(notebookRoot, filename string) error {
	clean, _, err := CleanName(filename)
	if err != nil {
		return err
	}
	path, err := store.ResolveInRoot(notebookRoot, filepath.Join(DirName, clean))
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, clean)
		}
		return fmt.Errorf("delete skill file: %w", err)
	}
	return nil
}
