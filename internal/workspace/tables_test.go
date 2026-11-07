package workspace

import (
	"context"
	"strings"
	"testing"
)

func TestPutExampleAddsAndEditsInPlace(t *testing.T) {
	ctx := context.Background()
	files := readDir(t, "../../examples/shipping")
	r := PutExample(ctx, files, "Zones.zone", []string{"DE", "international"}, "")
	if r.Status != OK || !strings.Contains(r.Files["zones.star"], `example: zone("DE") == "international"`) {
		t.Fatalf("%s %s", r.Status, r.Message)
	}
	before := strings.Index(r.Files["zones.star"], `zone("DE")`)
	r = PutExample(ctx, r.Files, "Zones.zone", []string{"DE", "europe"}, `Zones.zone("DE")`)
	if r.Status != OK || !strings.Contains(r.Files["zones.star"], `example: zone("DE") == "europe"`) || strings.Index(r.Files["zones.star"], `zone("DE")`) != before {
		t.Fatalf("not edited in place: %s\n%s", r.Message, r.Files["zones.star"])
	}
	if r := PutExample(ctx, r.Files, "Zones.zone", []string{"US", "x"}, ""); r.Status != Error {
		t.Fatal("duplicate row added")
	}
	// A row that code answers differently is kept, and the difference noted.
	r = PutExample(ctx, files, "Shipping.quote", []string{"US", "1000", "1"}, "")
	if r.Status != OK || len(r.Lines) != 1 || !strings.Contains(r.Message, "answers 1 row differently") {
		t.Fatalf("%s %v", r.Message, r.Lines)
	}
}

func TestPasteExamples(t *testing.T) {
	ctx := context.Background()
	files := readDir(t, "../../examples/shipping")
	paste := "country\tresult\nDE\tinternational\nUS\tdomestic\nCA\teurope\n\nJP\tinternational\n"
	r := PasteExamples(ctx, files, "Zones.zone", paste)
	if r.Status != OK {
		t.Fatal(r.Message)
	}
	for _, want := range []string{"Added 2 rows", "1 already there", `Zones.zone(country="CA") is "north-america", pasted "europe"`} {
		if !strings.Contains(r.Message, want) {
			t.Errorf("message %q lacks %q", r.Message, want)
		}
	}
	if !strings.Contains(r.Files["zones.star"], `zone("JP") == "international"`) || strings.Contains(r.Files["zones.star"], "europe") {
		t.Fatal(r.Files["zones.star"])
	}
	if r := PasteExamples(ctx, files, "Zones.zone", "DE\tinternational\nFR\n"); r.Status != Error || r.Files["zones.star"] != files["zones.star"] {
		t.Fatal("a bad row still wrote", r.Message)
	}
	if r := PasteExamples(ctx, files, "Zones.zone", "MX,north-america\nBR,south-america"); r.Status != OK || !strings.Contains(r.Message, "Added 1 row") {
		t.Fatal("comma-separated paste:", r.Message)
	}
}
