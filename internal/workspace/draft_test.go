package workspace

import (
	"context"
	"testing"
)

const draftShipping = `Process Shipping:
    """Quote shipping in cents from a destination country and a weight in grams."""

    example: quote("US", 500) == 599
    example: quote("CA", 1200) == 2199

    def quote(country, grams):
        """A base fee per zone, plus a fee for each started 500 g."""
        base = {"domestic": 499, "north-america": 1299, "international": 1999}
        per_step = {"domestic": 100, "north-america": 300, "international": 400}
        zone = Zones.zone(country)
        steps = (grams + 499) // 500
        cents = base[zone] + per_step[zone] * steps
        return cents
`

const draftZones = `Process Zones:
    """Which shipping zone each destination country belongs to."""

    example: zone("US") == "domestic"
    example: zone("CA") == "north-america"

    def zone(country):
        """Example-only: the operations team keeps this table."""
        pass
`

func newTestDraft(t *testing.T) (*DraftWorkspace, Files) {
	prodFiles := Files{
		"shipping.star": draftShipping,
		"zones.star":    draftZones,
	}
	dw, err := NewDraftWorkspace("test-draft", "Test Draft", "test-user", prodFiles)
	if err != nil {
		t.Fatal(err)
	}
	return dw, prodFiles
}

func TestNewDraftWorkspace(t *testing.T) {
	dw, prodFiles := newTestDraft(t)

	snap := dw.GetSnapshot(true)
	if snap.ID != "test-draft" {
		t.Errorf("expected id test-draft, got %s", snap.ID)
	}
	if snap.Name != "Test Draft" {
		t.Errorf("expected name Test Draft, got %s", snap.Name)
	}
	if snap.BaseRevision == "" {
		t.Error("base revision should not be empty")
	}
	if snap.Revision == "" {
		t.Error("revision should not be empty")
	}
	if snap.Version != 0 {
		t.Errorf("expected version 0, got %d", snap.Version)
	}
	if len(snap.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(snap.Files))
	}
	if snap.Snapshot.Error != "" {
		t.Errorf("snapshot should be valid: %s", snap.Snapshot.Error)
	}
	if len(snap.Snapshot.Processes) != 2 {
		t.Errorf("expected 2 processes, got %d", len(snap.Snapshot.Processes))
	}

	// Check that files match production
	if snap.Files["shipping.star"] != prodFiles["shipping.star"] {
		t.Error("shipping.star should match production")
	}
	if snap.Files["zones.star"] != prodFiles["zones.star"] {
		t.Error("zones.star should match production")
	}
}

func TestDraftWorkspaceFork(t *testing.T) {
	dw, _ := newTestDraft(t)

	// Make a change first
	_, _, err := dw.AddExample("test-user", `Zones.zone("MX") == "north-america"`)
	if err != nil {
		t.Fatal(err)
	}

	// Fork the draft
	forked, err := dw.Fork("forked-draft", "Forked Draft", "test-user")
	if err != nil {
		t.Fatal(err)
	}

	forkSnap := forked.GetSnapshot(true)
	if forkSnap.ID != "forked-draft" {
		t.Errorf("expected forked id, got %s", forkSnap.ID)
	}
	if forkSnap.Name != "Forked Draft" {
		t.Errorf("expected forked name, got %s", forkSnap.Name)
	}
	if forkSnap.BaseRevision != dw.baseRevision {
		t.Error("fork should have same base revision")
	}
	if forkSnap.Version != 0 {
		t.Errorf("fork should start at version 0, got %d", forkSnap.Version)
	}

	// Fork should have the same files as the parent at fork time
	if forkSnap.Files["zones.star"] != dw.files["zones.star"] {
		t.Error("fork should have parent's files")
	}
}

func TestDraftWorkspaceApplyEdits(t *testing.T) {
	dw, _ := newTestDraft(t)

	// Add a new zone example
	edits := Files{
		"zones.star": `Process Zones:
    """Which shipping zone each destination country belongs to."""

    example: zone("US") == "domestic"
    example: zone("CA") == "north-america"
    example: zone("MX") == "north-america"

    def zone(country):
        """Example-only: the operations team keeps this table."""
        pass
`,
	}

	snap, err := dw.ApplyEdits("test-user", edits, "Add Mexico zone")
	if err != nil {
		t.Fatal(err)
	}

	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}
	if snap.Revision == "" {
		t.Error("revision should not be empty")
	}
	if len(snap.History) != 1 {
		t.Errorf("expected 1 history entry, got %d", len(snap.History))
	}
	if snap.History[0].Message != "Add Mexico zone" {
		t.Errorf("expected message 'Add Mexico zone', got %s", snap.History[0].Message)
	}
	if snap.History[0].Author != "test-user" {
		t.Errorf("expected author test-user, got %s", snap.History[0].Author)
	}

	// Check the file was actually updated
	if !contains(snap.Files["zones.star"], `zone("MX") == "north-america"`) {
		t.Error("zones.star should contain new example")
	}
}

func TestDraftWorkspaceAddExample(t *testing.T) {
	dw, _ := newTestDraft(t)

	result, snap, err := dw.AddExample("test-user", `Zones.zone("FR") == "international"`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != OK {
		t.Errorf("expected OK, got %s: %s", result.Status, result.Message)
	}
	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}

	// Check the example was added
	if !contains(snap.Files["zones.star"], `zone("FR") == "international"`) {
		t.Error("zones.star should contain new example")
	}

	// Add another example
	_, snap2, err := dw.AddExample("test-user", `Zones.zone("DE") == "international"`)
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Version != 2 {
		t.Errorf("expected version 2, got %d", snap2.Version)
	}
}

func TestDraftWorkspaceRemoveExample(t *testing.T) {
	dw, _ := newTestDraft(t)

	// First add an example
	_, _, err := dw.AddExample("test-user", `Zones.zone("FR") == "international"`)
	if err != nil {
		t.Fatal(err)
	}

	// Now remove it
	result, snap, err := dw.RemoveExample("test-user", `Zones.zone("FR")`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != OK {
		t.Errorf("expected OK, got %s: %s", result.Status, result.Message)
	}
	if snap.Version != 2 {
		t.Errorf("expected version 2, got %d", snap.Version)
	}

	// Check the example was removed
	if contains(snap.Files["zones.star"], `zone("FR") == "international"`) {
		t.Error("zones.star should not contain removed example")
	}
}

func TestDraftWorkspaceRun(t *testing.T) {
	dw, _ := newTestDraft(t)

	// Run a call that has an example
	result, snap, err := dw.Run(context.Background(), "test-user", `Shipping.quote("US", 500)`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != OK {
		t.Errorf("expected OK, got %s: %s", result.Status, result.Message)
	}
	if result.Output != "599" {
		t.Errorf("expected output 599, got %s", result.Output)
	}
	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}
}

func TestDraftWorkspaceRunUnresolved(t *testing.T) {
	dw, _ := newTestDraft(t)

	// Run a call that has no example and no code for Zones.zone
	result, snap, err := dw.Run(context.Background(), "test-user", `Shipping.quote("FR", 800)`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != Unresolved {
		t.Errorf("expected Unresolved, got %s: %s", result.Status, result.Message)
	}
	if result.Message == "" {
		t.Error("expected message for unresolved call")
	}
	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}

	// Check question was queued
	if len(snap.Snapshot.Questions) == 0 {
		t.Error("expected question to be queued")
	}
	found := false
	for _, q := range snap.Snapshot.Questions {
		// Input values are Starlark literals, so strings have quotes
		if q.Function == "Zones.zone" && q.Input["country"] == "\"FR\"" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected question for Zones.zone(FR)")
	}
}

func TestDraftWorkspaceTest(t *testing.T) {
	dw, _ := newTestDraft(t)

	result, snap, err := dw.Test(context.Background(), "test-user", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != OK && result.Status != Failed {
		t.Errorf("expected OK or Failed, got %s: %s", result.Status, result.Message)
	}
	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}
}

func TestDraftWorkspaceLoad(t *testing.T) {
	dw, _ := newTestDraft(t)

	// Add an example
	_, _, err := dw.AddExample("test-user", `Zones.zone("MX") == "north-america"`)
	if err != nil {
		t.Fatal(err)
	}

	// Load should prune answered questions and refresh
	snap, err := dw.Load()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != 1 {
		t.Errorf("expected version 1, got %d", snap.Version)
	}
}

func TestDraftWorkspaceDiff(t *testing.T) {
	dw, prodFiles := newTestDraft(t)

	// Add a zone
	_, _, err := dw.AddExample("test-user", `Zones.zone("MX") == "north-america"`)
	if err != nil {
		t.Fatal(err)
	}

	diff, err := dw.Diff(prodFiles)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Diff: baseRev=%s, draftRev=%s, filesChanged=%v, funcChanges=%v", diff.BaseRevision, diff.DraftRevision, diff.FilesChanged, diff.FunctionChanges)
	for _, fc := range diff.FunctionChanges {
		t.Logf("  FuncChange: %s oldHash=%s newHash=%s oldState=%s newState=%s exAdded=%d bodyChanged=%v", fc.Qualified, fc.OldHash, fc.NewHash, fc.OldState, fc.NewState, fc.ExamplesAdded, fc.BodyChanged)
	}

	if diff.BaseRevision == "" {
		t.Error("base revision should not be empty")
	}
	if diff.DraftRevision == "" {
		t.Error("draft revision should not be empty")
	}
	if len(diff.FilesChanged) == 0 {
		t.Error("expected files changed")
	}
	if len(diff.FunctionChanges) == 0 {
		t.Error("expected function changes")
	}

	// Check function changes include Zones.zone
	found := false
	for _, fc := range diff.FunctionChanges {
		if fc.Qualified == "Zones.zone" {
			found = true
			if fc.ExamplesAdded != 1 {
				t.Errorf("expected 1 example added, got %d", fc.ExamplesAdded)
			}
			break
		}
	}
	if !found {
		t.Error("expected Zones.zone in function changes")
	}
}

func TestDraftManager(t *testing.T) {
	dm := NewDraftManager()
	prodFiles := Files{
		"shipping.star": draftShipping,
		"zones.star":    draftZones,
	}

	// Create draft
	dw1, err := dm.Create("draft-1", "Draft 1", "user1", prodFiles)
	if err != nil {
		t.Fatal(err)
	}
	if dw1.id != "draft-1" {
		t.Errorf("expected id draft-1, got %s", dw1.id)
	}

	// Get draft
	dw2, ok := dm.Get("draft-1")
	if !ok {
		t.Error("expected to find draft-1")
	}
	if dw2.id != "draft-1" {
		t.Errorf("expected id draft-1, got %s", dw2.id)
	}

	// List drafts
	list := dm.List()
	if len(list) != 1 {
		t.Errorf("expected 1 draft, got %d", len(list))
	}

	// Delete draft
	deleted := dm.Delete("draft-1")
	if !deleted {
		t.Error("expected draft to be deleted")
	}
	if _, ok := dm.Get("draft-1"); ok {
		t.Error("draft should not exist after delete")
	}

	// Delete non-existent
	deleted = dm.Delete("non-existent")
	if deleted {
		t.Error("deleting non-existent should return false")
	}
}

func TestDraftWorkspaceConcurrentAccess(t *testing.T) {
	dw, _ := newTestDraft(t)

	// Sequential edits to verify version increments
	_, _, err := dw.AddExample("user1", `Zones.zone("MX") == "north-america"`)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = dw.AddExample("user2", `Zones.zone("FR") == "international"`)
	if err != nil {
		t.Fatal(err)
	}

	snap := dw.GetSnapshot(true)
	if snap.Version != 2 {
		t.Errorf("expected version 2, got %d", snap.Version)
	}
	if len(snap.History) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(snap.History))
	}
}

func TestDraftWorkspaceHistory(t *testing.T) {
	dw, _ := newTestDraft(t)

	_, _, _ = dw.AddExample("user1", `Zones.zone("MX") == "north-america"`)
	_, _, _ = dw.AddExample("user2", `Zones.zone("FR") == "international"`)
	_, _, _ = dw.RemoveExample("user1", `Zones.zone("MX")`)

	snap := dw.GetSnapshot(true)
	if len(snap.History) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(snap.History))
	}

	if snap.History[0].Message != "Add example: Zones.zone(\"MX\") == \"north-america\"" {
		t.Errorf("unexpected history[0]: %s", snap.History[0].Message)
	}
	if snap.History[1].Author != "user2" {
		t.Errorf("expected author user2, got %s", snap.History[1].Author)
	}
	if snap.History[2].Message != "Remove example: Zones.zone(\"MX\")" {
		t.Errorf("unexpected history[2]: %s", snap.History[2].Message)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
