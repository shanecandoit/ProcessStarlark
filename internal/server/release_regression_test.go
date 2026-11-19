package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/draftstore"
	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func testReleaseStores(t *testing.T, test func(*testing.T, DraftStore)) {
	t.Helper()
	source := decodeResponse(t, send(shippingHandler(t), "GET", "/api/v1/base", nil))["files"].(map[string]any)
	base := workspace.Files{}
	for n, v := range source {
		base[n] = v.(string)
	}
	t.Run("memory", func(t *testing.T) { test(t, &memoryStore{base: base, drafts: workspace.NewDraftManager()}) })
	t.Run("sqlite", func(t *testing.T) {
		s, err := draftstore.Open(filepath.Join(t.TempDir(), "test.sqlite"), base)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		test(t, s)
	})
}
func generation(n int) *int { return &n }
func releaseExec(t *testing.T, s DraftStore, c workspace.ReleaseCommand) workspace.ReleaseResult {
	t.Helper()
	r, err := s.ExecuteRelease(context.Background(), c)
	if err != nil {
		t.Fatal(c.Action, err)
	}
	return r
}
func releaseDraft(t *testing.T, s DraftStore, id string) workspace.DraftSnapshot {
	t.Helper()
	snap, err := s.Create(context.Background(), id, id, "ops", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
func reviewDraft(t *testing.T, s DraftStore, d workspace.DraftSnapshot, n int) *workspace.Review {
	t.Helper()
	return releaseExec(t, s, workspace.ReleaseCommand{Action: "request", DraftID: d.ID, Author: "developer", ExpectedState: d.StateToken, ExpectedGeneration: generation(n)}).Review
}
func approveReview(t *testing.T, s DraftStore, id string, n int) workspace.ReleaseResult {
	t.Helper()
	return releaseExec(t, s, workspace.ReleaseCommand{Action: "approve", ID: id, Author: "reviewer", ExpectedGeneration: generation(n)})
}

func TestReviewRejectsBadChecksWithoutWrites(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		for _, tc := range []struct{ name, source string }{
			{"wrong answer", `Process Broken:
    example: f(1) == 2
    example: f(2) == 3
    def f(n):
        result = n + 10
        return result
`},
			{"unready implementation", `Process Broken:
    def f(n):
        result = n
        return result
`},
			{"missing dependency", `Process Broken:
    example: f(1) == 1
    example: f(2) == 2
    def f(n):
        result = table(n)
        return result
    def table(n):
        pass
`},
			{"unconfirmed dependency", `Process Broken:
    example: f(1) == 1
    example: f(2) == 2
    def f(n):
        result = unready(n)
        return result
    def unready(n):
        result = n
        return result
`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				d := releaseDraft(t, s, strings.ReplaceAll(tc.name, " ", "-"))
				r, d, err := s.Apply(context.Background(), d.ID, "developer", d.StateToken, nil, workspace.DraftOperation{Kind: "files", Files: workspace.Files{"broken.star": tc.source}})
				if err != nil || r.Status != workspace.OK {
					t.Fatal(r.Message, err)
				}
				before, _ := s.Read(context.Background(), d.ID)
				beforeRecord := before.Record()
				_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "request", DraftID: d.ID, Author: "developer", ExpectedState: d.StateToken, ExpectedGeneration: generation(0)})
				if !errors.Is(err, workspace.ErrReleaseChecks) {
					t.Fatal("expected failed review", err)
				}
				after, _ := s.Read(context.Background(), d.ID)
				if !reflect.DeepEqual(beforeRecord, after.Record()) {
					t.Fatal("failed review changed draft")
				}
				state := releaseExec(t, s, workspace.ReleaseCommand{Action: "reviews"})
				if len(state.Reviews) != 0 || state.Production.Generation != 0 {
					t.Fatal("failed checks persisted review", state)
				}
			})
		}
	})
}

func TestReviewPreconditionsAndDecisions(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		d := releaseDraft(t, s, "draft")
		command := workspace.ReleaseCommand{Action: "request", DraftID: d.ID, Author: "developer", ExpectedState: d.StateToken, ExpectedGeneration: generation(0)}
		for _, tc := range []struct {
			name   string
			mutate func(*workspace.ReleaseCommand)
			want   error
		}{
			{"missing generation", func(c *workspace.ReleaseCommand) { c.ExpectedGeneration = nil }, workspace.ErrReleaseInput},
			{"negative generation", func(c *workspace.ReleaseCommand) { c.ExpectedGeneration = generation(-1) }, workspace.ErrReleaseInput},
			{"stale generation", func(c *workspace.ReleaseCommand) { c.ExpectedGeneration = generation(1) }, workspace.ErrReleaseConflict},
			{"missing state", func(c *workspace.ReleaseCommand) { c.ExpectedState = "" }, workspace.ErrReleaseInput},
			{"stale state", func(c *workspace.ReleaseCommand) { c.ExpectedState = "stale" }, workspace.ErrReleaseConflict},
			{"missing author", func(c *workspace.ReleaseCommand) { c.Author = " " }, workspace.ErrReleaseInput},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := command
				tc.mutate(&c)
				_, err := s.ExecuteRelease(context.Background(), c)
				if !errors.Is(err, tc.want) {
					t.Fatal(err)
				}
			})
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.ExecuteRelease(ctx, command); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		review := reviewDraft(t, s, d, 0)
		original := review.Files["shipping.star"]
		review.Files["shipping.star"] = "corrupted by caller"
		fetched := releaseExec(t, s, workspace.ReleaseCommand{Action: "review", ID: review.ID}).Review
		if fetched.Files["shipping.star"] != original {
			t.Fatal("returned review aliases store")
		}
		// Operational activity is not a source edit and must not invalidate checks.
		_, d, err := s.Apply(context.Background(), d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "run", Call: `Shipping.quote("DE", 800)`})
		if err != nil {
			t.Fatal(err)
		}
		accepted := approveReview(t, s, review.ID, 0)
		if accepted.Production.Generation != 1 || accepted.Release.Revision != review.Revision {
			t.Fatal(accepted)
		}
		_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "approve", ID: review.ID, Author: "reviewer", ExpectedGeneration: generation(1)})
		if !errors.Is(err, workspace.ErrReleaseConflict) {
			t.Fatal("repeat approval accepted", err)
		}
		next := reviewDraft(t, s, d, 1)
		_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "reject", ID: next.ID, Author: "reviewer"})
		if !errors.Is(err, workspace.ErrReleaseInput) {
			t.Fatal("empty rejection reason", err)
		}
		rejected := releaseExec(t, s, workspace.ReleaseCommand{Action: "reject", ID: next.ID, Author: "reviewer", Reason: "Needs a business example"})
		if rejected.Review.Status != "rejected" || rejected.Production != accepted.Production {
			t.Fatal(rejected)
		}
		_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "approve", ID: next.ID, Author: "reviewer", ExpectedGeneration: generation(1)})
		if !errors.Is(err, workspace.ErrReleaseConflict) {
			t.Fatal("rejected review activated", err)
		}
	})
}

func TestStaleSourceAndProductionCannotActivate(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		d := releaseDraft(t, s, "draft")
		review := reviewDraft(t, s, d, 0)
		_, d, err := s.Apply(context.Background(), d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "approve", ID: review.ID, Author: "reviewer", ExpectedGeneration: generation(0)})
		if !errors.Is(err, workspace.ErrReleaseConflict) {
			t.Fatal("stale source activated", err)
		}
		// Returning to identical bytes still creates a new source-version head.
		_, d, err = s.Apply(context.Background(), d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "remove-example", Call: `Zones.zone("DE")`})
		if err != nil {
			t.Fatal(err)
		}
		if d.SourceRevision != review.SourceRevision || d.Revision == review.Revision {
			t.Fatal("expected identical source with new ancestry")
		}
		_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "approve", ID: review.ID, Author: "reviewer", ExpectedGeneration: generation(0)})
		if !errors.Is(err, workspace.ErrReleaseConflict) {
			t.Fatal("source revert revived stale approval", err)
		}
		a := reviewDraft(t, s, d, 0)
		b := reviewDraft(t, s, d, 0)
		active := approveReview(t, s, a.ID, 0)
		_, err = s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "approve", ID: b.ID, Author: "reviewer", ExpectedGeneration: generation(1)})
		if !errors.Is(err, workspace.ErrReleaseConflict) {
			t.Fatal("stale base activated with fresh token", err)
		}
		status := releaseExec(t, s, workspace.ReleaseCommand{Action: "status"})
		if status.Production != active.Production || len(status.Activations) != 1 {
			t.Fatal("rejected approval changed state", status)
		}
	})
}

func TestConcurrentReviewActivations(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		d := releaseDraft(t, s, "draft")
		a := reviewDraft(t, s, d, 0)
		b := reviewDraft(t, s, d, 0)
		h, err := NewStoredDraftHandler(s)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan *httptest.ResponseRecorder, 2)
		var wg sync.WaitGroup
		for _, id := range []string{a.ID, b.ID} {
			wg.Go(func() {
				<-start
				results <- send(h, http.MethodPost, "/api/v1/reviews/"+id+"/approve", map[string]any{"author": "reviewer", "expected_generation": 0})
			})
		}
		close(start)
		wg.Wait()
		close(results)
		codes := map[int]int{}
		for r := range results {
			codes[r.Code]++
			if r.Code != 200 && r.Code != 409 {
				t.Fatal(r.Code, r.Body.String())
			}
		}
		if codes[200] != 1 || codes[409] != 1 {
			t.Fatal(codes)
		}
		state := releaseExec(t, s, workspace.ReleaseCommand{Action: "releases"})
		if len(state.Releases) != 1 || state.Production.Generation != 1 {
			t.Fatal(state)
		}
	})
}

func TestReviewHTTPBoundaryAndRuntimeMismatch(t *testing.T) {
	h := shippingHandler(t)
	for _, list := range []string{"reviews", "releases"} {
		w := send(h, "GET", "/api/v1/"+list, nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		got, ok := decodeResponse(t, w)[list].([]any)
		if !ok || len(got) != 0 {
			t.Fatal("expected empty list", w.Body.String())
		}
	}
	createDraft(t, h, "draft")
	for _, path := range []string{"/api/v1/reviews/missing", "/api/v1/releases/missing"} {
		if w := send(h, "GET", path, nil); w.Code != 404 {
			t.Fatal(w.Code)
		}
	}
	for _, path := range []string{"/api/v1/drafts/draft/reviews", "/api/v1/reviews/missing/approve", "/api/v1/production/rollback"} {
		w := send(h, "POST", path, map[string]any{"author": "ops"})
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	// The boundary must reject metadata clients attempt to forge.
	w := send(h, "POST", "/api/v1/drafts/draft/reviews", map[string]any{"author": "ops", "checks": map[string]any{"failures": 0}})
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	s := &memoryStore{base: workspace.Files{"a.star": "Process A:\n    def f(n):\n        pass\n"}, drafts: workspace.NewDraftManager()}
	d := releaseDraft(t, s, "runtime")
	review := reviewDraft(t, s, d, 0)
	s.mu.Lock()
	v := s.releases.Reviews[review.ID]
	v.Runtime = "old-runtime"
	s.releases.Reviews[review.ID] = v
	s.mu.Unlock()
	_, err := s.ExecuteRelease(context.Background(), workspace.ReleaseCommand{Action: "approve", ID: review.ID, Author: "reviewer", ExpectedGeneration: generation(0)})
	if !errors.Is(err, workspace.ErrReleaseConflict) {
		t.Fatal("old runtime approved", err)
	}
	// A runtime upgrade must also refuse old active evaluations and rollbacks.
	s.mu.Lock()
	s.releases.Releases = map[string]workspace.Release{"old": {ID: "old", Runtime: "old-runtime"}}
	s.releases.Production = workspace.Production{ReleaseID: "old", Generation: 1}
	s.mu.Unlock()
	for _, c := range []workspace.ReleaseCommand{{Action: "run", Call: "A.f(1)"}, {Action: "rollback", ID: "old", Author: "ops", Reason: "test", ExpectedGeneration: generation(1)}} {
		_, err = s.ExecuteRelease(context.Background(), c)
		if !errors.Is(err, workspace.ErrReleaseConflict) {
			t.Fatal(c.Action, err)
		}
	}
}
