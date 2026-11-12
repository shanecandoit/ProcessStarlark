package draftstore

import (
	"context"
	"os"
	"path/filepath"
)

// Backup produces a consistent standalone database, including committed WAL
// content. Reserve a new output file so existing user files are never replaced.
func (s *Store) Backup(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(abs)
		return err
	}
	if _, err = s.db.ExecContext(ctx, `VACUUM INTO ?`, abs); err != nil {
		os.Remove(abs)
		return err
	}
	return nil
}
