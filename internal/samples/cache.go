package samples

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Record struct {
	Version    int             `json:"version"`
	Function   string          `json:"function"`
	Input      json.RawMessage `json:"input"`
	Output     json.RawMessage `json:"output"`
	Revision   string          `json:"revision"`
	Provenance string          `json:"provenance"`
}

func CacheName(function, revision string, input any) string {
	sum := sha256.Sum256([]byte(function + "\x00" + revision + "\x00" + Key(input)))
	return fmt.Sprintf("%x.json", sum)
}
func ReadCache(dir, function, revision string, input any) (any, bool, error) {
	path := filepath.Join(dir, CacheName(function, revision, input))
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4*MaxValueBytes {
		return nil, false, fmt.Errorf("invalid cache file %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var r Record
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return nil, false, fmt.Errorf("invalid cache record: %w; delete .cache to rebuild", err)
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, false, fmt.Errorf("cache contains trailing JSON")
	}
	if r.Version != 1 || r.Function != function || r.Revision != revision || !bytes.Equal(r.Input, []byte(Key(input))) || r.Provenance != "body" {
		return nil, false, fmt.Errorf("cache key/provenance mismatch; delete .cache to rebuild")
	}
	value, err := ParseJSON(r.Output)
	return value, err == nil, err
}
func WriteCache(dir, function, revision string, input, output any) error {
	if old, ok, err := ReadCache(dir, function, revision, input); err != nil {
		return err
	} else if ok {
		if Key(old) != Key(output) {
			return fmt.Errorf("conflicting cached output for %s", function)
		}
		return nil
	}
	in, err := Canonical(input)
	if err != nil {
		return err
	}
	out, err := Canonical(output)
	if err != nil {
		return err
	}
	r := Record{Version: 1, Function: function, Input: in, Output: out, Revision: revision, Provenance: "body"}
	// A map gives the record itself canonical field order; RawMessage preserves
	// the already canonical input/output values without float64 conversion.
	b, err := json.Marshal(map[string]any{
		"version": r.Version, "function": r.Function, "input": r.Input, "output": r.Output,
		"revision": r.Revision, "provenance": r.Provenance,
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cache-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	path := filepath.Join(dir, CacheName(function, revision, input))
	// Hard-link publication never replaces a concurrently published immutable record.
	if err := os.Link(tmp, path); os.IsExist(err) {
		old, ok, e := ReadCache(dir, function, revision, input)
		if e != nil {
			return e
		}
		if !ok || Key(old) != Key(output) {
			return fmt.Errorf("conflicting concurrent cache result")
		}
		return nil
	} else {
		return err
	}
}
