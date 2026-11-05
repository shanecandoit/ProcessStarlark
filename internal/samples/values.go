// Package samples implements Park's value codec and authored example format.
package samples

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"go.yaml.in/yaml/v3"
)

const MaxValueBytes = 1 << 20
const MaxDepth = 64

type Pair struct{ Input, Output any }

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

func ParseYAML(b []byte) (*yaml.Node, error) {
	d := yaml.NewDecoder(bytes.NewReader(b))
	var n yaml.Node
	if err := d.Decode(&n); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected exactly one YAML document")
	}
	if len(n.Content) != 1 {
		return nil, fmt.Errorf("expected a YAML document")
	}
	if err := validateNode(n.Content[0], 0); err != nil {
		return nil, err
	}
	return &n, nil
}

func At(n *yaml.Node, format string, args ...any) error {
	return fmt.Errorf("line %d, column %d: %s", n.Line, n.Column, fmt.Sprintf(format, args...))
}

func validateNode(n *yaml.Node, depth int) error {
	if depth > MaxDepth {
		return At(n, "value nesting exceeds %d", MaxDepth)
	}
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return At(n, "YAML anchors and aliases are unsupported")
	}
	switch n.Tag {
	case "!!map", "!!seq", "!!str", "!!null", "!!bool", "!!int":
	default:
		return At(n, "unsupported YAML tag %q", n.Tag)
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" {
				return At(k, "map keys must be strings")
			}
			if seen[k.Value] {
				return At(k, "duplicate key %q", k.Value)
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := validateNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func Fields(n *yaml.Node, allowed ...string) (map[string]*yaml.Node, error) {
	if n.Kind != yaml.MappingNode {
		return nil, At(n, "expected mapping")
	}
	valid := map[string]bool{}
	for _, k := range allowed {
		valid[k] = true
	}
	fields := map[string]*yaml.Node{}
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		if !valid[k.Value] {
			return nil, At(k, "unknown field %q", k.Value)
		}
		fields[k.Value] = n.Content[i+1]
	}
	return fields, nil
}

func Text(n *yaml.Node) (string, error) {
	if n == nil {
		return "", fmt.Errorf("missing required string field")
	}
	if n.Tag != "!!str" {
		return "", At(n, "expected string")
	}
	return n.Value, nil
}

func YAMLValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i < len(n.Content); i += 2 {
			v, err := YAMLValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[n.Content[i].Value] = v
		}
		if _, err := Canonical(m); err != nil {
			return nil, At(n, "%v", err)
		}
		return m, nil
	case yaml.SequenceNode:
		a := []any{}
		for _, child := range n.Content {
			v, err := YAMLValue(child)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		if _, err := Canonical(a); err != nil {
			return nil, At(n, "%v", err)
		}
		return a, nil
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!null":
			return nil, nil
		case "!!str":
			if len(n.Value) > MaxValueBytes {
				return nil, At(n, "string too large")
			}
			return n.Value, nil
		case "!!bool":
			var v bool
			err := n.Decode(&v)
			return v, err
		case "!!int":
			var v int64
			err := n.Decode(&v)
			if err != nil {
				return nil, At(n, "expected signed 64-bit integer")
			}
			return v, nil
		}
	}
	return nil, At(n, "unsupported value")
}

func EncodeYAML(n *yaml.Node) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func ValueNode(v any) *yaml.Node { var n yaml.Node; _ = n.Encode(v); return &n }
