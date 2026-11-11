package workspace

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestDraftSnapshotIsolationAndQuestionHistory(t *testing.T) {
	draft, _ := newTestDraft(t)
	_, snapshot, err := draft.Run(context.Background(), "ops", `Shipping.quote("DE", 800)`)
	if err != nil || len(snapshot.Snapshot.Questions) != 1 {
		t.Fatal(err, snapshot)
	}
	current := draft.GetSnapshot(true)
	if len(current.Snapshot.Questions) != 1 || current.Snapshot.Catalog == "" {
		t.Fatal("snapshot lost the question or catalog")
	}
	current.Files["zones.star"] = "invalid source"
	current.History[0].Author = "tampered"
	current.History[0].FilesChanged[0] = "tampered.star"
	snapshot.Files["zones.star"] = "also invalid"
	fresh := draft.GetSnapshot(true)
	if fresh.History[0].Author != "ops" || fresh.History[0].FilesChanged[0] != "questions.txt" || !fresh.Snapshot.Valid {
		t.Fatal("snapshot aliases internal state", fresh)
	}
	if history := draft.GetSnapshot(false).History; history != nil {
		t.Fatal("history was requested to be omitted", history)
	}
	if _, _, err := draft.AddExample("ops", `Zones.zone("DE") == "international"`); err != nil {
		t.Fatal(err)
	}
	if snapshot := draft.GetSnapshot(true); len(snapshot.Snapshot.Questions) != 0 {
		t.Fatal("answered question survived", snapshot)
	}
}

func TestDraftConcurrentOperationsDoNotLoseEdits(t *testing.T) {
	draft, _ := newTestDraft(t)
	const count = 16
	start := make(chan struct{})
	errors := make(chan error, count*2)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Go(func() {
			<-start
			result, _, err := draft.AddExample("ops", fmt.Sprintf(`Zones.zone("X%d") == "international"`, i))
			if err != nil {
				errors <- err
			} else if result.Status != OK {
				errors <- fmt.Errorf("%s", result.Message)
			}
		})
		wg.Go(func() {
			<-start
			_, _, err := draft.Run(context.Background(), "dev", `Shipping.quote("US", 500)`)
			if err != nil {
				errors <- err
			}
		})
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	snapshot := draft.GetSnapshot(true)
	if snapshot.Version != count*2 || len(snapshot.History) != count*2 {
		t.Fatal(snapshot.Version, len(snapshot.History))
	}
	for i := 0; i < count; i++ {
		if !strings.Contains(snapshot.Files["zones.star"], fmt.Sprintf(`zone("X%d")`, i)) {
			t.Errorf("lost X%d", i)
		}
	}
}

func TestDraftOptimisticConflictAndInvalidEditAreAtomic(t *testing.T) {
	draft, _ := newTestDraft(t)
	ctx := context.Background()
	_, snapshot, err := draft.Apply(ctx, "ops", 0, DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
	if err != nil {
		t.Fatal(err)
	}
	_, conflict, err := draft.Apply(ctx, "dev", 0, DraftOperation{Kind: "remove-example", Call: `Zones.zone("US")`})
	if !errors.Is(err, ErrDraftConflict) || conflict.Version != 1 {
		t.Fatal("missing conflict", err, conflict.Version)
	}
	result, invalid, err := draft.Apply(ctx, "dev", 1, DraftOperation{Kind: "files", Files: Files{"zones.star": "invalid source"}})
	if err != nil || result.Status != Error || invalid.Version != 1 {
		t.Fatal(err, result.Status, invalid.Version)
	}
	if !reflect.DeepEqual(snapshot.Files, draft.GetSnapshot(false).Files) {
		t.Fatal("rejected operation changed source")
	}
}

func TestDraftDiffReplacementsDeletionsAndFullHashes(t *testing.T) {
	_, base := newTestDraft(t)
	base["extra.star"] = "Process Extra:\n    example: name(1) == \"one\"\n    example: name(2) == \"two\"\n    def name(value):\n        pass\n"
	draft, err := NewDraftWorkspace("diff", "Diff", "ops", base)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(base["zones.star"], `zone("CA") == "north-america"`, `zone("CA") == "international"`, 1)
	result, _, err := draft.Apply(context.Background(), "ops", 0, DraftOperation{Kind: "files", Files: Files{"zones.star": edited}, Delete: []string{"extra.star"}})
	if err != nil || result.Status != OK {
		t.Fatal(err, result.Message)
	}
	diff, err := draft.Diff(base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(diff.FilesChanged, []string{"zones.star"}) || !reflect.DeepEqual(diff.FilesDeleted, []string{"extra.star"}) {
		t.Fatal(diff)
	}
	var zones, extra *FunctionChange
	for i := range diff.FunctionChanges {
		change := &diff.FunctionChanges[i]
		if len(change.OldHash) != 64 {
			t.Error("hash was truncated", change)
		}
		if change.Qualified == "Zones.zone" {
			zones = change
		}
		if change.Qualified == "Extra.name" {
			extra = change
		}
	}
	if zones == nil || zones.ExamplesAdded != 1 || zones.ExamplesRemoved != 1 || zones.BodyChanged || len(zones.NewHash) != 64 {
		t.Fatal("replacement was not represented", zones)
	}
	if extra == nil || extra.ExamplesRemoved != 2 || extra.NewHash != "" || !extra.BodyChanged {
		t.Fatal("deletion was not represented", extra)
	}
	for i := 0; i < 3; i++ {
		again, err := draft.Diff(base)
		if err != nil || !reflect.DeepEqual(diff, again) {
			t.Fatal("nondeterministic diff", err)
		}
	}
}

func TestDraftRunHistoryRecordsOnlyActualFileChanges(t *testing.T) {
	draft, _ := newTestDraft(t)
	_, snapshot, err := draft.Run(context.Background(), "ops", `Shipping.quote("US", 500)`)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.History[0].FilesChanged) != 0 {
		t.Fatal("read-only run claims source edits", snapshot.History)
	}
	if snapshot.Version != 1 {
		t.Fatal("version counts accepted operations", snapshot.Version)
	}
}
