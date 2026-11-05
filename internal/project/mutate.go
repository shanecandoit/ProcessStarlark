package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"park/internal/samples"
)

func AtomicWrite(root, rel string, b []byte) error {
	path, err := SafePath(root, rel)
	if err != nil {
		return err
	}
	if b == nil {
		err = os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return syncDirectory(filepath.Dir(path))
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".park-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0644); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if _, err := SafePath(root, rel); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func managedPath(rel string) bool {
	if rel != filepath.Clean(rel) {
		return false
	}
	return rel == "park.html" || rel == QuestionsFile || (filepath.Dir(rel) == "." && filepath.Ext(rel) == ".star")
}

// Publish writes module and question edits, then the catalog, each with an
// atomic rename. A nil edit removes the file.
// park.html is derived from the modules, so if publishing stops partway the
// modules are authoritative and `park build` regenerates the catalog.
func Publish(root string, edits map[string][]byte) error {
	var modules []string
	for rel := range edits {
		if !managedPath(rel) {
			return fmt.Errorf("invalid edit path %s", rel)
		}
		if rel != "park.html" {
			modules = append(modules, rel)
		}
	}
	sort.Strings(modules)
	for _, rel := range modules {
		if err := AtomicWrite(root, rel, edits[rel]); err != nil {
			return err
		}
	}
	if html, ok := edits["park.html"]; ok {
		if err := AtomicWrite(root, "park.html", html); err != nil {
			return fmt.Errorf("saved %s but park.html is stale; run park build: %w", strings.Join(modules, ", "), err)
		}
	}
	return nil
}

// ExampleLine renders an example as written in the target function's own
// module, with positional arguments in signature order.
func ExampleLine(f *Function, input map[string]any, output any) string {
	args := make([]string, len(f.Inputs))
	for i, name := range f.Inputs {
		args[i] = samples.Literal(input[name])
	}
	return "example: " + f.Name + "(" + strings.Join(args, ", ") + ") == " + samples.Literal(output)
}

// AddExample writes a new example line into the function's own module: after
// the module's last example, else after the process docstring or header, else
// at the top of the file. It returns the module path and its new contents.
func AddExample(p *Project, qualified string, raw map[string]any, output any) (string, []byte, error) {
	proc, f, err := p.Function(qualified)
	if err != nil {
		return "", nil, err
	}
	input, err := f.BindInput(raw)
	if err != nil {
		return "", nil, err
	}
	if _, err := samples.Canonical(output); err != nil {
		return "", nil, err
	}
	if _, ok := f.Index[samples.Key(input)]; ok {
		return "", nil, fmt.Errorf("duplicate example for %s", FormatCall(qualified, f.Inputs, input))
	}
	path := f.Path
	lines := strings.Split(string(p.Files[path]), "\n")
	text := proc.Indent + ExampleLine(f, input, output)
	var insert []string
	at := 0
	switch {
	case proc.ExampleEnd > 0:
		at, insert = proc.ExampleEnd, []string{text}
	case proc.DocEnd > 0:
		at, insert = proc.DocEnd, []string{"", text}
	case proc.Header > 0:
		at, insert = proc.Header, []string{text, ""}
	default:
		at, insert = 0, []string{text, ""}
	}
	lines = append(lines[:at], append(insert, lines[at:]...)...)
	return path, []byte(strings.Join(lines, "\n")), nil
}

// RemoveExample deletes the lines of the example for input, wherever it was written.
func RemoveExample(p *Project, qualified string, raw map[string]any) (string, []byte, error) {
	_, f, err := p.Function(qualified)
	if err != nil {
		return "", nil, err
	}
	input, err := f.BindInput(raw)
	if err != nil {
		return "", nil, err
	}
	key := samples.Key(input)
	for _, ex := range f.Examples {
		if samples.Key(ex.Input) == key {
			lines := strings.Split(string(p.Files[ex.Path]), "\n")
			lines = append(lines[:ex.Line-1], lines[ex.EndLine:]...)
			return ex.Path, []byte(strings.Join(lines, "\n")), nil
		}
	}
	return "", nil, fmt.Errorf("no authored example for %s", FormatCall(qualified, f.Inputs, input))
}
