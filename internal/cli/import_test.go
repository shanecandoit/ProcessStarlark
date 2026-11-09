package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shippingProject copies examples/shipping's modules into a temp project.
func shippingProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"zones.star", "shipping.star"} {
		b, err := os.ReadFile(filepath.Join("../../examples/shipping", name))
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, name, string(b))
	}
	return root
}

func TestImportProposesRows(t *testing.T) {
	root := shippingProject(t)
	// Columns in any order; one row known, one contradicting an example, one
	// call answered two ways in the file.
	write(t, root, "zones.csv", "Result,country\ninternational,DE\ndomestic,US\neurope,CA\n\ninternational,JP\nasia,JP\nsouth-america,BR\n")
	before := read(t, root, "zones.star")
	code, out, errout := invoke(t, root, "import", "Zones.zone", filepath.Join(root, "zones.csv"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errout)
	}
	for _, want := range []string{"2 new, 1 already in the examples, 2 conflicting", "Proposed 2 answers"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	for _, want := range []string{`Zones.zone("CA") is "north-america" in the examples, "europe" in the table`, `Zones.zone("JP") is "international" on line 6, "asia" on line 7`} {
		if !strings.Contains(errout, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errout)
		}
	}
	if read(t, root, "zones.star") != before {
		t.Fatal("proposing rows edited the module")
	}
	questions := read(t, root, "questions.txt")
	if !strings.Contains(questions, "Zones.zone(\"DE\") == \"international\"\nZones.zone(\"BR\") == \"south-america\"\n") || strings.Contains(questions, "JP") {
		t.Fatalf("questions:\n%s", questions)
	}
	if !strings.Contains(read(t, root, "park.html"), "south-america") {
		t.Fatal("park.html does not show the proposals")
	}

	// Confirming one removes it from the queue; importing again is idempotent.
	success(t, root, "example", "add", `Zones.zone("DE") == "international"`)
	success(t, root, "import", "Zones.zone", filepath.Join(root, "zones.csv"))
	questions = read(t, root, "questions.txt")
	if strings.Contains(questions, `"DE"`) || strings.Count(questions, `Zones.zone("BR")`) != 1 {
		t.Fatalf("questions after confirm and reimport:\n%s", questions)
	}
}

func TestImportAddWritesExamples(t *testing.T) {
	root := shippingProject(t)
	write(t, root, "zones.tsv", "country\tresult\nDE\tinternational\nUS\tdomestic\nCA\teurope\n")
	out := success(t, root, "import", "--add", "Zones.zone", filepath.Join(root, "zones.tsv"))
	if !strings.Contains(out, "Added 1 example") {
		t.Fatal(out)
	}
	zones := read(t, root, "zones.star")
	if !strings.Contains(zones, `    example: zone("DE") == "international"`) || strings.Contains(zones, "europe") {
		t.Fatal(zones)
	}
	if out := success(t, root, "run", `Shipping.quote("DE", 500)`); strings.TrimSpace(out) != "2399" {
		t.Fatalf("imported example does not answer: %q", out)
	}
}

func TestImportComparesWithCode(t *testing.T) {
	root := shippingProject(t)
	write(t, root, "quotes.csv", "country,grams,cents\nUS,500,599\nUS,1000,699\nFR,500,2500\nZZ,500,1\n")
	code, out, errout := invoke(t, root, "import", "Shipping.quote", filepath.Join(root, "quotes.csv"))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errout)
	}
	for _, want := range []string{"3 new", "agrees with 1 new row and disagrees with 1", "nothing was queued"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	for _, want := range []string{`Shipping.quote("FR", 500) is 2500 in the table; the code says 2399`, `code has no answer: Shipping.quote("ZZ", 500)`} {
		if !strings.Contains(errout, want) {
			t.Errorf("stderr lacks %q:\n%s", want, errout)
		}
	}
	// The rows themselves are not queued, but the dependency call the code
	// could not answer is, as park test would.
	questions := read(t, root, "questions.txt")
	if strings.Contains(questions, "Shipping.quote") || !strings.Contains(questions, "Zones.zone(\"ZZ\")\n") {
		t.Fatalf("questions:\n%s", questions)
	}
	if !strings.Contains(errout, "Queued 1 question the code raised") {
		t.Errorf("stderr does not mention the queued question:\n%s", errout)
	}
	if entries := files(t, root, ".cache"); len(entries) != 0 {
		t.Fatal("comparing rows wrote the cache")
	}

	// --add keeps the file's answers; examples outrank code.
	success(t, root, "import", "--add", "Shipping.quote", filepath.Join(root, "quotes.csv"))
	if out := success(t, root, "run", `Shipping.quote("FR", 500)`); strings.TrimSpace(out) != "2500" {
		t.Fatalf("got %q", out)
	}
}

func TestImportRejectsBadFiles(t *testing.T) {
	root := shippingProject(t)
	cases := map[string]struct{ contents, want string }{
		"unknown column": {"country,weight,cents\nUS,1,2\n", `unknown column "weight"`},
		"missing column": {"country,cents\nUS,2\n", "missing column grams"},
		"repeat column":  {"country,grams,Grams,cents\nUS,1,1,2\n", "column grams appears twice"},
		"bad cell":       {"country,grams,cents\nUS,abc,2\n", `line 2: grams: "abc": this column holds whole numbers`},
		"short row":      {"country,grams,cents\nUS,1\n", "line 2: expected 3 cells, got 2"},
		"header only":    {"country,grams,cents\n", "no rows"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, "in.csv")
			write(t, root, "in.csv", tc.contents)
			code, _, errout := invoke(t, root, "import", "Shipping.quote", path)
			if code != 2 || !strings.Contains(errout, tc.want) {
				t.Fatalf("exit %d, want %q in: %s", code, tc.want, errout)
			}
		})
	}
	if code, _, errout := invoke(t, root, "import", "Shipping.nope", "x.csv"); code != 2 || !strings.Contains(errout, "nope") {
		t.Fatalf("unknown function: %d %s", code, errout)
	}
	if _, err := os.Stat(filepath.Join(root, "park.html")); !os.IsNotExist(err) {
		t.Fatal("a rejected import wrote park.html")
	}
}
