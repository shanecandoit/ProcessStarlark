package draftstore

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func gen(n int) *int { return &n }
func TestReleaseTransactionsAcrossConnectionsAndBackup(t *testing.T) {
	ctx := context.Background()
	base := shipping(t)
	path := filepath.Join(t.TempDir(), "release.sqlite")
	a := openTest(t, path, base)
	b := openTest(t, path, base)
	d, err := a.Create(ctx, "shipping", "Shipping", "ops", "", "")
	if err != nil {
		t.Fatal(err)
	}
	request := workspace.ReleaseCommand{Action: "request", DraftID: d.ID, Author: "developer", ExpectedState: d.StateToken, ExpectedGeneration: gen(0)}
	r1, err := a.ExecuteRelease(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := b.ExecuteRelease(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, s := range []*Store{a, b} {
		wg.Go(func() {
			<-start
			id := r1.Review.ID
			if i == 1 {
				id = r2.Review.ID
			}
			_, e := s.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "approve", ID: id, Author: "reviewer", ExpectedGeneration: gen(0)})
			results <- e
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, workspace.ErrReleaseConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal(successes, conflicts)
	}
	before, err := a.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if before.Production.Generation != 1 || len(before.Activations) != 1 {
		t.Fatal(before)
	}
	manifests, err := a.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "release", ID: before.Production.ReleaseID})
	if err != nil {
		t.Fatal(err)
	}
	pinned := *manifests.Release
	// Edits after activation cannot alter the captured release or its checks.
	_, _, err = a.Apply(ctx, d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
	if err != nil {
		t.Fatal(err)
	}
	answer := workspace.RunRelease(ctx, pinned, before.Production, `Shipping.quote("DE", 800)`)
	if answer.Status != workspace.Unresolved || answer.ReleaseID != pinned.ID {
		t.Fatal(answer)
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openTest(t, path, base)
	restored, err := reopened.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "status"})
	if err != nil || !reflect.DeepEqual(before, restored) {
		t.Fatal("restart lost production", restored, err)
	}
	release, err := reopened.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "release", ID: pinned.ID})
	if err != nil || !reflect.DeepEqual(manifests, release) {
		t.Fatal("restart changed release", err)
	}
	backupPath := filepath.Join(t.TempDir(), "backup.sqlite")
	if err = reopened.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	backup := openTest(t, backupPath, base)
	recovered, err := backup.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "status"})
	if err != nil || !reflect.DeepEqual(before, recovered) {
		t.Fatal("backup lost activations", err)
	}
	reviews, err := backup.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "reviews"})
	if err != nil || len(reviews.Reviews) != 2 {
		t.Fatal("backup lost reviews", err)
	}
	run, err := backup.ExecuteRelease(ctx, workspace.ReleaseCommand{Action: "run", Call: `Shipping.quote("US", 500)`})
	if err != nil || run.Run.Output != "599" || run.Run.ReleaseID != pinned.ID {
		t.Fatal(run, err)
	}
}
