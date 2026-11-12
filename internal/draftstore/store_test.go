package draftstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func shipping(t *testing.T) workspace.Files {
	t.Helper()
	out := workspace.Files{}
	paths, err := filepath.Glob("../../examples/shipping/*.star")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(p)] = string(b)
	}
	return out
}
func openTest(t *testing.T, path string, base workspace.Files) *Store {
	t.Helper()
	s, err := Open(path, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func applyTest(t *testing.T, s *Store, snap workspace.DraftSnapshot, author string, op workspace.DraftOperation) (workspace.Result, workspace.DraftSnapshot) {
	t.Helper()
	r, next, err := s.Apply(context.Background(), snap.ID, author, snap.StateToken, nil, op)
	if err != nil || r.Status == workspace.Error {
		t.Fatalf("%s: %v %s", op.Kind, err, r.Message)
	}
	return r, next
}

func TestPersistentDraftQueueFlowRestartForkAndBackup(t *testing.T) {
	ctx := context.Background()
	base := shipping(t)
	path := filepath.Join(t.TempDir(), "drafts.sqlite")
	s := openTest(t, path, base)
	initial, err := s.Create(ctx, "shipping", "Germany", "ops", "", "")
	if err != nil {
		t.Fatal(err)
	}
	r, asked := applyTest(t, s, initial, "ops", workspace.DraftOperation{Kind: "run", Call: `Shipping.quote("DE", 800)`})
	if r.Status != workspace.Unresolved || asked.Revision != initial.Revision {
		t.Fatal(r.Status, asked)
	}
	draft, err := s.Read(ctx, asked.ID)
	if err != nil {
		t.Fatal(err)
	}
	queue := draft.Questions(false)
	if len(queue) != 1 || queue[0].OpenedBy != workspace.LocalUserID("ops") || queue[0].OpenedAt.IsZero() {
		t.Fatal(queue)
	}
	questionID := queue[0].ID
	opened := queue[0].OpenedAt
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTest(t, path, base)
	draft, err = s.Read(ctx, asked.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored := draft.GetSnapshot(true)
	if !reflect.DeepEqual(asked, restored) || !reflect.DeepEqual(queue, draft.Questions(false)) {
		t.Fatal("restart changed source, dates, activity, token, or questions")
	}
	_, again := applyTest(t, s, restored, "ops", workspace.DraftOperation{Kind: "run", Call: `Shipping.quote(country="DE", grams=800)`})
	draft, _ = s.Read(ctx, asked.ID)
	if len(draft.Questions(false)) != 1 {
		t.Fatal("repeat question duplicated")
	}
	_, dismissed := applyTest(t, s, again, "reviewer", workspace.DraftOperation{Kind: "dismiss", Call: queue[0].Call})
	draft, _ = s.Read(ctx, asked.ID)
	closed := draft.Questions(true)
	if len(draft.Questions(false)) != 0 || closed[0].Status != "dismissed" || closed[0].ClosedBy != workspace.LocalUserID("reviewer") {
		t.Fatal(closed)
	}
	_, reopened := applyTest(t, s, dismissed, "ops", workspace.DraftOperation{Kind: "run", Call: `Shipping.quote("DE", 800)`})
	draft, _ = s.Read(ctx, asked.ID)
	queue = draft.Questions(false)
	if len(queue) != 1 || queue[0].ID != questionID || !queue[0].OpenedAt.Equal(opened) || queue[0].ClosedAt != nil {
		t.Fatal(queue)
	}
	_, saved := applyTest(t, s, reopened, "developer", workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
	draft, _ = s.Read(ctx, asked.ID)
	closed = draft.Questions(true)
	if saved.Revision == initial.Revision || len(draft.Questions(false)) != 0 || closed[0].Status != "resolved" || closed[0].ClosedByName != "developer" {
		t.Fatal(closed)
	}
	versions := draft.Versions()
	if len(versions) != 2 || versions[0].Parent != initial.Revision || strings.Contains(versions[1].Files["zones.star"], `zone("DE")`) {
		t.Fatal(versions)
	}
	r, ran := applyTest(t, s, saved, "ops", workspace.DraftOperation{Kind: "run", Call: `Shipping.quote("DE", 800)`})
	if r.Output != "2799" || r.Status != workspace.OK {
		t.Fatal(r)
	}
	r, checked := applyTest(t, s, ran, "developer", workspace.DraftOperation{Kind: "test"})
	if r.Status != workspace.OK || checked.Revision != saved.Revision {
		t.Fatal(r)
	}
	_, _, err = s.Apply(ctx, asked.ID, "other", saved.StateToken, nil, workspace.DraftOperation{Kind: "load"})
	if !errors.Is(err, workspace.ErrDraftConflict) {
		t.Fatal("stale token accepted", err)
	}
	_, formatted := applyTest(t, s, checked, "developer", workspace.DraftOperation{Kind: "files", Files: workspace.Files{"zones.star": "\n\n\n" + checked.Files["zones.star"] + "\n\n"}})
	if formatted.Revision != checked.Revision || formatted.Files["zones.star"] != checked.Files["zones.star"] {
		t.Fatal("formatting-only edit made a distinct source version")
	}
	branch, err := s.Create(ctx, "branch", "Earlier rule", "reviewer", asked.ID, initial.Revision)
	if err != nil || branch.Revision != initial.Revision {
		t.Fatal(branch, err)
	}
	if _, _, err = s.Apply(ctx, branch.ID, "reviewer", formatted.StateToken, nil, workspace.DraftOperation{Kind: "load"}); !errors.Is(err, workspace.ErrDraftConflict) {
		t.Fatal("cross-draft token accepted", err)
	}
	backup := filepath.Join(t.TempDir(), "backup.sqlite")
	if err = s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err = s.Backup(ctx, backup); err == nil {
		t.Fatal("backup overwrote an existing file")
	}
	copy := openTest(t, backup, base)
	draft, err = copy.Read(ctx, asked.ID)
	if err != nil || !reflect.DeepEqual(formatted, draft.GetSnapshot(true)) {
		t.Fatal("backup did not restore exact draft", err)
	}
	var users int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil || users != 3 {
		t.Fatal(users, err)
	}
}

func TestIndependentConnectionsRejectLostWrites(t *testing.T) {
	base := shipping(t)
	path := filepath.Join(t.TempDir(), "db.sqlite")
	a := openTest(t, path, base)
	b := openTest(t, path, base)
	snap, err := a.Create(context.Background(), "shared", "Shared", "ops", "", "")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, s := range []*Store{a, b} {
		wg.Go(func() {
			<-start
			country := []string{"DE", "JP"}[i]
			_, _, err := s.Apply(context.Background(), snap.ID, "ops", snap.StateToken, nil, workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("` + country + `") == "international"`})
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, workspace.ErrDraftConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	draft, err := b.Read(context.Background(), snap.ID)
	if err != nil || ok != 1 || conflicts != 1 || len(draft.GetSnapshot(true).History) != 1 || len(draft.Versions()) != 2 {
		t.Fatal(ok, conflicts, err)
	}
}

func TestDatabaseFailureRollsBackAllObjectsAndActivity(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "db.sqlite"), shipping(t))
	ctx := context.Background()
	snap, err := s.Create(ctx, "atomic", "Atomic", "ops", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`CREATE TRIGGER fail_activity BEFORE INSERT ON activity WHEN NEW.sequence>0 BEGIN SELECT RAISE(ABORT,'injected write failure'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.Apply(ctx, snap.ID, "new-user", snap.StateToken, nil, workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
	if err == nil {
		t.Fatal("injected failure ignored")
	}
	draft, err := s.Read(ctx, snap.ID)
	if err != nil || !reflect.DeepEqual(snap, draft.GetSnapshot(true)) {
		t.Fatal("failed commit changed working state", err)
	}
	for table, want := range map[string]int{"users": 1, "source_versions": 1, "source_snapshots": 1, "activity": 1, "questions": 0} {
		var count int
		if err = s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatal(table, count, want, err)
		}
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_activity`); err != nil {
		t.Fatal(err)
	}
	applyTest(t, s, snap, "new-user", workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
}

func TestReferenceMismatchAndInvalidEdit(t *testing.T) {
	base := shipping(t)
	path := filepath.Join(t.TempDir(), "db.sqlite")
	s := openTest(t, path, base)
	snap, err := s.Create(context.Background(), "safe", "Safe", "ops", "", "")
	if err != nil {
		t.Fatal(err)
	}
	r, after, err := s.Apply(context.Background(), snap.ID, "ops", snap.StateToken, nil, workspace.DraftOperation{Kind: "files", Files: workspace.Files{"zones.star": "invalid"}})
	if err != nil || r.Status != workspace.Error || !reflect.DeepEqual(snap, after) {
		t.Fatal("invalid edit changed draft", err, r.Status)
	}
	base["zones.star"] += "\n# a different reference\n"
	if wrong, err := Open(path, base); err == nil {
		wrong.Close()
		t.Fatal("reference silently changed")
	}
}
