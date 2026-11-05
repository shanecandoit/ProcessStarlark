package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"park/internal/project"
	"park/internal/samples"
)

const mathModule = `Process Math:
    example: add(1, 2) == 12
    example: add(2, 2) == 22

    def add(left, right):
        result = left * 10 + right
        return result
`

// load writes Math plus a Caller process whose call(value) invokes Math.add
// with the given Starlark argument list.
func load(t *testing.T, callArgs string) (*Engine, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"math.star": mathModule,
		"caller.star": "Process Caller:\n    example: call(100) == 0\n    example: call(200) == 0\n\n" +
			"    def call(value):\n        result = Math.add(" + callArgs + ")\n        return result\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	return e, root
}

func run(t *testing.T, e *Engine, qualified string, input map[string]any) (any, error) {
	t.Helper()
	return e.Run(context.Background(), qualified, input)
}

func TestBodyReceivesNamedInputsInOrder(t *testing.T) {
	e, _ := load(t, "value, 1")
	out, err := run(t, e, "Math.add", map[string]any{"right": int64(7), "left": int64(5)})
	if err != nil || out != int64(57) {
		t.Fatalf("got %v %v; inputs were not passed by name", out, err)
	}
	// Authored examples still answer before the body.
	if out, _ := run(t, e, "Math.add", map[string]any{"left": int64(1), "right": int64(2)}); out != int64(12) {
		t.Fatalf("example did not win: %v", out)
	}
}

func TestStarlarkCallArguments(t *testing.T) {
	cases := map[string]struct{ args, want string }{
		"positional":         {"value, 1", ""},
		"keywords":           {"right=1, left=value", ""},
		"mixed":              {"value, right=1", ""},
		"missing keyword":    {"left=value", "missing input right"},
		"extra keyword":      {"left=value, right=1, carry=0", "unknown input carry"},
		"duplicate":          {"value, left=1", "given more than once"},
		"too many":           {"value, 1, 2", "takes 2 inputs"},
		"too few positional": {"value", "missing input right"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e, _ := load(t, tc.args)
			e.Testing = true // keep the call out of the cache
			out, err := run(t, e, "Caller.call", map[string]any{"value": int64(9)})
			if tc.want == "" {
				if err != nil || out != int64(91) {
					t.Fatalf("got %v %v", out, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPositionalAndNamedCallsShareCacheKey(t *testing.T) {
	_, f, _ := func() (*project.Process, *project.Function, error) {
		e, _ := load(t, "value, 1")
		return e.Project.Function("Math.add")
	}()
	positional, err := bindArgs("Math.add", f, starlark.Tuple{starlark.MakeInt(9), starlark.MakeInt(1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	named, err := bindArgs("Math.add", f, nil, []starlark.Tuple{{starlark.String("right"), starlark.MakeInt(1)}, {starlark.String("left"), starlark.MakeInt(9)}})
	if err != nil {
		t.Fatal(err)
	}
	if samples.Key(positional) != samples.Key(named) {
		t.Fatalf("keys differ: %s vs %s", samples.Key(positional), samples.Key(named))
	}

	// End to end: a positional call from Starlark writes the cache entry, and a
	// named call from the host reads it back without running the body.
	e, root := load(t, "value, 1")
	if _, err := run(t, e, "Caller.call", map[string]any{"value": int64(9)}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".cache")); len(entries) != 2 { // Caller.call and Math.add
		t.Fatalf("expected 2 cache entries, got %d", len(entries))
	}
	delete(e.bodies, "Math.add")
	if out, err := run(t, e, "Math.add", named); err != nil || out != int64(91) {
		t.Fatalf("named call missed the positional cache entry: %v %v", out, err)
	}
}

func TestExampleOnlyFunctionsDispatchToExamples(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "words.star"), []byte("example: plural(\"ox\", 2) == \"oxen\"\n\ndef plural(noun, count):\n    \"\"\"Irregular plurals.\"\"\"\n    pass\n\ndef _helper(noun):\n    return plural(noun, 2)\n"), 0644)
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := run(t, e, "Words.plural", map[string]any{"noun": "ox", "count": int64(2)}); err != nil || out != "oxen" {
		t.Fatalf("got %v %v", out, err)
	}
	_, err = run(t, e, "Words.plural", map[string]any{"count": int64(2), "noun": "cat"})
	if err == nil || err.Error() != `unresolved call Words.plural(noun="cat", count=2)` {
		t.Fatalf("got %v", err)
	}
}
