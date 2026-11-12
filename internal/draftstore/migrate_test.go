package draftstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func migrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "db.sqlite")+"?_txlock=immediate&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrationUpgradeChecksumsAndUnknownSchema(t *testing.T) {
	db := migrationDB(t)
	ctx := context.Background()
	first := fstest.MapFS{"migrations/001.sql": {Data: []byte("CREATE TABLE example (value TEXT); INSERT INTO example VALUES ('kept');")}}
	if err := migrate(ctx, db, first); err != nil {
		t.Fatal(err)
	}
	upgraded := fstest.MapFS{"migrations/001.sql": first["migrations/001.sql"], "migrations/002.sql": {Data: []byte("ALTER TABLE example ADD COLUMN note TEXT NOT NULL DEFAULT 'new';")}}
	if err := migrate(ctx, db, upgraded); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, upgraded); err != nil {
		t.Fatal("repeated startup failed", err)
	}
	var value, note string
	if err := db.QueryRow(`SELECT value,note FROM example`).Scan(&value, &note); err != nil || value != "kept" || note != "new" {
		t.Fatal(value, note, err)
	}
	if err := migrate(ctx, db, first); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatal("older binary accepted new schema", err)
	}
	upgraded["migrations/001.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE changed (x);")}
	if err := migrate(ctx, db, upgraded); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatal("altered migration accepted", err)
	}
}

func TestMigrationFailureRollsBackAndConcurrentStartup(t *testing.T) {
	db := migrationDB(t)
	ctx := context.Background()
	bad := fstest.MapFS{"migrations/001.sql": {Data: []byte("CREATE TABLE partial (x); INVALID SQL;")}}
	if err := migrate(ctx, db, bad); err == nil {
		t.Fatal("invalid migration accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('partial','schema_migrations')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial migration committed", count, err)
	}
	path := filepath.Join(t.TempDir(), "concurrent.sqlite")
	base := shipping(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		wg.Go(func() {
			<-start
			s, err := Open(path, base)
			if err == nil {
				s.Close()
			}
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent startup failed", err)
		}
	}
}
