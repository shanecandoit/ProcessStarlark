package project

import "testing"

func TestCellTypes(t *testing.T) {
	strs := []any{"US", "CA"}
	ints := []any{int64(1), int64(2)}
	cases := []struct {
		text   string
		column []any
		want   any
	}{
		{"US", strs, "US"},
		{`"US"`, strs, "US"},
		{`" Germany "`, strs, " Germany "},
		{"02134", strs, "02134"},
		{"42", strs, "42"},
		{"42", ints, int64(42)},
		{"42", nil, int64(42)},
		{"usa", nil, "usa"},
		{"True", nil, true},
	}
	for _, c := range cases {
		got, err := ParseCell(c.text, c.column)
		if err != nil || got != c.want {
			t.Errorf("ParseCell(%q) = %#v, %v; want %#v", c.text, got, err, c.want)
		}
	}
	if _, err := ParseCell("abc", ints); err == nil {
		t.Error("text accepted in a number column")
	}
	if _, err := ParseCell("  ", strs); err == nil {
		t.Error("empty cell accepted")
	}
}
