package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"park/internal/project"
)

func write(t *testing.T, root, path, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, path), []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	entries, err := os.ReadDir("../../fixtures/roman")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".star") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("../../fixtures/roman", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, entry.Name(), string(b))
	}
	return root
}
func invoke(t *testing.T, root string, terminal bool, input string, args ...string) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	c := &CLI{In: strings.NewReader(input), Out: &out, Err: &err, Terminal: terminal}
	code := c.Run(context.Background(), append([]string{"--root", root}, args...))
	return code, out.String(), err.String()
}
func success(t *testing.T, root string, args ...string) string {
	t.Helper()
	code, out, err := invoke(t, root, false, "", args...)
	if code != 0 {
		t.Fatalf("%v => %d: %s", args, code, err)
	}
	return out
}
func files(t *testing.T, root, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, dir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
func read(t *testing.T, root, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDefaultRootStaysInCurrentDirectory(t *testing.T) {
	parent := fixture(t)
	write(t, parent, "training.yaml", "learning_rate: 0.0002\n")
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	var out, errout bytes.Buffer
	c := &CLI{In: strings.NewReader(""), Out: &out, Err: &errout}
	if code := c.Run(context.Background(), []string{"test"}); code != 2 || !strings.Contains(errout.String(), "no Park project found in current directory") {
		t.Fatalf("expected local project error, got %d: %s", code, errout.String())
	}
	if _, err := os.Stat(filepath.Join(parent, ".park")); !os.IsNotExist(err) {
		t.Fatalf("default discovery touched parent project state: %v", err)
	}
	if err := os.Remove(filepath.Join(parent, "training.yaml")); err != nil {
		t.Fatal(err)
	}
	// A parent project is accessible when explicitly selected.
	success(t, parent, "test")
	t.Chdir(parent)
	out.Reset()
	errout.Reset()
	if code := c.Run(context.Background(), []string{"test"}); code != 0 || !strings.Contains(out.String(), "0 failures") {
		t.Fatalf("current-directory project failed: %d: %s", code, errout.String())
	}
}

func TestRunCacheAndBodyVerification(t *testing.T) {
	root := fixture(t)
	success(t, root, "build")
	html := read(t, root, "park.html")
	success(t, root, "build")
	if html != read(t, root, "park.html") {
		t.Fatal("build is nondeterministic")
	}
	out := success(t, root, "test")
	if !strings.Contains(out, "4 implementation checks, 2 example-only records, 0 failures") {
		t.Fatal(out)
	}
	if len(files(t, root, ".cache")) != 0 {
		t.Fatal("tests persisted cache results")
	}
	if out := success(t, root, "run", "--input", `"iv + i"`, "RomanAdd.eval"); out != "5\n" {
		t.Fatal(out)
	}
	entries := files(t, root, ".cache")
	if len(entries) != 3 {
		t.Fatalf("expected 3 cached body evaluations, got %d", len(entries))
	}
	cached := map[string]string{}
	for _, entry := range entries {
		cached[entry.Name()] = read(t, root, filepath.Join(".cache", entry.Name()))
	}
	success(t, root, "run", "--input", `"iv + i"`, "RomanAdd.eval")
	for name, contents := range cached {
		if read(t, root, filepath.Join(".cache", name)) != contents {
			t.Fatal("cache entry changed on hit")
		}
	}
	if read(t, root, "park.html") != html {
		t.Fatal("runtime cache regenerated catalog")
	}
	source := read(t, root, "roman_parse.star")
	write(t, root, "roman_parse.star", strings.Replace(source, "return total", "return 0", 1))
	code, _, err := invoke(t, root, false, "", "test", "RomanParse.parse")
	if code != 1 || !strings.Contains(err, "expected 9") {
		t.Fatalf("bad body escaped test: %d %s", code, err)
	}
	if out := success(t, root, "run", "--input", `"iv + i"`, "RomanAdd.eval"); out != "0\n" {
		t.Fatal("code change did not invalidate cached result:", out)
	}
}
func TestTeachingAndExampleEdits(t *testing.T) {
	root := fixture(t)
	success(t, root, "build")
	code, out, err := invoke(t, root, false, `["iv","+","i"]`+"\n", "run", "--interactive", "--input", `"iv + i"`, "RomanParse.tokenize")
	if code != 3 || out != "" || !strings.Contains(err, "unresolved call") {
		t.Fatalf("unattended call %d %s %s", code, out, err)
	}
	code, out, err = invoke(t, root, true, `["iv","+","i"]`+"\n", "run", "--interactive", "--input", `"iv + i"`, "RomanParse.tokenize")
	if code != 0 || out != "[\"iv\",\"+\",\"i\"]\n" {
		t.Fatalf("teaching %d %s %s", code, out, err)
	}
	if !strings.Contains(read(t, root, "roman_parse.yaml"), "iv + i") {
		t.Fatal("taught pair was not saved")
	}
	success(t, root, "example", "remove", "--input", `"iv + i"`, "RomanParse.tokenize")
	code, _, _ = invoke(t, root, false, "", "run", "--input", `"iv + i"`, "RomanParse.tokenize")
	if code != 3 {
		t.Fatal(code)
	}
	success(t, root, "example", "remove", "--input", `"vi"`, "RomanParse.parse")
	if !strings.Contains(read(t, root, "park.html"), "needs examples") {
		t.Fatal("catalog did not reflect readiness")
	}
	code, _, _ = invoke(t, root, false, "", "run", "--input", `"iv"`, "RomanParse.parse")
	if code != 1 {
		t.Fatal("unready body executed", code)
	}
	if success(t, root, "run", "--input", `"ix"`, "RomanParse.parse") != "9\n" {
		t.Fatal("known example was gated")
	}
}
func TestRegistrationPreservesExamplesAndComments(t *testing.T) {
	root := fixture(t)
	source := read(t, root, "roman_parse.yaml")
	write(t, root, "roman_parse.yaml", strings.Replace(source, `- {in: "ix", out: 9}`, `- {in: "ix", out: 9} # keep this comment`, 1))
	success(t, root, "register", "roman_parse.star")
	contents := read(t, root, "roman_parse.yaml")
	if !strings.Contains(contents, "# keep this comment") {
		t.Fatal("registration discarded comments")
	}
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Processes["RomanParse"].Functions["parse"].Examples) != 2 || len(p.Processes["RomanParse"].Functions["tokenize"].Examples) != 2 {
		t.Fatal("registration discarded authored examples")
	}
	success(t, root, "register", "roman_parse.star")
	if read(t, root, "roman_parse.yaml") != contents {
		t.Fatal("registration is not idempotent")
	}
}
func TestInitCRUD(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "double.star", "\"\"\"Double an integer.\"\"\"\ndef double(value):\n    return value * 2\n")
	success(t, root, "register", "double.star")
	if !strings.Contains(success(t, root, "list"), "needs examples") {
		t.Fatal("registration should need examples")
	}
	for _, n := range []int{1, 2} {
		success(t, root, "example", "add", "--input", fmt.Sprint(n), "--output", fmt.Sprint(n*2), "Double.double")
	}
	if success(t, root, "run", "--input", "3", "Double.double") != "6\n" {
		t.Fatal("wrong result")
	}
	success(t, root, "function", "add", "--signature", "label(in) out", "Double.label")
	success(t, root, "example", "add", "--input", "null", "--output", `{"ok":true}`, "Double.label")
	if success(t, root, "run", "--input", "null", "Double.label") != "{\"ok\":true}\n" {
		t.Fatal("null/object lookup failed")
	}
	success(t, root, "function", "remove", "Double.label")
	success(t, root, "process", "add", "--name", "Names")
	success(t, root, "process", "remove", "Names")
	success(t, root, "process", "remove", "Double")
	if _, err := os.Stat(filepath.Join(root, "double.star")); err != nil {
		t.Fatal("removal deleted source")
	}
	success(t, root, "build")
	if strings.Contains(read(t, root, "park.html"), "Double") {
		t.Fatal("removed registration reappeared")
	}
	code, _, _ := invoke(t, root, false, "", "init")
	if code != 2 {
		t.Fatal("init overwrote existing project")
	}
}
func TestValidationHasNoSourceWrites(t *testing.T) {
	cases := []struct{ name, old, new string }{
		{"duplicate inputs", `- {in: "vi", out: 6}`, `- {in: "ix", out: 6}`},
		{"unknown field", "version: 1", "version: 1\nunexpected: true"},
		{"float", `out: 9`, `out: 9.5`},
		{"missing pair field", `{in: "ix", out: 9}`, `{in: "ix"}`},
		{"duplicate yaml key", "name: RomanParse", "name: RomanParse\nname: Duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			success(t, root, "build")
			html := read(t, root, "park.html")
			bad := strings.Replace(read(t, root, "roman_parse.yaml"), tc.old, tc.new, 1)
			write(t, root, "roman_parse.yaml", bad)
			code, _, err := invoke(t, root, false, "", "build")
			if code != 2 {
				t.Fatalf("%d %s", code, err)
			}
			if read(t, root, "park.html") != html || read(t, root, "roman_parse.yaml") != bad {
				t.Fatal("validation failure changed source/catalog")
			}
		})
	}
}

func TestDependencyTeachingRetriesFreshSnapshot(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	success(t, root, "process", "add", "--name", "Names")
	success(t, root, "function", "add", "--signature", "canonical(in) out", "Names.canonical")
	success(t, root, "example", "add", "--input", `"a"`, "--output", `"alpha"`, "Names.canonical")
	success(t, root, "example", "add", "--input", `"b"`, "--output", `"beta"`, "Names.canonical")
	write(t, root, "lookup.star", "def render(value):\n    return Names.canonical(value).upper()\n")
	success(t, root, "register", "lookup.star")
	success(t, root, "example", "add", "--input", `"a"`, "--output", `"ALPHA"`, "Lookup.render")
	success(t, root, "example", "add", "--input", `"b"`, "--output", `"BETA"`, "Lookup.render")
	code, out, err := invoke(t, root, true, "\"gamma\"\n", "run", "--interactive", "--input", `"c"`, "Lookup.render")
	if code != 0 || out != "\"GAMMA\"\n" || !strings.Contains(err, "Names.canonical") {
		t.Fatalf("%d %q %q", code, out, err)
	}
	p, loadErr := project.LoadWith(root, nil)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(p.Processes["Names"].Functions["canonical"].Examples) != 3 {
		t.Fatal("dependency not taught")
	}
	if len(p.Processes["Lookup"].Functions["render"].Examples) != 2 {
		t.Fatal("top-level example was changed")
	}
}
func TestSameProcessDispatchCannotBypassReadiness(t *testing.T) {
	for label, source := range map[string]string{
		"regular":   "def inner(value):\n    return value + 1\n\ndef outer(value):\n    return inner(value)\n",
		"bare":      "Process Local {\n    def inner(value):\n        return value + 1\n    def outer(value):\n        return inner(value)\n}\n",
		"qualified": "Process Local {\n    def inner(value):\n        return value + 1\n    def outer(value):\n        return Local.inner(value)\n}\n",
	} {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			success(t, root, "init")
			write(t, root, "local.star", source)
			success(t, root, "register", "local.star")
			// Refresh must not infer a dependency on the process itself.
			success(t, root, "register", "local.star")
			for _, fn := range []string{"inner", "outer"} {
				for _, n := range []int{1, 2} {
					success(t, root, "example", "add", "--input", fmt.Sprint(n), "--output", fmt.Sprint(n+1), "Local."+fn)
				}
			}
			if out := success(t, root, "run", "--input", "3", "Local.outer"); out != "4\n" {
				t.Fatal(out)
			}
			success(t, root, "example", "remove", "--input", "2", "Local.inner")
			code, _, err := invoke(t, root, false, "", "run", "--input", "3", "Local.outer")
			if code != 1 || !strings.Contains(err, "Local.inner needs examples") {
				t.Fatalf("inner bypassed dispatch: %d %s", code, err)
			}
		})
	}
}

func TestProcessDeclarationNamesAndPrivateHelpers(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "arithmetic.star", `Process Numbers {
    """Functions on numbers, with no shared state."""
    def _increment(value):
        items = []
        items.append(value)
        return items[0] + len(items)
    def next(value):
        """Return the following number."""
        return _increment(value)
}
`)
	code, _, errout := invoke(t, root, false, "", "register", "--name", "Other", "arithmetic.star")
	if code != 2 || !strings.Contains(errout, "does not match") {
		t.Fatalf("accepted conflicting name: %d %s", code, errout)
	}
	success(t, root, "register", "arithmetic.star")
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	proc := p.Processes["Numbers"]
	if proc == nil || len(proc.Functions) != 1 || proc.Description != "Functions on numbers, with no shared state." || proc.Functions["next"].Description != "Return the following number." {
		t.Fatalf("wrong registration: %+v", proc)
	}
	for _, n := range []int{1, 2} {
		success(t, root, "example", "add", "--input", fmt.Sprint(n), "--output", fmt.Sprint(n+1), "Numbers.next")
	}
	for _, n := range []int{3, 4} {
		if out := success(t, root, "run", "--input", fmt.Sprint(n), "Numbers.next"); out != fmt.Sprintln(n+1) {
			t.Fatal(out)
		}
	}
	success(t, root, "test")
	before := read(t, root, "arithmetic.yaml")
	write(t, root, "arithmetic.star", "Process Other {\n    def next(value):\n        return value + 1\n}\n")
	for _, command := range []string{"build", "register"} {
		args := []string{command}
		if command == "register" {
			args = append(args, "arithmetic.star")
		}
		code, _, errout := invoke(t, root, false, "", args...)
		if code != 2 || !strings.Contains(errout, "does not match") || read(t, root, "arithmetic.yaml") != before {
			t.Fatalf("%s accepted changed namespace or changed sidecar: %d %s", command, code, errout)
		}
	}
}

func TestSourceRestrictionsAndStepLimit(t *testing.T) {
	for _, source := range []string{
		"def f(a, b):\n    return a\n",
		"def f(a=1):\n    return a\n",
		"load(\"other.star\", \"f\")\n",
		"print(\"initialization ran\")\ndef f(x):\n    return x\n",
		"def _helper(x=print(\"initialization ran\")):\n    return x\ndef f(x):\n    return x\n",
		"Process Bad {\n    state = []\n    def f(x):\n        return x\n}\n",
		"Process Bad {\n    def _helper(x=print(\"initialization ran\")):\n        return x\n    def f(x):\n        return x\n}\n",
	} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			success(t, root, "init")
			before := read(t, root, "park.html")
			write(t, root, "bad.star", source)
			code, _, _ := invoke(t, root, false, "", "register", "bad.star")
			if code != 2 {
				t.Fatal("invalid module was registered", code)
			}
			if _, err := os.Stat(filepath.Join(root, "bad.yaml")); !os.IsNotExist(err) {
				t.Fatal("failed registration wrote a sidecar")
			}
			if read(t, root, "park.html") != before {
				t.Fatal("failed registration changed catalog")
			}
		})
	}
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "loop.star", "def f(n):\n    total = 0\n    for i in range(n):\n        total += i\n    return total\n")
	success(t, root, "register", "loop.star")
	success(t, root, "example", "add", "--input", "1", "--output", "0", "Loop.f")
	success(t, root, "example", "add", "--input", "2", "--output", "1", "Loop.f")
	code, _, err := invoke(t, root, false, "", "run", "--input", "10000000", "Loop.f")
	if code != 1 || !strings.Contains(err, "steps") {
		t.Fatalf("step limit failed: %d %s", code, err)
	}
	if len(files(t, root, ".cache")) != 0 {
		t.Fatal("failed evaluation was cached")
	}
}
func TestObjectInputCanonicalizationAndEOF(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	success(t, root, "process", "add", "--name", "Objects")
	success(t, root, "function", "add", "--signature", "f(request) response", "Objects.f")
	success(t, root, "example", "add", "--input", `{"b":2,"a":1}`, "--output", "null", "Objects.f")
	if out := success(t, root, "run", "--input", `{"a":1,"b":2}`, "Objects.f"); out != "null\n" {
		t.Fatal(out)
	}
	before := read(t, root, "Objects.yaml")
	code, _, _ := invoke(t, root, false, "", "example", "add", "--input", `{"a":1,"b":2}`, "--output", "1", "Objects.f")
	if code != 2 {
		t.Fatal("duplicate object input accepted")
	}
	code, _, _ = invoke(t, root, true, "", "run", "--interactive", "--input", "null", "Objects.f")
	if code != 3 {
		t.Fatal("EOF did not cancel unresolved call")
	}
	if read(t, root, "Objects.yaml") != before {
		t.Fatal("cancel or duplicate changed examples")
	}
}

func TestProcessAddDoesNotOverwriteAnotherSidecar(t *testing.T) {
	root := fixture(t)
	before := read(t, root, "roman_parse.yaml")
	code, _, _ := invoke(t, root, false, "", "process", "add", "--name", "Other", "--module", "roman_parse.star")
	if code != 2 || read(t, root, "roman_parse.yaml") != before {
		t.Fatal("process add overwrote an existing sidecar")
	}
}

func TestHelpWithoutProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	for _, args := range [][]string{{"run", "--help"}, {"register", "--help"}, {"process", "add", "--help"}, {"function", "add", "--help"}, {"example", "remove", "--help"}} {
		code, _, err := invoke(t, root, false, "", args...)
		if code != 0 || !strings.Contains(err, "Usage of") {
			t.Fatalf("help failed %v: %d %s", args, code, err)
		}
	}
}

func TestInteractiveCancellationDoesNotWrite(t *testing.T) {
	root := fixture(t)
	before := read(t, root, "roman_parse.yaml")
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var out, err bytes.Buffer
	c := &CLI{In: reader, Out: &out, Err: &err, Terminal: true}
	done := make(chan int, 1)
	go func() {
		done <- c.Run(ctx, []string{"--root", root, "run", "--interactive", "--input", `"unknown"`, "RomanParse.tokenize"})
	}()
	select {
	case code := <-done:
		if code != 1 {
			t.Fatal(code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation left prompt blocked")
	}
	if read(t, root, "roman_parse.yaml") != before {
		t.Fatal("cancelled prompt changed authored data")
	}
}
