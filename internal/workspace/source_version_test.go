package workspace

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSourceVersionIdentityAndAncestry(t *testing.T) {
	draft, base := newTestDraft(t)
	root := draft.GetSnapshot(false)
	other, err := NewDraftWorkspace("other", "Other", "dev", Files{"zones.star": base["zones.star"], "shipping.star": base["shipping.star"]})
	if err != nil {
		t.Fatal(err)
	}
	if root.Revision != other.GetSnapshot(false).Revision || root.StateToken == other.GetSnapshot(false).StateToken || len(root.Revision) != 64 {
		t.Fatal("source versions must be deterministic, state tokens draft-scoped")
	}
	recreated, err := NewDraftWorkspace(root.ID, root.Name, "dev", base)
	if err != nil || recreated.GetSnapshot(false).StateToken == root.StateToken || recreated.GetSnapshot(false).Revision != root.Revision {
		t.Fatal("a recreated draft must not accept an earlier instance's state token", err)
	}
	ctx := context.Background()
	_, ran, err := draft.ApplyState(ctx, "ops", root.StateToken, DraftOperation{Kind: "run", Call: `Shipping.quote("DE", 800)`})
	if err != nil || ran.Revision != root.Revision || ran.StateToken == root.StateToken || len(ran.Snapshot.Questions) != 1 || len(draft.Versions()) != 1 {
		t.Fatal("questions must change working state, not source history", err, ran)
	}
	_, saved, err := draft.ApplyState(ctx, "ops", ran.StateToken, DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
	if err != nil || saved.Revision == root.Revision || saved.SourceRevision == root.SourceRevision {
		t.Fatal(err, saved)
	}
	v, ok := draft.SourceVersion(saved.Revision)
	if !ok || v.Parent != root.Revision || v.SourceRevision != saved.SourceRevision || len(v.Files) != 2 {
		t.Fatal(v)
	}
	v.Files["zones.star"] = "tampered"
	versions := draft.Versions()
	versions[1].Files["zones.star"] = "also tampered"
	initial, _ := draft.SourceVersion(root.Revision)
	if initial.Parent != "" || initial.Files["zones.star"] != base["zones.star"] {
		t.Fatal("old source changed or aliases a returned snapshot", initial)
	}
	_, checked, err := draft.ApplyState(ctx, "dev", saved.StateToken, DraftOperation{Kind: "test"})
	if err != nil || checked.Revision != saved.Revision || checked.StateToken == saved.StateToken || len(draft.Versions()) != 2 {
		t.Fatal("checking must not create a source version", err)
	}
	_, reverted, err := draft.ApplyState(ctx, "dev", checked.StateToken, DraftOperation{Kind: "files", Files: base})
	if err != nil || reverted.Revision == root.Revision || reverted.SourceRevision != root.SourceRevision {
		t.Fatal("returning to old source must retain new ancestry", err, reverted)
	}
	v, _ = draft.SourceVersion(reverted.Revision)
	if v.Parent != saved.Revision || len(draft.Versions()) != 3 {
		t.Fatal(v)
	}
}

func TestSourceVersionIncludesExactSourceAndDeletes(t *testing.T) {
	base := `PREFIX = "a b"
def _helper(x):
    return PREFIX + x
example: word("x") == "a bx"
example: word("y") == "a by"
def word(x):
    result = "empty"
    if x:
        result = _helper(x)
    return result
`
	mutations := []string{
		strings.Replace(base, `"a b"`, `"a  b"`, 1),
		strings.Replace(base, "return PREFIX + x", "return x + PREFIX", 1),
		strings.Replace(base, "    return result", "        return result", 1),
		base + "\n# author comment\n",
	}
	draft, err := NewDraftWorkspace("exact", "Exact source", "dev", Files{"words.star": base})
	if err != nil {
		t.Fatal(err)
	}
	initial := draft.GetSnapshot(false)
	for _, source := range mutations {
		result, snap, err := draft.ApplyState(context.Background(), "dev", draft.GetSnapshot(false).StateToken, DraftOperation{Kind: "files", Files: Files{"words.star": source}})
		if err != nil || result.Status != OK || snap.SourceRevision == initial.SourceRevision {
			t.Fatalf("source change lost: %v %s", err, result.Message)
		}
	}
	snap := draft.GetSnapshot(false)
	result, rejected, err := draft.ApplyState(context.Background(), "dev", snap.StateToken, DraftOperation{Kind: "files", Files: Files{"words.star": "invalid"}})
	if err != nil || result.Status != Error || rejected.StateToken != snap.StateToken || rejected.Revision != snap.Revision || len(draft.Versions()) != 5 {
		t.Fatal("invalid edit changed history", err)
	}
	_, withExtra, err := draft.ApplyState(context.Background(), "dev", snap.StateToken, DraftOperation{Kind: "files", Files: Files{"extra.star": draftZones}})
	if err != nil {
		t.Fatal(err)
	}
	_, deleted, err := draft.ApplyState(context.Background(), "dev", withExtra.StateToken, DraftOperation{Kind: "files", Delete: []string{"extra.star"}})
	if err != nil || deleted.SourceRevision != snap.SourceRevision || deleted.Revision == snap.Revision {
		t.Fatal("deletion did not produce a descendant", err)
	}
}

func TestForkHistoricalSourcePreservesAncestryAndIsolation(t *testing.T) {
	draft, base := newTestDraft(t)
	root := draft.GetSnapshot(false)
	_, current, _ := draft.AddExample("ops", `Zones.zone("DE") == "international"`)
	fork, err := draft.ForkAt("branch", "Earlier experiment", "dev", root.Revision)
	if err != nil {
		t.Fatal(err)
	}
	start := fork.GetSnapshot(false)
	if start.Revision != root.Revision || start.StateToken == root.StateToken || len(fork.Versions()) != 1 || start.Files["zones.star"] != base["zones.star"] {
		t.Fatal(start)
	}
	if _, ok := fork.SourceVersion(current.Revision); ok {
		t.Fatal("fork exposes a version outside its ancestry")
	}
	_, branched, _ := fork.AddExample("dev", `Zones.zone("JP") == "international"`)
	v, _ := fork.SourceVersion(branched.Revision)
	if v.Parent != root.Revision || draft.GetSnapshot(false).Revision != current.Revision {
		t.Fatal("branch changed original draft")
	}
	if _, err := draft.ForkAt("bad", "Bad", "dev", strings.Repeat("0", 64)); err == nil {
		t.Fatal("unknown parent accepted")
	}
}

func TestHashStateConflictIsAtomic(t *testing.T) {
	draft, _ := newTestDraft(t)
	before := draft.GetSnapshot(true)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted, conflicts := 0, 0
	for _, country := range []string{"DE", "JP"} {
		wg.Go(func() {
			<-start
			_, _, err := draft.ApplyState(context.Background(), "ops", before.StateToken, DraftOperation{Kind: "add-example", Example: `Zones.zone("` + country + `") == "international"`})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				accepted++
			} else if errors.Is(err, ErrDraftConflict) {
				conflicts++
			} else {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
	after := draft.GetSnapshot(true)
	if accepted != 1 || conflicts != 1 || len(after.History) != 1 || len(draft.Versions()) != 2 {
		t.Fatal(accepted, conflicts, after)
	}
	_, rejected, err := draft.ApplyState(context.Background(), "ops", before.StateToken, DraftOperation{Kind: "load"})
	if !errors.Is(err, ErrDraftConflict) || !reflect.DeepEqual(rejected.Files, after.Files) {
		t.Fatal(err)
	}
}
