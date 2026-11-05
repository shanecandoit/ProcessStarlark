package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"park/internal/samples"
	"strings"
)

// Lock serializes edits and gives readers a coherent project snapshot.
func Lock(root string, exclusive bool) (func(), error) {
	dir, err := SafePath(root, ".park")
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path, err := SafePath(root, ".park/lock")
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	mode := unix.LOCK_SH
	if exclusive {
		mode = unix.LOCK_EX
	}
	if err := unix.Flock(fd, mode|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("project is busy: %w", err)
	}
	return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = f.Close() }, nil
}

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

type journal struct {
	Version int               `json:"version"`
	Before  map[string][]byte `json:"before"`
	After   map[string][]byte `json:"after"`
}

const journalPath = ".park/journal.json"
const MaxJournalBytes = 256 << 20

func Pending(root string) error {
	path, err := SafePath(root, journalPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("unfinished mutation; run park build to recover")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Recover rolls an interrupted mutation back to its complete original snapshot.
// Callers must hold an exclusive project lock.
func Recover(root string) error {
	path, err := SafePath(root, journalPath)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxJournalBytes {
		return fmt.Errorf("invalid or oversized recovery journal")
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) > MaxJournalBytes {
		return fmt.Errorf("recovery journal exceeds size limit")
	}
	var j journal
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&j); err != nil {
		return fmt.Errorf("invalid recovery journal: %w", err)
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("recovery journal has trailing JSON")
	}
	if j.Version != 1 || j.Before == nil || j.After == nil {
		return fmt.Errorf("invalid recovery journal schema")
	}
	for rel := range j.Before {
		if !managedPath(rel) {
			return fmt.Errorf("invalid recovery path %q", rel)
		}
	}
	for _, rel := range Names(j.Before) {
		if err := AtomicWrite(root, rel, j.Before[rel]); err != nil {
			return fmt.Errorf("recovery of %s: %w", rel, err)
		}
	}
	return AtomicWrite(root, journalPath, nil)
}
func managedPath(rel string) bool {
	if rel != filepath.Clean(rel) {
		return false
	}
	return rel == "park.html" || (filepath.Dir(rel) == "." && filepath.Ext(rel) == ".star")
}

// Publish journals all module/catalog edits before atomically replacing each file.
func Publish(root string, edits map[string][]byte) error {
	j := journal{Version: 1, Before: map[string][]byte{}, After: edits}
	for rel, b := range edits {
		if !managedPath(rel) {
			return fmt.Errorf("invalid mutation path %s", rel)
		}
		path, err := SafePath(root, rel)
		if err != nil {
			return err
		}
		if info, statErr := os.Lstat(path); statErr == nil {
			limit := int64(MaxFileBytes)
			if rel == "park.html" {
				limit = 32 << 20
			}
			if !info.Mode().IsRegular() || info.Size() > limit {
				return fmt.Errorf("invalid or oversized mutation target %s", rel)
			}
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		old, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if errors.Is(err, os.ErrNotExist) {
			old = nil
		}
		if bytes.Equal(old, b) {
			continue
		}
		j.Before[rel] = old
	}
	if len(j.Before) == 0 {
		return nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if len(b) > MaxJournalBytes {
		return fmt.Errorf("mutation exceeds recovery journal limit")
	}
	if err := AtomicWrite(root, journalPath, b); err != nil {
		return err
	}
	for _, rel := range Names(j.Before) {
		if err := AtomicWrite(root, rel, edits[rel]); err != nil {
			return fmt.Errorf("publication interrupted (%s); run park build to recover: %w", rel, err)
		}
	}
	return AtomicWrite(root, journalPath, nil)
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
