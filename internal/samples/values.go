// Package samples implements Park's value codec: canonical JSON keys, JSON
// parsing, and conversion to and from Starlark values.
package samples

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

const MaxValueBytes = 1 << 20
const MaxDepth = 64

// Canonical encodes the supported JSON subset with sorted object keys.
func Canonical(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxValueBytes {
		return nil, fmt.Errorf("value exceeds %d bytes", MaxValueBytes)
	}
	return b, nil
}

func Key(v any) string { b, _ := Canonical(v); return string(b) }

func ParseJSON(b []byte) (any, error) {
	if len(b) > MaxValueBytes {
		return nil, fmt.Errorf("JSON value exceeds %d bytes", MaxValueBytes)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	v, err := jsonValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("expected exactly one JSON value")
	}
	return v, nil
}

func jsonValue(d *json.Decoder, depth int) (any, error) {
	if depth > MaxDepth {
		return nil, fmt.Errorf("value nesting exceeds %d", MaxDepth)
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := t.(type) {
	case nil, bool, string:
		return t, nil
	case json.Number:
		n, err := strconv.ParseInt(string(t), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: expected signed 64-bit integer (floats are unsupported)", t)
		}
		return n, nil
	case json.Delim:
		switch t {
		case '[':
			a := []any{}
			for d.More() {
				v, err := jsonValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				a = append(a, v)
			}
			_, err := d.Token()
			return a, err
		case '{':
			m := map[string]any{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				s, ok := k.(string)
				if !ok {
					return nil, fmt.Errorf("expected string object key")
				}
				if _, ok := m[s]; ok {
					return nil, fmt.Errorf("duplicate JSON key %q", s)
				}
				v, err := jsonValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				m[s] = v
			}
			_, err := d.Token()
			return m, err
		}
	}
	return nil, fmt.Errorf("unexpected JSON token %v", t)
}
