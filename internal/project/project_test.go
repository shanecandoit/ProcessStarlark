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
	j := journal{Version: 1, Before: map[string][]byte{"p.yaml": original, "park.html": catalog, "new.yaml": nil}, After: map[string][]byte{"p.yaml": []byte("new sidecar"), "park.html": []byte("new catalog"), "new.yaml": []byte("new")}}
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{journalPath: b, "p.yaml": []byte("new sidecar"), "park.html": catalog, "new.yaml": []byte("new")} {
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
	for name, expected := range map[string][]byte{"p.yaml": original, "park.html": catalog} {
		actual, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(actual) != string(expected) {
			t.Fatalf("%s not restored: %q %v", name, actual, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "new.yaml")); !os.IsNotExist(err) {
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
	j := journal{Version: 1, Before: map[string][]byte{"../outside.yaml": []byte("bad")}, After: map[string][]byte{}}
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
	for name, dep := range map[string]string{"A": "B", "B": "A"} {
		b := "version: 1\nname: " + name + "\nuses: [" + dep + "]\nfunctions: []\n"
		if err := os.WriteFile(filepath.Join(root, name+".yaml"), []byte(b), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadWith(root, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "A.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "B.yaml"), filepath.Join(root, "A.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWith(root, nil); err == nil {
		t.Fatal("symlinked sidecar accepted")
	}
}
