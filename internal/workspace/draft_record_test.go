package workspace

import (
	"context"
	"reflect"
	"testing"
)

func TestDraftRecordRoundTripAndCopyIsolation(t *testing.T) {
	draft, _ := newTestDraft(t)
	_, _, err := draft.Run(context.Background(), "ops", `Shipping.quote("DE",800)`)
	if err != nil {
		t.Fatal(err)
	}
	record := draft.Record()
	restored, err := RestoreDraft(record)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(draft.GetSnapshot(true), restored.GetSnapshot(true)) || !reflect.DeepEqual(draft.Questions(true), restored.Questions(true)) {
		t.Fatal("record did not round-trip")
	}
	record.Draft.Files["zones.star"] = "changed"
	record.Questions[0].Input["country"] = "changed"
	record.Versions[0].Files["zones.star"] = "changed"
	if !reflect.DeepEqual(draft.Record(), restored.Record()) {
		t.Fatal("restore aliases input record")
	}
}

func TestRestoreRejectsCorruptSourceActivityAndQueue(t *testing.T) {
	draft, _ := newTestDraft(t)
	_, _, _ = draft.Run(context.Background(), "ops", `Shipping.quote("DE",800)`)
	for name, change := range map[string]func(*DraftRecord){
		"source":   func(r *DraftRecord) { r.Versions[0].Files["zones.star"] += "\n# corrupt source\n" },
		"activity": func(r *DraftRecord) { r.Draft.History = nil },
		"queue":    func(r *DraftRecord) { r.Questions = []QuestionRecord{} },
		"token":    func(r *DraftRecord) { r.Draft.StateToken = "wrong" },
	} {
		t.Run(name, func(t *testing.T) {
			record := draft.Record()
			change(&record)
			if _, err := RestoreDraft(record); err == nil {
				t.Fatal("corrupt record accepted")
			}
		})
	}
}
