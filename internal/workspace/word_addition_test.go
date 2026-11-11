package workspace

import (
	"context"
	"maps"
	"strings"
	"testing"
)

func TestWordAdditionDefaultAndProvenance(t *testing.T) {
	files := readDir(t, "../../examples/word-addition")
	if !maps.Equal(files, readDir(t, "../..")) {
		t.Fatal("the root demo must match the bundled word-addition sample")
	}
	ctx := context.Background()
	r := Run(ctx, files, `WordAdd.eval("twenty-two plus eleven")`)
	expect(t, r, OK, "33")
	if r.Why == nil || r.Why.Source != "code" || len(r.Why.Steps) != 2 {
		t.Fatalf("the default call must compute its answer: %+v", r.Why)
	}
	if r.Why.Function != "WordAdd.eval" || len(r.Why.Inputs) != 1 || r.Why.Inputs[0] != "text" || r.Why.Input["text"] != `"twenty-two plus eleven"` || r.Why.Failed {
		t.Fatalf("trace must expose literal inputs and explicit outcome: %+v", r.Why)
	}
	left, right := r.Why.Steps[0], r.Why.Steps[1]
	if left.Source != "code" || left.Output != "22" || len(left.Steps) != 2 {
		t.Fatalf("twenty-two must compose two lookups: %+v", left)
	}
	if right.Source != "code" || len(right.Steps) != 1 || right.Steps[0].Source != "example" || right.Steps[0].Call != `WordValues.value(word="eleven")` {
		t.Fatalf("eleven must use its saved word meaning: %+v", right)
	}
	for _, leaf := range append(left.Steps, right.Steps...) {
		if leaf.Source != "example" || len(leaf.Steps) != 0 {
			t.Fatalf("word meanings must be example leaves: %+v", leaf)
		}
	}
	if checked := Test(ctx, files, ""); checked.Status != OK {
		t.Fatalf("sample checks: %s %v", checked.Message, checked.Lines)
	}

	r = AddExample(files, `WordAdd.eval("twenty-two plus eleven") == 33`)
	expect(t, r, OK, "")
	r = Run(ctx, r.Files, `WordAdd.eval("twenty-two plus eleven")`)
	if r.Why.Source != "example" || len(r.Why.Steps) != 0 {
		t.Fatalf("a whole-expression example must bypass computation: %+v", r.Why)
	}
}

func TestWordAdditionMissingMeaningAndReuse(t *testing.T) {
	files := readDir(t, "../../examples/word-addition")
	ctx := context.Background()
	r := Run(ctx, files, `WordAdd.eval("twenty-two plus twelve")`)
	if r.Status != Unresolved || len(r.Snapshot.Questions) != 1 || r.Snapshot.Questions[0].Line != `WordValues.value("twelve")` {
		t.Fatalf("expected only the missing word meaning: %+v", r)
	}
	q := r.Snapshot.Questions[0]
	if q.Input["word"] != `"twelve"` || len(q.Inputs) != 1 || q.Inputs[0] != "word" {
		t.Fatalf("question needs structured input for its display: %+v", q)
	}
	if !r.Why.Failed || !r.Why.Steps[1].Failed || !r.Why.Steps[1].Steps[0].Failed || r.Why.Steps[1].Steps[0].Source != "unknown" {
		t.Fatalf("missing answers must preserve blocked ancestors: %+v", r.Why)
	}
	r = AddExample(r.Files, `WordValues.value("twelve") == 12`)
	if r.Status != OK || len(r.Snapshot.Questions) != 0 {
		t.Fatalf("answer did not resolve the question: %+v", r)
	}
	for call, want := range map[string]string{
		`WordAdd.eval("twenty-two plus twelve")`: "34",
		`WordAdd.eval("twelve plus two")`:        "14",
	} {
		expect(t, Run(ctx, r.Files, call), OK, want)
	}
}

func TestWordAdditionGrammar(t *testing.T) {
	files := readDir(t, "../../examples/word-addition")
	ctx := context.Background()
	for call, want := range map[string]string{
		`WordAdd.eval("  TWENTY-TWO  PLUS\tELEVEN  ")`: "33",
		`WordAdd.eval("zero plus zero")`:               "0",
		`WordAdd.eval("ninety-nine plus ninety-nine")`: "198",
		`NumberWords.parse(" ninety-nine ")`:           "99",
	} {
		expect(t, Run(ctx, files, call), OK, want)
	}
	for _, call := range []string{
		`WordAdd.eval("eleven-two plus one")`,
		`WordAdd.eval("twenty-zero plus one")`,
		`WordAdd.eval("twenty-eleven plus one")`,
		`WordAdd.eval("twenty--two plus one")`,
		`WordAdd.eval("one-hundred plus one")`,
		`WordAdd.eval("22 plus eleven")`,
		`WordAdd.eval("twenty two plus eleven")`,
		`WordAdd.eval("one minus two")`,
		`WordAdd.eval("one plus two plus three")`,
		`WordAdd.eval("")`,
	} {
		r := Run(ctx, files, call)
		if r.Status != Error || len(r.Snapshot.Questions) != 0 {
			t.Errorf("%s: expected a grammar error, got %s with questions %+v", call, r.Status, r.Snapshot.Questions)
		}
	}

	// A newly authored word meaning must still obey the number rule's range.
	for _, value := range []string{"100", "-1", `"eleven"`} {
		bad := maps.Clone(files)
		bad["word_values.star"] = strings.Replace(bad["word_values.star"], `value("eleven") == 11`, `value("eleven") == `+value, 1)
		if r := Run(ctx, bad, `NumberWords.parse("eleven")`); r.Status != Error {
			t.Errorf("meaning %s: expected range/type error, got %s (%s)", value, r.Status, r.Message)
		}
	}
}
