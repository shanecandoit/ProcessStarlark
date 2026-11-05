package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePathsAndLock(t *testing.T) {
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
	unlock, err := Lock(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := Lock(root, false); err == nil {
		second()
		t.Fatal("reader entered during mutation")
	}
	unlock()
	reader, err := Lock(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer reader()
	second, err := Lock(root, false)
	if err != nil {
		t.Fatal("shared reads should coexist", err)
	}
	second()
}
func TestRecoveryRestoresCompleteSnapshot(t *testing.T) {
	root := t.TempDir()
	unlock, err := Lock(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	original := []byte("original sidecar\n")
	catalog := []byte("original catalog\n")
	j := journal{Version: 1, Before: map[string][]byte{"p.star": original, "park.html": catalog, "new.star": nil}, After: map[string][]byte{"p.star": []byte("new sidecar"), "park.html": []byte("new catalog"), "new.star": []byte("new")}}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{journalPath: b, "p.star": []byte("new sidecar"), "park.html": catalog, "new.star": []byte("new")} {
		if err := AtomicWrite(root, name, contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := Pending(root); err == nil {
		t.Fatal("reader did not detect unfinished mutation")
	}
	if err := Recover(root); err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string][]byte{"p.star": original, "park.html": catalog} {
		actual, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(actual) != string(expected) {
			t.Fatalf("%s not restored: %q %v", name, actual, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "new.star")); !os.IsNotExist(err) {
		t.Fatal("new file was not rolled back")
	}
	if err := Pending(root); err != nil {
		t.Fatal(err)
	}
}
func TestInvalidJournalDoesNotTouchFiles(t *testing.T) {
	root := t.TempDir()
	unlock, err := Lock(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	j := journal{Version: 1, Before: map[string][]byte{"../outside.star": []byte("bad")}, After: map[string][]byte{}}
	b, _ := json.Marshal(j)
	if err := AtomicWrite(root, journalPath, b); err != nil {
		t.Fatal(err)
	}
	if err := Recover(root); err == nil {
		t.Fatal("accepted invalid recovery path")
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
