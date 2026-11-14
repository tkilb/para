package state

import "fmt"

// MaxMarks is how many marked notes a notebook keeps.
const MaxMarks = 1000

// LoadMarks returns the uids of the notes marked in the notebook at path.
// Blank or oversized values are dropped. The order is not meaningful.
func (d *DB) LoadMarks(path string) ([]string, error) {
	rows, err := d.sql.Query(`SELECT uid FROM mark WHERE notebook = ?`, path)
	if err != nil {
		return nil, fmt.Errorf("load marks: %w", err)
	}
	defer rows.Close()
	var uids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("load marks: %w", err)
		}
		uids = append(uids, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load marks: %w", err)
	}
	return cleanUIDs(uids, MaxMarks), nil
}

// SaveMarks replaces the marks of the notebook at path, all or nothing.
// Blank, oversized and repeated uids are dropped and at most MaxMarks are
// kept.
func (d *DB) SaveMarks(path string, uids []string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("save marks: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM mark WHERE notebook = ?`, path); err != nil {
		return fmt.Errorf("save marks: %w", err)
	}
	for _, uid := range cleanUIDs(uids, MaxMarks) {
		if _, err := tx.Exec(`INSERT INTO mark (notebook, uid) VALUES (?, ?)`, path, uid); err != nil {
			return fmt.Errorf("save marks: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save marks: %w", err)
	}
	return nil
}
