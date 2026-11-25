package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func TestPinnedBehaviorComparisonDoesNotMutateState(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		ctx := context.Background()
		d := releaseDraft(t, s, "comparison")
		baseline := approveReview(t, s, reviewDraft(t, s, d, 0).ID, 0)
		original := d
		edits := workspace.Files{"zones.star": strings.ReplaceAll(d.Files["zones.star"], `zone("US") == "domestic"`, `zone("US") == "international"`)}
		edits["zones.star"] = strings.ReplaceAll(edits["zones.star"], "    example: zone(\"CA\") == \"north-america\"\n", "")
		_, d, err := s.Apply(ctx, d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "files", Files: edits})
		if err != nil {
			t.Fatal(err)
		}
		_, d, err = s.Apply(ctx, d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("DE") == "international"`})
		if err != nil {
			t.Fatal(err)
		}
		before, err := s.Read(ctx, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		record := before.Record()
		scenarios := []workspace.Scenario{
			{Name: "US changed", Call: `Shipping.quote("US", 800)`},
			{Name: "Germany newly answered", Call: `Shipping.quote("DE", 800)`},
			{Name: "Canada missing", Call: `Shipping.quote("CA", 800)`},
			{Name: "Japan still missing", Call: `Shipping.quote("JP", 800)`},
			{Name: "Known exact answer", Call: `Shipping.quote("FR", 2500)`},
			{Name: "Bad input", Call: `Shipping.quote("US", "bad")`},
		}
		command := workspace.ReleaseCommand{Action: "compare", DraftID: d.ID, DraftRevision: d.Revision, ExpectedGeneration: generation(1), Scenarios: scenarios}
		result := releaseExec(t, s, command)
		report := result.Comparison
		if report.DraftRevision != d.Revision || report.ReleaseID != baseline.Release.ID || report.Generation != 1 || len(report.ScenarioSetHash) != 64 || len(report.ID) != 64 {
			t.Fatal(report)
		}
		expected := []string{"changed", "newly_answered", "became_unresolved", "unresolved", "unchanged", "error"}
		for i, row := range report.Rows {
			if row.Category != expected[i] {
				t.Errorf("%s: got %s", row.Name, row.Category)
			}
		}
		if report.Rows[0].Before.Output != "699" || report.Rows[0].After.Output != "2799" {
			t.Fatal(report.Rows[0])
		}
		after, err := s.Read(ctx, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(record, after.Record()) {
			t.Fatal("comparison changed source, questions, tokens, or activity")
		}
		// The same retained source version remains selectable after a newer edit.
		_, newer, err := s.Apply(ctx, d.ID, "ops", d.StateToken, nil, workspace.DraftOperation{Kind: "add-example", Example: `Zones.zone("JP") == "international"`})
		if err != nil {
			t.Fatal(err)
		}
		repeated := releaseExec(t, s, command).Comparison
		if repeated.ID != report.ID || !reflect.DeepEqual(repeated.Rows, report.Rows) || newer.Revision == report.DraftRevision {
			t.Fatal("comparison did not use the pinned historical version")
		}
		// A baseline-source comparison is also allowed without moving the draft head.
		command.DraftRevision = original.Revision
		old := releaseExec(t, s, command).Comparison
		if old.Rows[0].Category != "unchanged" || old.DraftRevision != original.Revision {
			t.Fatal(old)
		}
		production := releaseExec(t, s, workspace.ReleaseCommand{Action: "status"})
		if production.Production != baseline.Production {
			t.Fatal("comparison changed production")
		}
		// The public boundary must not leak temporary captured files/manifests.
		h, err := NewStoredDraftHandler(s)
		if err != nil {
			t.Fatal(err)
		}
		w := send(h, "POST", "/api/v1/drafts/"+d.ID+"/compare", map[string]any{"expected_generation": 1, "draft_revision": d.Revision, "scenarios": scenarios})
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var response map[string]json.RawMessage
		if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response["candidate"] != nil || response["release"] != nil || response["comparison"] == nil {
			t.Fatal("snapshot capture leaked through API", w.Body.String())
		}
	})
}

func TestComparisonRejectsInvalidPinsAndScenarioSets(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		d := releaseDraft(t, s, "invalid-comparison")
		valid := workspace.ReleaseCommand{Action: "compare", DraftID: d.ID, DraftRevision: d.Revision, ExpectedGeneration: generation(0), Scenarios: []workspace.Scenario{{Name: "US", Call: `Shipping.quote("US", 500)`}}}
		if _, err := s.ExecuteRelease(context.Background(), valid); !errors.Is(err, workspace.ErrReleaseNotFound) {
			t.Fatal("missing active release accepted", err)
		}
		approveReview(t, s, reviewDraft(t, s, d, 0).ID, 0)
		valid.ExpectedGeneration = generation(1)
		for _, tc := range []struct {
			name   string
			change func(*workspace.ReleaseCommand)
			want   error
		}{
			{"missing generation", func(c *workspace.ReleaseCommand) { c.ExpectedGeneration = nil }, workspace.ErrReleaseInput},
			{"stale generation", func(c *workspace.ReleaseCommand) { c.ExpectedGeneration = generation(0) }, workspace.ErrReleaseConflict},
			{"missing revision", func(c *workspace.ReleaseCommand) { c.DraftRevision = "" }, workspace.ErrReleaseInput},
			{"unknown revision", func(c *workspace.ReleaseCommand) { c.DraftRevision = "missing" }, workspace.ErrReleaseNotFound},
			{"empty scenarios", func(c *workspace.ReleaseCommand) { c.Scenarios = nil }, workspace.ErrReleaseInput},
			{"repeated name", func(c *workspace.ReleaseCommand) {
				c.Scenarios = []workspace.Scenario{{Name: "US", Call: "x"}, {Name: " US ", Call: "y"}}
			}, workspace.ErrReleaseInput},
			{"too many", func(c *workspace.ReleaseCommand) { c.Scenarios = make([]workspace.Scenario, 33) }, workspace.ErrReleaseInput},
			{"empty call", func(c *workspace.ReleaseCommand) { c.Scenarios = []workspace.Scenario{{Name: "US", Call: " "}} }, workspace.ErrReleaseInput},
		} {
			t.Run(tc.name, func(t *testing.T) {
				c := valid
				tc.change(&c)
				_, err := s.ExecuteRelease(context.Background(), c)
				if !errors.Is(err, tc.want) {
					t.Fatal(err)
				}
			})
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.ExecuteRelease(ctx, valid); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		// Malformed calls are row outcomes, not a failed whole comparison request.
		valid.Scenarios = []workspace.Scenario{{Name: "bad syntax", Call: "not a literal call"}}
		if r := releaseExec(t, s, valid).Comparison.Rows[0]; r.Category != "error" || r.After.Status != workspace.Error {
			t.Fatal(r)
		}
	})
}

func TestComparisonUnconfirmedAnswersRemainExplicit(t *testing.T) {
	testReleaseStores(t, func(t *testing.T, s DraftStore) {
		d := releaseDraft(t, s, "unconfirmed-comparison")
		approveReview(t, s, reviewDraft(t, s, d, 0).ID, 0)
		_, d, err := s.Apply(context.Background(), d.ID, "developer", d.StateToken, nil, workspace.DraftOperation{Kind: "files", Files: workspace.Files{"zones.star": `Process Zones:
    example: zone("US") == "domestic"
    def zone(country):
        result = "international"
        return result
`}})
		if err != nil {
			t.Fatal(err)
		}
		report := releaseExec(t, s, workspace.ReleaseCommand{Action: "compare", DraftID: d.ID, DraftRevision: d.Revision, ExpectedGeneration: generation(1), Scenarios: []workspace.Scenario{{Name: "Unknown zone", Call: `Zones.zone("JP")`}}}).Comparison
		row := report.Rows[0]
		if row.Category != "unconfirmed" || row.Before.Status != workspace.Unresolved || row.After.Status != workspace.Unconfirmed || row.After.Output != `"international"` {
			t.Fatal(row)
		}
		stored, err := s.Read(context.Background(), d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stored.Questions(false)) != 0 {
			t.Fatal("comparison persisted unconfirmed proposal")
		}
	})
}
