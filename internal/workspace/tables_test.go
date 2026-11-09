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
	for _, want := range []string{"Added 2 rows", "1 already there", "Left out 1 row that contradicts the table"} {
		if !strings.Contains(r.Message, want) {
			t.Errorf("message %q lacks %q", r.Message, want)
		}
	}
	if len(r.Lines) != 1 || !strings.Contains(r.Lines[0].Text, `Zones.zone("CA") is "north-america" in the examples, "europe" in the table`) {
		t.Errorf("lines: %v", r.Lines)
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

func TestImportRowsProposes(t *testing.T) {
	ctx := context.Background()
	files := readDir(t, "../../examples/shipping")
	// Header columns in any order, matched without regard to case.
	r := ImportRows(ctx, files, "Zones.zone", "Result,Country\ninternational,DE\ndomestic,US\n", true)
	if r.Status != OK || !strings.Contains(r.Message, "Proposed 1 answer") {
		t.Fatalf("%s: %s", r.Status, r.Message)
	}
	if r.Files["zones.star"] != files["zones.star"] {
		t.Fatal("proposing rows edited the module")
	}
	if q := r.Files["questions.txt"]; !strings.Contains(q, `Zones.zone("DE") == "international"`) {
		t.Fatalf("questions:\n%s", q)
	}
	if len(r.Snapshot.Questions) != 1 || !r.Snapshot.Questions[0].HasProposal {
		t.Fatalf("snapshot questions: %+v", r.Snapshot.Questions)
	}
	// A header naming an unknown column is an error, not data.
	if r := ImportRows(ctx, files, "Zones.zone", "country,zone\nDE,international\n", true); r.Status != Error || !strings.Contains(r.Message, `unknown column "zone"`) {
		t.Fatalf("%s: %s", r.Status, r.Message)
	}
}

func TestImportRowsQueuesDependencyQuestions(t *testing.T) {
	ctx := context.Background()
	files := readDir(t, "../../examples/shipping")
	// Shipping.quote is ready, so nothing is proposed for its own rows, but
	// the call to Zones.zone that nothing answers is queued.
	r := ImportRows(ctx, files, "Shipping.quote", "country,grams,cents\nZZ,500,1\nUS,1000,699\n", true)
	if r.Status != OK || !strings.Contains(r.Message, "Proposed nothing") || !strings.Contains(r.Message, "Queued 1 question the code raised") {
		t.Fatalf("%s: %s", r.Status, r.Message)
	}
	q := r.Files["questions.txt"]
	if strings.Contains(q, "Shipping.quote") || !strings.Contains(q, `Zones.zone("ZZ")`) {
		t.Fatalf("questions:\n%s", q)
	}
	if len(r.Lines) != 1 || !strings.Contains(r.Lines[0].Text, `code has no answer: Shipping.quote("ZZ", 500)`) {
		t.Fatalf("lines: %v", r.Lines)
	}
}
