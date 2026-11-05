package samples

import (
	"strings"
	"testing"
)

func TestCanonicalJSON(t *testing.T) {
	a, err := ParseJSON([]byte(`{"z":[null,true,-9223372036854775808],"a":9223372036854775807}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseJSON([]byte(`{"a":9223372036854775807,"z":[null,true,-9223372036854775808]}`))
	if err != nil {
		t.Fatal(err)
	}
	if Key(a) != Key(b) {
		t.Fatal("object order changed canonical key")
	}
	if got := Key(a); got != `{"a":9223372036854775807,"z":[null,true,-9223372036854775808]}` {
		t.Fatal(got)
	}
	for _, input := range []string{`1.0`, `1e2`, `9223372036854775808`, `{"a":1,"a":2}`, `true false`, `[1,]`, strings.Repeat("[", MaxDepth+2) + "0" + strings.Repeat("]", MaxDepth+2)} {
		if _, err := ParseJSON([]byte(input)); err == nil {
			t.Errorf("accepted invalid input %s", input)
		}
	}
}
func TestImmutableCache(t *testing.T) {
	dir := t.TempDir()
	input := map[string]any{"value": int64(4)}
	if err := WriteCache(dir, "P.f", "rev", input, int64(9)); err != nil {
		t.Fatal(err)
	}
	if err := WriteCache(dir, "P.f", "rev", input, int64(10)); err == nil {
		t.Fatal("accepted conflicting cache output")
	}
	out, ok, err := ReadCache(dir, "P.f", "rev", input)
	if err != nil || !ok || out != int64(9) {
		t.Fatalf("%v %v %v", out, ok, err)
	}
	if _, ok, err := ReadCache(dir, "P.f", "new-revision", input); err != nil || ok {
		t.Fatal("old revision satisfied new lookup")
	}
}

func TestCacheAllowsLargeInputOutputPair(t *testing.T) {
	dir := t.TempDir()
	input := strings.Repeat("a", 600_000)
	output := strings.Repeat("b", 600_000)
	if err := WriteCache(dir, "P.f", "revision", input, output); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadCache(dir, "P.f", "revision", input)
	if err != nil || !ok || got != output {
		t.Fatalf("large pair lookup: hit=%v error=%v", ok, err)
	}
}
