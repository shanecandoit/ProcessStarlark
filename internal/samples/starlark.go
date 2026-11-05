package samples

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
)

// ToStar converts a host value to a Starlark value. Map keys are inserted in
// sorted order so printed dictionaries are deterministic.
func ToStar(v any) starlark.Value {
	switch x := v.(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(x)
	case string:
		return starlark.String(x)
	case int64:
		return starlark.MakeInt64(x)
	case []any:
		a := make([]starlark.Value, len(x))
		for i, v := range x {
			a[i] = ToStar(v)
		}
		return starlark.NewList(a)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		d := starlark.NewDict(len(x))
		for _, k := range keys {
			_ = d.SetKey(starlark.String(k), ToStar(x[k]))
		}
		return d
	default:
		panic(fmt.Sprintf("unsupported host value %T", v))
	}
}

// Literal renders a value as Starlark source, e.g. {"a": [1, None]}.
func Literal(v any) string { return ToStar(v).String() }

// FromStar converts a Starlark value to a host value in the supported subset.
func FromStar(v starlark.Value, depth int) (any, error) {
	if depth > MaxDepth {
		return nil, fmt.Errorf("value nesting exceeds %d", MaxDepth)
	}
	var out any
	switch x := v.(type) {
	case starlark.NoneType:
		out = nil
	case starlark.Bool:
		out = bool(x)
	case starlark.String:
		out = string(x)
	case starlark.Int:
		n, ok := x.Int64()
		if !ok {
			return nil, fmt.Errorf("integer outside signed 64-bit range")
		}
		out = n
	case *starlark.List:
		if x.Len() > MaxValueBytes/2 {
			return nil, fmt.Errorf("list exceeds size limit")
		}
		a := make([]any, x.Len())
		for i := range a {
			v, err := FromStar(x.Index(i), depth+1)
			if err != nil {
				return nil, err
			}
			a[i] = v
		}
		out = a
	case starlark.Tuple:
		a := make([]any, len(x))
		for i := range x {
			v, err := FromStar(x[i], depth+1)
			if err != nil {
				return nil, err
			}
			a[i] = v
		}
		out = a
	case *starlark.Dict:
		if x.Len() > MaxValueBytes/4 {
			return nil, fmt.Errorf("object exceeds size limit")
		}
		m := map[string]any{}
		for _, item := range x.Items() {
			k, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("map keys must be strings")
			}
			v, err := FromStar(item[1], depth+1)
			if err != nil {
				return nil, err
			}
			m[string(k)] = v
		}
		out = m
	default:
		return nil, fmt.Errorf("unsupported Starlark value %s", v.Type())
	}
	if _, err := Canonical(out); err != nil {
		return nil, err
	}
	return out, nil
}
