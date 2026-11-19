package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

func hashFixture(t *testing.T, files map[string]string) map[string]*FunctionVersion {
	t.Helper()
	modules := map[string][]byte{}
	for name, text := range files {
		modules[name] = []byte(text)
	}
	p, err := project.FromFiles("", modules)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ComputeAllFunctionHashes(p)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSyntaxHashPreservesBehaviorAndIgnoresFormatting(t *testing.T) {
	base := `Process Text:
    example: f(1) == "a  b"
    example: f(2) == "a  b"
    def f(n):
        result = "a  b"
        if n > 0:
            result = "positive"
        result = result + "!"
        return result
`
	original := hashFixture(t, map[string]string{"text.star": base})["Text.f"].Hash
	for _, tc := range []struct {
		name, source string
		same         bool
	}{
		{"literal spaces", strings.Replace(base, `result = "a  b"`, `result = "a b"`, 1), false},
		{"block structure", strings.Replace(base, `        result = result + "!"`, `            result = result + "!"`, 1), false},
		{"comment and blank lines", strings.Replace(base, "    def f(n):", "    # comment\n\n    def f(n):", 1), true},
		{"operator spacing", strings.ReplaceAll(base, "n > 0", "n>0"), true},
		{"literal quote spelling", strings.ReplaceAll(base, `"a  b"`, `'a  b'`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := hashFixture(t, map[string]string{"text.star": tc.source})["Text.f"].Hash
			if (got == original) != tc.same {
				t.Fatalf("same hash = %v, want %v", got == original, tc.same)
			}
		})
	}
}

func TestHashResolvedDependencies(t *testing.T) {
	base := depsShipping + `

def alias(country):
    fn = Zones.zone
    result = fn(country)
    return result

def local_alias(country):
    fn = quote
    result = fn(country, 500)
    return result
`
	original := hashFixture(t, map[string]string{"shipping.star": base, "zones.star": depsZones})
	for _, tc := range []struct {
		name, from, to  string
		changed, stable []string
	}{
		{"helper", "return (grams + 499)", "return (grams + 498)", []string{"Shipping.quote", "Shipping.local_alias"}, []string{"Shipping.shadow", "Zones.zone"}},
		{"constant", "STEP = 100", "STEP = 101", []string{"Shipping.quote", "Shipping.local_alias"}, []string{"Shipping.shadow", "Zones.zone"}},
		{"shadowed constant", "BASE = 499", "BASE = 500", []string{"Shipping.quote"}, []string{"Shipping.shadow"}},
		{"unreachable helper", "return UNUSED", "return UNUSED + 1", nil, []string{"Shipping.quote", "Shipping.local_alias"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := hashFixture(t, map[string]string{"shipping.star": strings.Replace(base, tc.from, tc.to, 1), "zones.star": depsZones})
			for _, name := range tc.changed {
				if original[name].Hash == next[name].Hash {
					t.Errorf("%s did not invalidate", name)
				}
			}
			for _, name := range tc.stable {
				if original[name].Hash != next[name].Hash {
					t.Errorf("%s unnecessarily invalidated", name)
				}
			}
		})
	}
	changed := hashFixture(t, map[string]string{"shipping.star": base, "zones.star": strings.Replace(depsZones, `== "domestic"`, `== "overseas"`, 1)})
	for _, name := range []string{"Shipping.alias", "Shipping.local_alias", "Shipping.dynamic", "Shipping.quote"} {
		if original[name].Hash == changed[name].Hash {
			t.Errorf("alias/namespace failed to invalidate %s", name)
		}
	}
	// A direct member only depends on that member, not every function in Zones.
	unrelated := hashFixture(t, map[string]string{"shipping.star": base, "zones.star": strings.Replace(depsZones, `result = "x"`, `result = "y"`, 1)})
	if original["Shipping.quote"].Hash != unrelated["Shipping.quote"].Hash {
		t.Fatal("unrelated namespace member invalidated direct call")
	}
}

func TestMutualRecursionVersions(t *testing.T) {
	src := `Process Cycle:
    def a(n):
        result = 0
        if n > 0:
            result = b(n - 1)
        return result
    def b(n):
        result = 0
        if n > 0:
            result = a(n - 1)
        return result
    def unrelated(n):
        result = n
        return result
`
	before := hashFixture(t, map[string]string{"cycle.star": src})
	after := hashFixture(t, map[string]string{"cycle.star": strings.Replace(src, "result = b(n - 1)", "result = b(n - 2)", 1)})
	for _, name := range []string{"Cycle.a", "Cycle.b"} {
		if before[name].Hash == after[name].Hash {
			t.Errorf("recursive group failed to invalidate %s", name)
		}
	}
	if before["Cycle.unrelated"].Hash != after["Cycle.unrelated"].Hash {
		t.Fatal("unrelated recursive group invalidation")
	}
}

func TestDiskCacheDoesNotReuseChangedLiteralOrHelper(t *testing.T) {
	dir := t.TempDir()
	src := `example: text(1) == "a  b"
example: text(2) == "a  b"

def text(n):
    result = _text(n)
    return result

def _text(n):
    return "a  b"
`
	run := func(source string) (string, *Engine) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "text.star"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		p, err := project.LoadWith(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		e, err := New(p)
		if err != nil {
			t.Fatal(err)
		}
		out, err := e.Run(context.Background(), "Text.text", map[string]any{"n": int64(3)})
		if err != nil {
			t.Fatal(err)
		}
		return out.(string), e
	}
	out, e := run(src)
	if out != "a  b" {
		t.Fatal(out)
	}
	cached, ok, err := e.Cache.Get("Text.text", e.FunctionHashes["Text.text"], map[string]any{"n": int64(3)})
	if err != nil || !ok || cached != out {
		t.Fatal("cache not populated", cached, ok, err)
	}
	out, _ = run(strings.Replace(src, `return "a  b"`, `return "a b"`, 1))
	if out != "a b" {
		t.Fatal("stale literal result reused", out)
	}
	// Tests must execute the implementation even though examples and cached values exist.
	p, err := project.LoadWith(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	entriesBefore, _ := os.ReadDir(filepath.Join(dir, ".cache"))
	report, err := Check(context.Background(), p, "")
	if err != nil || report.Failures != 2 {
		t.Fatal("checks bypassed body", report, err)
	}
	entriesAfter, _ := os.ReadDir(filepath.Join(dir, ".cache"))
	if len(entriesBefore) != len(entriesAfter) {
		t.Fatal("checks wrote cache")
	}
}

func TestLegacyProjectCacheCannotSatisfyFunctionGraph(t *testing.T) {
	// A legacy project-wide cache record cannot satisfy the new graph key.
	src := `Process A:
    example: f(1) == 2
    example: f(2) == 3
    def f(n):
        result = n + 1
        return result
`
	dir := t.TempDir()
	p, err := project.FromFiles(dir, map[string][]byte{"a.star": []byte(src)})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"n": int64(3)}
	if err := samples.WriteCache(filepath.Join(dir, ".cache"), "A.f", p.Revision, input, int64(999)); err != nil {
		t.Fatal(err)
	}
	e, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.Run(context.Background(), "A.f", input)
	if err != nil || out != int64(4) {
		t.Fatal(out, err)
	}
}

func TestRuntimeIdentityTracksPinnedInterpreter(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	for i, name := range fields {
		if name == "go.starlark.net" && i+1 < len(fields) {
			parts := strings.Split(fields[i+1], "-")
			commit := parts[len(parts)-1]
			if !strings.Contains(RuntimeIdentity, "starlark-"+commit) {
				t.Fatalf("update RuntimeIdentity for interpreter %s", fields[i+1])
			}
			return
		}
	}
	t.Fatal("pinned interpreter missing from go.mod")
}
