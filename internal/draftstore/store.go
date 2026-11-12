// Package draftstore persists the local draft service. The SQLite dependency
// stays outside workspace so it is never linked into the browser's WASM engine.
package draftstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
	_ "modernc.org/sqlite"
)

type Store struct {
	db        *sql.DB
	reference workspace.Files
}

const timeFormat = time.RFC3339Nano

func Open(path string, reference workspace.Files) (*Store, error) {
	base := workspace.Format(reference)
	if base.Status != workspace.OK {
		return nil, fmt.Errorf("reference: %s", base.Message)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = retryBusy(ctx, func() error { _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); return err }); err != nil {
		return nil, err
	}
	if err = retryBusy(ctx, func() error { return migrate(ctx, db, migrations) }); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var hash, files string
	err = tx.QueryRowContext(ctx, `SELECT source_hash, files_json FROM reference_project WHERE id=1`).Scan(&hash, &files)
	if errors.Is(err, sql.ErrNoRows) {
		files = encode(base.Files)
		_, err = tx.ExecContext(ctx, `INSERT INTO reference_project VALUES (1,?,?,?)`, base.Snapshot.Revision, files, stamp())
	} else if err == nil && hash != base.Snapshot.Revision {
		return nil, fmt.Errorf("database reference differs from --root; use the original reference or a new database")
	}
	if err != nil {
		return nil, err
	}
	var stored workspace.Files
	if err = json.Unmarshal([]byte(files), &stored); err != nil {
		return nil, err
	}
	if check := workspace.Load(stored); check.Status != workspace.OK || check.Snapshot.Revision != base.Snapshot.Revision {
		return nil, fmt.Errorf("stored reference is invalid")
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	ok = true
	return &Store{db: db, reference: stored}, nil
}

func retryBusy(ctx context.Context, operation func() error) error {
	for {
		err := operation()
		if err == nil {
			return nil
		}
		var coded interface{ Code() int }
		if !errors.As(err, &coded) || (coded.Code()&255 != 5 && coded.Code()&255 != 6) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Kind() string { return "sqlite" }
func (s *Store) Reference() workspace.Files {
	out := workspace.Files{}
	for name, text := range s.reference {
		out[name] = text
	}
	return out
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func encode(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func readDraft(ctx context.Context, tx *sql.Tx, id string) (*workspace.DraftWorkspace, error) {
	var text string
	if err := tx.QueryRowContext(ctx, `SELECT state_json FROM drafts WHERE id=?`, id).Scan(&text); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workspace.ErrDraftNotFound
		}
		return nil, err
	}
	var record workspace.DraftRecord
	if err := json.Unmarshal([]byte(text), &record.Draft); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT v.id, COALESCE(v.parent_id,''), v.source_hash, s.files_json FROM draft_versions d JOIN source_versions v ON v.id=d.version_id JOIN source_snapshots s ON s.source_hash=v.source_hash WHERE d.draft_id=?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v workspace.SourceVersion
		var files string
		if err = rows.Scan(&v.ID, &v.Parent, &v.SourceRevision, &files); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(files), &v.Files); err != nil {
			rows.Close()
			return nil, err
		}
		record.Versions = append(record.Versions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT entry_json FROM activity WHERE draft_id=? AND sequence>0 ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var entry workspace.DraftEntry
		if err = rows.Scan(&text); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(text), &entry); err != nil {
			rows.Close()
			return nil, err
		}
		record.Draft.History = append(record.Draft.History, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	record.Questions = []workspace.QuestionRecord{}
	rows, err = tx.QueryContext(ctx, `SELECT question_json FROM questions WHERE draft_id=? ORDER BY opened_at,id`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var q workspace.QuestionRecord
		if err = rows.Scan(&text); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(text), &q); err != nil {
			rows.Close()
			return nil, err
		}
		record.Questions = append(record.Questions, q)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return workspace.RestoreDraft(record)
}

// Read returns an isolated workspace for inspection; use Apply to persist edits.
func (s *Store) Read(ctx context.Context, id string) (*workspace.DraftWorkspace, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	draft, err := readDraft(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return draft, nil
}

func (s *Store) List(ctx context.Context) ([]workspace.DraftSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM drafts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []workspace.DraftSnapshot{}
	for _, id := range ids {
		d, err := readDraft(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d.GetSnapshot(false))
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) Create(ctx context.Context, id, name, author, from, revision string) (workspace.DraftSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workspace.DraftSnapshot{}, err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM drafts WHERE id=?`, id).Scan(&count); err != nil {
		return workspace.DraftSnapshot{}, err
	}
	if count != 0 {
		return workspace.DraftSnapshot{}, workspace.ErrDraftExists
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM drafts`).Scan(&count); err != nil {
		return workspace.DraftSnapshot{}, err
	}
	if count >= 16 {
		return workspace.DraftSnapshot{}, workspace.ErrDraftLimit
	}
	var draft *workspace.DraftWorkspace
	if from == "" {
		draft, err = workspace.NewDraftWorkspace(id, name, author, s.reference)
	} else {
		var parent *workspace.DraftWorkspace
		parent, err = readDraft(ctx, tx, from)
		if err == nil {
			draft, err = parent.ForkAt(id, name, author, revision)
		}
	}
	if err != nil {
		return workspace.DraftSnapshot{}, err
	}
	if err = saveDraft(ctx, tx, draft, "", author); err != nil {
		return workspace.DraftSnapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return workspace.DraftSnapshot{}, err
	}
	return draft.GetSnapshot(true), nil
}

func (s *Store) Apply(ctx context.Context, id, author, expected string, legacy *int, op workspace.DraftOperation) (workspace.Result, workspace.DraftSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workspace.Result{}, workspace.DraftSnapshot{}, err
	}
	defer tx.Rollback()
	draft, err := readDraft(ctx, tx, id)
	if err != nil {
		return workspace.Result{}, workspace.DraftSnapshot{}, err
	}
	before := draft.GetSnapshot(false)
	var result workspace.Result
	var snap workspace.DraftSnapshot
	if legacy != nil {
		result, snap, err = draft.Apply(ctx, author, *legacy, op)
	} else {
		result, snap, err = draft.ApplyState(ctx, author, expected, op)
	}
	if err != nil || result.Status == workspace.Error {
		return result, snap, err
	}
	if err = saveDraft(ctx, tx, draft, before.StateToken, author); err != nil {
		return workspace.Result{}, before, err
	}
	if err = tx.Commit(); err != nil {
		return workspace.Result{}, before, err
	}
	return result, snap, nil
}

func user(ctx context.Context, tx *sql.Tx, label, now string) (string, error) {
	id := workspace.LocalUserID(label)
	_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO users VALUES (?,?,?,?)`, id, label, now, now)
	return id, err
}

func saveDraft(ctx context.Context, tx *sql.Tx, draft *workspace.DraftWorkspace, expected, author string) error {
	r := draft.Record()
	snap := r.Draft
	now := snap.UpdatedAt.Format(time.RFC3339Nano)
	actor, err := user(ctx, tx, author, now)
	if err != nil {
		return err
	}
	creator, err := user(ctx, tx, snap.CreatedBy, snap.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	for i := len(r.Versions) - 1; i >= 0; i-- {
		v := r.Versions[i]
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_snapshots VALUES (?,?,?)`, v.SourceRevision, encode(v.Files), now); err != nil {
			return err
		}
		var parent any
		if v.Parent != "" {
			parent = v.Parent
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_versions VALUES (?,?,?,?)`, v.ID, parent, v.SourceRevision, now); err != nil {
			return err
		}
	}
	snap.Snapshot = workspace.Snapshot{}
	snap.History = nil
	if expected == "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO drafts VALUES (?,?,?,?,?,?,?)`, snap.ID, snap.Revision, snap.StateToken, encode(snap), creator, snap.CreatedAt.Format(time.RFC3339Nano), now)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE drafts SET head_id=?,state_token=?,state_json=?,updated_at=? WHERE id=? AND state_token=?`, snap.Revision, snap.StateToken, encode(snap), now, snap.ID, expected)
		if err == nil {
			n, e := result.RowsAffected()
			if e != nil {
				return e
			}
			if n != 1 {
				return workspace.ErrDraftConflict
			}
		}
	}
	if err != nil {
		return err
	}
	for _, v := range r.Versions {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO draft_versions VALUES (?,?)`, snap.ID, v.ID); err != nil {
			return err
		}
	}
	if expected == "" {
		entry := workspace.DraftEntry{Timestamp: snap.CreatedAt, Author: author, AuthorID: workspace.LocalUserID(author), Message: "Created draft", Revision: snap.Revision, FilesChanged: []string{}}
		if _, err = tx.ExecContext(ctx, `INSERT INTO activity VALUES (?,?,?,?,?,?)`, snap.ID, 0, actor, now, snap.Revision, encode(entry)); err != nil {
			return err
		}
	}
	for _, entry := range r.Draft.History {
		uid, err := user(ctx, tx, entry.Author, entry.Timestamp.Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO activity VALUES (?,?,?,?,?,?)`, snap.ID, entry.Version, uid, entry.Timestamp.Format(time.RFC3339Nano), entry.Revision, encode(entry)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET last_seen_at=? WHERE id=?`, now, actor); err != nil {
		return err
	}
	return syncQuestions(ctx, tx, snap.ID, r.Questions)
}
