package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestToursReplay replays every tour in web/tours, as the playground build does.
func TestToursReplay(t *testing.T) {
	paths, err := filepath.Glob("../../web/tours/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatal("no tours", err)
	}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var tour Tour
		if err := json.Unmarshal(b, &tour); err != nil {
			t.Fatal(path, err)
		}
		tour.Files = readDir(t, filepath.Join("../..", tour.Start))
		if err := Prepare(context.Background(), &tour); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDismiss(t *testing.T) {
	files := readDir(t, "../../examples/shipping")
	r := Run(context.Background(), files, `Shipping.quote("DE", 800)`)
	if len(r.Snapshot.Questions) != 1 {
		t.Fatalf("%+v", r.Snapshot.Questions)
	}
	d := Dismiss(r.Files, `Zones.zone(country="DE")`)
	if d.Status != OK || len(d.Snapshot.Questions) != 0 {
		t.Fatalf("%s %s", d.Status, d.Message)
	}
	if _, ok := d.Files["questions.txt"]; ok {
		t.Fatal("empty queue kept its file")
	}
	if d := Dismiss(d.Files, `Zones.zone("DE")`); d.Status != Error {
		t.Fatal("dismissed a question that isn't open")
	}
}

func TestGoals(t *testing.T) {
	files := readDir(t, "../../examples/shipping")
	last := LastRun{Call: `Shipping.quote(country="FR", grams=2500)`, Status: OK, Output: "3999"}
	cases := []struct {
		goal Goal
		want bool
	}{
		{Goal{Ran: `Shipping.quote("FR", 2500)`}, true},
		{Goal{Ran: `Shipping.quote("FR", 2500)`, Output: "1"}, false},
		{Goal{Status: "unresolved"}, false},
		{Goal{Example: `Zones.zone("FR")`, Output: `"international"`}, true},
		{Goal{Example: `Zones.zone("DE")`}, false},
		{Goal{Not: &Goal{Example: `Zones.zone("DE")`}}, true},
		{Goal{All: []Goal{{Status: OK}, {Question: `Zones.zone("DE")`}}}, false},
	}
	for i, c := range cases {
		if got := Met(files, last, c.goal); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}
