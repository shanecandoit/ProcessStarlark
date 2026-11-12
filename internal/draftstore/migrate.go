package draftstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"
)

//go:embed migrations/*.sql
var migrations embed.FS

func migrate(ctx context.Context, db *sql.DB, files fs.FS) error {
	// The immediate transaction serializes concurrent startup, including the
	// migration ledger. A failed migration rolls back its SQL and ledger row.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT name, checksum FROM schema_migrations ORDER BY name`)
	if err != nil {
		return err
	}
	applied := map[string]string{}
	for rows.Next() {
		var name, checksum string
		if err = rows.Scan(&name, &checksum); err != nil {
			rows.Close()
			return err
		}
		applied[name] = checksum
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	names, err := fs.Glob(files, "migrations/*.sql")
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range names {
		known[name] = true
	}
	for name := range applied {
		if !known[name] {
			return fmt.Errorf("database has unsupported migration %s; use a compatible server", name)
		}
	}
	missing := false
	for _, name := range names {
		text, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		checksum := fmt.Sprintf("%x", sha256.Sum256(text))
		if previous, ok := applied[name]; ok {
			if previous != checksum {
				return fmt.Errorf("applied migration checksum changed: %s", name)
			}
			if missing {
				return fmt.Errorf("migration history has a gap before %s", name)
			}
			continue
		}
		missing = true
		if _, err = tx.ExecContext(ctx, string(text)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?,?,?)`, name, checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
