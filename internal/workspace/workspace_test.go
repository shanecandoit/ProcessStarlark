package workspace

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/cli"
)

func readDir(t *testing.T, dir string) Files {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := Files{}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".star") {
			b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name()] = string(b)
		}
	}
	return files
}

func expect(t *testing.T, r Result, status, output string) {
	t.Helper()
	if r.Status != status || r.Output != output {
		t.Fatalf("got %s %q (%s); want %s %q", r.Status, r.Output, r.Message, status, output)
	}
}

func TestRunFromExamplesAndCode(t *testing.T) {
	files := readDir(t, "../../fixtures/roman")
	ctx := context.Background()
	expect(t, Run(ctx, files, `RomanParse.parse("ix")`), OK, "9")
	expect(t, Run(ctx, files, `RomanParse.parse("xiv")`), OK, "14")
	expect(t, Run(ctx, files, `RomanAdd.eval("iv + i")`), OK, "5")
	w := Run(ctx, files, `RomanAdd.eval("ix + i")`).Why
	if w == nil || w.Source != "code" || len(w.Steps) != 2 {
		t.Fatalf("why: %+v", w)
	}
	if s := w.Steps[0]; s.Call != `RomanParse.parse(text="ix")` || s.Output != "9" || s.Source != "example" || s.Path != "roman_parse.star" || s.Line != 4 {
		t.Fatalf("why step: %+v", s)
	}
	r := Load(files)
	if !r.Snapshot.Valid || len(r.Snapshot.Processes) != 2 || !strings.Contains(r.Snapshot.Catalog, "RomanParse") {
		t.Fatalf("bad snapshot: %+v", r.Snapshot)
	}
	if r := Test(ctx, files, ""); r.Status != OK {
		t.Fatalf("test: %s %v", r.Message, r.Lines)
	}
}

func TestQuestionAnsweredInline(t *testing.T) {
	files := readDir(t, "../../examples/shipping")
	ctx := context.Background()
	r := Run(ctx, files, `Shipping.quote("DE", 800)`)
	if r.Status != Unresolved || len(r.Snapshot.Questions) != 1 || r.Snapshot.Questions[0].Line != `Zones.zone("DE")` {
		t.Fatalf("expected a question for DE: %+v", r)
	}
	if !strings.Contains(r.Files["questions.txt"], `Zones.zone("DE")`) {
		t.Fatal("question not queued in questions.txt")
	}
	r = AddExample(r.Files, `Zones.zone("DE") == "international"`)
	if r.Status != OK || len(r.Snapshot.Questions) != 0 {
		t.Fatalf("answer did not clear the question: %+v", r)
	}
	if _, ok := r.Files["questions.txt"]; ok {
		t.Fatal("empty queue should remove questions.txt")
	}
	expect(t, Run(ctx, r.Files, `Shipping.quote("DE", 800)`), OK, "2799")
}

func TestProposalConfirmed(t *testing.T) {
	files := Files{"words.star": "Process Words:\n    example: shout(\"a\") == \"A!\"\n\n    def shout(text):\n        loud = text.upper() + \"!\"\n        return loud\n"}
	ctx := context.Background()
	r := Run(ctx, files, `Words.shout("hi")`)
	expect(t, r, Unconfirmed, `"HI!"`)
	if len(r.Snapshot.Questions) != 1 || !r.Snapshot.Questions[0].HasProposal {
		t.Fatalf("expected a proposal: %+v", r.Snapshot.Questions)
	}
	r = AddExample(r.Files, r.Snapshot.Questions[0].Line)
	if r.Status != OK || len(r.Snapshot.Questions) != 0 || r.Snapshot.Processes[0].Functions[0].State != "implemented" {
		t.Fatalf("confirming did not make the function ready: %+v", r)
	}
	expect(t, Run(ctx, r.Files, `Words.shout("ok")`), OK, `"OK!"`)
	r = RemoveExample(r.Files, `Words.shout("hi")`)
	if r.Status != OK || strings.Contains(r.Files["words.star"], `"hi"`) {
		t.Fatalf("remove failed: %+v", r)
	}
}

func TestErrorsKeepFiles(t *testing.T) {
	files := readDir(t, "../../fixtures/roman")
	r := Run(context.Background(), files, `Nope.f(1)`)
	if r.Status != Error || !r.Snapshot.Valid || len(r.Files) != len(files) {
		t.Fatalf("a bad call should leave a valid snapshot: %+v", r)
	}
	files["broken.star"] = "def f(x):\n    return x + 1\n"
	r = Load(files)
	if r.Status != Error || r.Snapshot.Valid || r.Snapshot.Error == "" {
		t.Fatalf("expected an invalid project: %+v", r.Snapshot)
	}
	broken := Files{"m.star": "Process M:\n    def f(x):\n        y = x * 2\n        return y\n    example: f(1) == 3\n    example: f(2) == 4\n"}
	r = Test(context.Background(), broken, "")
	if r.Status != Failed || len(r.Lines) != 1 || !r.Lines[0].Failed {
		t.Fatalf("expected one failing check: %+v", r)
	}
}

// The playground and the CLI must agree: same queue and same catalog for the
// same project and the same commands.
func TestMatchesCLI(t *testing.T) {
	files := readDir(t, "../../examples/shipping")
	root := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errout bytes.Buffer
	cli.Main([]string{"--root", root, "run", `Shipping.quote("JP", 100)`}, &out, &errout)
	cli.Main([]string{"--root", root, "run", `Shipping.quote("DE", 800)`}, &out, &errout)
	r := Run(context.Background(), files, `Shipping.quote("JP", 100)`)
	r = Run(context.Background(), r.Files, `Shipping.quote("DE", 800)`)
	for _, name := range []string{"questions.txt", "park.html"} {
		want, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		got := r.Files[name]
		if name == "park.html" {
			got = r.Snapshot.Catalog
		}
		if got != string(want) {
			t.Fatalf("%s differs between the CLI and the workspace", name)
		}
	}
}

// A downloaded project must open in the CLI as-is.
func TestArchiveOpensInCLI(t *testing.T) {
	r := Run(context.Background(), readDir(t, "../../examples/shipping"), `Shipping.quote("JP", 100)`)
	r = Archive(r.Files)
	if r.Status != Unresolved && r.Status != OK || len(r.Archive) == 0 {
		t.Fatalf("archive failed: %s %s", r.Status, r.Message)
	}
	zr, err := zip.NewReader(bytes.NewReader(r.Archive), int64(len(r.Archive)))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ArchiveDir), 0755); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if err := os.WriteFile(filepath.Join(root, f.Name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(names, ",") != "park-project/park.html,park-project/questions.txt,park-project/shipping.star,park-project/zones.star" {
		t.Fatalf("unexpected entries: %v", names)
	}
	dir := filepath.Join(root, ArchiveDir)
	html, _ := os.ReadFile(filepath.Join(dir, "park.html"))
	var out, errout bytes.Buffer
	if code := cli.Main([]string{"--root", dir, "list"}, &out, &errout); code != 0 || !strings.Contains(out.String(), `Zones.zone("JP")`) {
		t.Fatalf("CLI could not open the archive: %d %s %s", code, out.String(), errout.String())
	}
	if code := cli.Main([]string{"--root", dir, "build"}, &out, &errout); code != 0 {
		t.Fatal(errout.String())
	}
	if rebuilt, _ := os.ReadFile(filepath.Join(dir, "park.html")); !bytes.Equal(rebuilt, html) {
		t.Fatal("archived park.html differs from a CLI rebuild")
	}
}
