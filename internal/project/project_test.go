package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePaths(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../file", "/absolute", "escape/new/file"} {
		if _, err := SafePath(root, path); err == nil {
			t.Errorf("accepted escaping path %q", path)
		}
	}
}

func TestPublishWritesOnlyModulesAndCatalog(t *testing.T) {
	root := t.TempDir()
	if err := Publish(root, map[string][]byte{"p.star": []byte("def f(x):\n    pass\n"), "park.html": []byte("catalog")}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"p.star": "def f(x):\n    pass\n", "park.html": "catalog"} {
		if b, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(b) != want {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
	for _, bad := range []string{"../outside.star", "notes.txt", "sub/p.star"} {
		if err := Publish(root, map[string][]byte{bad: []byte("x")}); err == nil {
			t.Errorf("accepted edit to %s", bad)
		}
	}
}

func TestCyclesAndSymlinkedFiles(t *testing.T) {
	root := t.TempDir()
	for name, dep := range map[string]string{"a": "B", "b": "A"} {
		b := "def f(x):\n    result = " + dep + ".f(x)\n    return result\n"
		if err := os.WriteFile(filepath.Join(root, name+".star"), []byte(b), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadWith(root, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "a.star")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "b.star"), filepath.Join(root, "a.star")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWith(root, nil); err == nil {
		t.Fatal("symlinked module accepted")
	}
}

func load(t *testing.T, files map[string]string) (*Project, error) {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return LoadWith(root, nil)
}

func TestSignaturesComeFromSource(t *testing.T) {
	p, err := load(t, map[string]string{"math.star": `Process Math:
    def add(left, right):
        """Add two integers."""
        if left < 0:
            total = 0
            return total
        total = left + right
        return total

    def label(code):
        """Example-only."""
        pass

    def described(code):
        """A docstring alone also declares an example-only function."""

    def _private(x):
        return x + 1
`})
	if err != nil {
		t.Fatal(err)
	}
	fns := p.Processes["Math"].Functions
	if len(fns) != 3 {
		t.Fatalf("private helper exported or function missing: %v", Names(fns))
	}
	if fns["add"].Signature != "add(left, right) total" || !fns["add"].Implemented || fns["add"].Description != "Add two integers." {
		t.Fatalf("add: %+v", fns["add"])
	}
	for _, name := range []string{"label", "described"} {
		if fns[name].Implemented || fns[name].Signature != name+"(code) result" {
			t.Fatalf("%s: %+v", name, fns[name])
		}
	}
}

func TestReturnRules(t *testing.T) {
	for body, want := range map[string]string{
		"    return x + 1\n": "named variable",
		"    return\n":       "named variable",
		"    y = x\n":        "must end with `return <name>`",
		"    if x:\n        a = 1\n        return a\n    b = 2\n    return b\n": "returns both a and b",
		"    return x\n": "returns its input x",
	} {
		_, err := load(t, map[string]string{"m.star": "def f(x):\n" + body})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want %q, got %v", body, want, err)
		}
	}
	// Returns inside nested functions belong to those functions.
	if _, err := load(t, map[string]string{"m.star": "def f(x):\n    g = lambda y: y + 1\n    result = g(x)\n    return result\n"}); err != nil {
		t.Fatal(err)
	}
}

func TestBindInput(t *testing.T) {
	f := &Function{Name: "add", Inputs: []string{"left", "right"}}
	if _, err := f.BindInput(map[string]any{"right": int64(2), "left": int64(1)}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []any{int64(1), map[string]any{"left": int64(1)}, map[string]any{"left": int64(1), "right": int64(2), "carry": int64(0)}} {
		if _, err := f.BindInput(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}
