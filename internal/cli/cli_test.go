package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
		if !strings.HasSuffix(entry.Name(), ".star") {
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
func invoke(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var out, err bytes.Buffer
	c := &CLI{Out: &out, Err: &err}
	code := c.Run(context.Background(), append([]string{"--root", root}, args...))
	return code, out.String(), err.String()
}
func success(t *testing.T, root string, args ...string) string {
	t.Helper()
	code, out, err := invoke(t, root, args...)
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
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	var out, errout bytes.Buffer
	c := &CLI{Out: &out, Err: &errout}
	if code := c.Run(context.Background(), []string{"test"}); code != 2 || !strings.Contains(errout.String(), "no Park project found in current directory") {
		t.Fatalf("expected local project error, got %d: %s", code, errout.String())
	}
	if _, err := os.Stat(filepath.Join(parent, "park.html")); !os.IsNotExist(err) {
		t.Fatalf("default discovery touched the parent project: %v", err)
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
	if !strings.Contains(out, "5 implementation checks, 2 example-only records, 0 failures") {
		t.Fatal(out)
	}
	if len(files(t, root, ".cache")) != 0 {
		t.Fatal("tests persisted cache results")
	}
	if out := success(t, root, "run", `RomanAdd.eval("iv + i")`); out != "5\n" {
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
	success(t, root, "run", `RomanAdd.eval(text="iv + i")`)
	for name, contents := range cached {
		if read(t, root, filepath.Join(".cache", name)) != contents {
			t.Fatal("cache entry changed on hit")
		}
	}
	if len(files(t, root, ".cache")) != 3 {
		t.Fatal("a named call did not share the positional call's cache entry")
	}
	if read(t, root, "park.html") != html {
		t.Fatal("runtime cache regenerated catalog")
	}
	source := read(t, root, "roman_parse.star")
	write(t, root, "roman_parse.star", strings.Replace(source, "        return value", "        value = 0\n        return value", 1))
	code, _, err := invoke(t, root, "test", "RomanParse.parse")
	if code != 1 || !strings.Contains(err, "expected 9") || !strings.Contains(err, "roman_parse.star:4") {
		t.Fatalf("bad body escaped test: %d %s", code, err)
	}
	if out := success(t, root, "run", `RomanAdd.eval("iv + i")`); out != "0\n" {
		t.Fatal("code change did not invalidate cached result:", out)
	}
}

func TestUnansweredCallsBecomeQuestions(t *testing.T) {
	root := fixture(t)
	success(t, root, "build")
	for range 2 { // asking twice queues the question once
		code, out, err := invoke(t, root, "run", `RomanParse.tokenize("iv + i")`)
		if code != 3 || out != "" || !strings.Contains(err, `no answer for RomanParse.tokenize(text="iv + i")`) || !strings.Contains(err, `park example add 'RomanParse.tokenize("iv + i") == ...'`) {
			t.Fatalf("unanswered call %d %q %q", code, out, err)
		}
	}
	if q := read(t, root, "questions.txt"); strings.Count(q, `RomanParse.tokenize("iv + i")`) != 1 {
		t.Fatal("question not queued exactly once:\n" + q)
	}
	if !strings.Contains(read(t, root, "park.html"), "Questions for you") || !strings.Contains(success(t, root, "list"), "Questions (1)") {
		t.Fatal("question not shown in catalog and list")
	}
	success(t, root, "example", "add", `RomanParse.tokenize("iv + i") == ["iv", "+", "i"]`)
	if _, err := os.Stat(filepath.Join(root, "questions.txt")); !os.IsNotExist(err) {
		t.Fatal("answered question was not removed")
	}
	if strings.Contains(read(t, root, "park.html"), "Questions for you") {
		t.Fatal("catalog still shows the answered question")
	}
	if out := success(t, root, "run", `RomanParse.tokenize("iv + i")`); out != `["iv", "+", "i"]`+"\n" {
		t.Fatal(out)
	}
	success(t, root, "example", "remove", `RomanParse.tokenize("iv + i")`)
	if code, _, _ := invoke(t, root, "run", `RomanParse.tokenize("iv + i")`); code != 3 {
		t.Fatal(code)
	}
	// parse has examples in two files; removing finds each where it is written.
	success(t, root, "example", "remove", `RomanParse.parse("vi")`)
	success(t, root, "example", "remove", `RomanParse.parse("xii")`)
	if strings.Contains(read(t, root, "roman_add.star"), "xii") {
		t.Fatal("cross-file example was not removed")
	}
	if !strings.Contains(read(t, root, "park.html"), "needs examples") {
		t.Fatal("catalog did not reflect readiness")
	}
	if code, _, _ := invoke(t, root, "run", `RomanParse.parse("iv")`); code != 3 {
		t.Fatal("unready body executed", code)
	}
	if !strings.Contains(read(t, root, "questions.txt"), `RomanParse.parse("iv")`) {
		t.Fatal("unready call was not queued")
	}
	if success(t, root, "run", `RomanParse.parse("ix")`) != "9\n" {
		t.Fatal("known example was gated")
	}
}

func TestQuestionsAnsweredByImplementationOrStale(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "names.star", "example: canonical(\"a\") == \"alpha\"\nexample: canonical(\"b\") == \"beta\"\n\ndef canonical(alias):\n    pass\n")
	write(t, root, "lookup.star", "example: render(\"a\") == \"ALPHA\"\nexample: render(\"b\") == \"BETA\"\n\ndef render(value):\n    result = Names.canonical(value).upper()\n    return result\n")
	success(t, root, "build")
	// A miss inside a dependency asks about the dependency's call.
	code, _, err := invoke(t, root, "run", `Lookup.render("c")`)
	if code != 3 || !strings.Contains(err, `Names.canonical(alias="c")`) {
		t.Fatalf("%d %q", code, err)
	}
	if !strings.Contains(read(t, root, "questions.txt"), `Names.canonical("c")`) {
		t.Fatal("dependency question not queued")
	}
	// park test records misses too.
	write(t, root, "lookup.star", read(t, root, "lookup.star")+"example: render(\"d\") == \"DELTA\"\n")
	code, _, err = invoke(t, root, "test")
	if code != 1 || !strings.Contains(read(t, root, "questions.txt"), `Names.canonical("d")`) {
		t.Fatalf("test did not queue its miss: %d %s", code, err)
	}
	// Implementing the function answers its questions.
	write(t, root, "names.star", "example: canonical(\"a\") == \"alpha\"\nexample: canonical(\"b\") == \"beta\"\n\ndef canonical(alias):\n    name = alias + \"!\"\n    return name\n")
	write(t, root, "questions.txt", read(t, root, "questions.txt")+"Gone.f(1)\n")
	code, _, err = invoke(t, root, "build")
	if code != 0 || !strings.Contains(err, "dropped a question that no longer matches a function: Gone.f(1)") {
		t.Fatalf("%d %s", code, err)
	}
	if _, err := os.Stat(filepath.Join(root, "questions.txt")); !os.IsNotExist(err) {
		t.Fatal("implemented function's questions remain:\n" + read(t, root, "questions.txt"))
	}
}

func TestExampleEditsPreserveSource(t *testing.T) {
	root := fixture(t)
	source := strings.Replace(read(t, root, "roman_parse.star"), `example: parse("ix") == 9`, `example: parse("ix") == 9  # keep this comment`, 1)
	write(t, root, "roman_parse.star", source)
	success(t, root, "example", "add", `RomanParse.parse("xiv") == 14`)
	after := read(t, root, "roman_parse.star")
	if !strings.Contains(after, "# keep this comment") || !strings.Contains(after, `    example: parse("xiv") == 14`+"\n\n    def parse") {
		t.Fatal("example add did not insert after the last example:\n" + after)
	}
	success(t, root, "example", "remove", `RomanParse.parse("xiv")`)
	if read(t, root, "roman_parse.star") != source {
		t.Fatal("add then remove did not restore the module")
	}
}

func TestQuestionsRemainUntilImplementationIsReady(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "double.star", "def double(value):\n    pass\n")
	if code, _, _ := invoke(t, root, "run", "Double.double(3)"); code != 3 {
		t.Fatal("expected unanswered call", code)
	}
	write(t, root, "double.star", "def double(value):\n    result = value * 2\n    return result\n")
	for count := range 2 { // Neither zero nor one example may clear the question.
		if count == 1 {
			success(t, root, "example", "add", "Double.double(1) == 2")
		}
		success(t, root, "build")
		if !strings.Contains(read(t, root, "questions.txt"), "Double.double(3)") || !strings.Contains(success(t, root, "list"), "Questions (1)") || !strings.Contains(read(t, root, "park.html"), "Questions for you") {
			t.Fatal("unready implementation cleared its question")
		}
		code, _, errout := invoke(t, root, "run", "Double.double(3)")
		if code != 3 || !strings.Contains(errout, "Double.double needs examples") || !strings.Contains(errout, "park example add 'Double.double(3) == ...'") {
			t.Fatalf("unready call should queue an actionable question: %d %s", code, errout)
		}
		if strings.Count(read(t, root, "questions.txt"), "Double.double(3)") != 1 || len(files(t, root, ".cache")) != 0 {
			t.Fatal("unready call duplicated its question or wrote cache")
		}
	}
	if success(t, root, "run", "Double.double(1)") != "2\n" {
		t.Fatal("known example should still answer")
	}
	success(t, root, "example", "add", "Double.double(2) == 4")
	if _, err := os.Stat(filepath.Join(root, "questions.txt")); !os.IsNotExist(err) {
		t.Fatal("ready implementation should clear its question", err)
	}
	if success(t, root, "run", "Double.double(3)") != "6\n" {
		t.Fatal("ready body did not execute")
	}
	// An exact example also answers a question without enabling the body.
	success(t, root, "example", "remove", "Double.double(2)")
	if code, _, _ := invoke(t, root, "run", "Double.double(4)"); code != 3 {
		t.Fatal("expected new unready question", code)
	}
	success(t, root, "example", "remove", "Double.double(1)")
	success(t, root, "example", "add", "Double.double(4) == 8")
	if _, err := os.Stat(filepath.Join(root, "questions.txt")); !os.IsNotExist(err) {
		t.Fatal("exact example should clear its question", err)
	}
}

func TestBuildRejectsUnknownProcessMembers(t *testing.T) {
	for label, expression := range map[string]string{
		"same process": "Fact.missing(key)",
		"dependency":   "Other.missing(key)",
		"private":      "Other._helper(key)",
		"reference":    "Other.missing",
	} {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			success(t, root, "init")
			html := read(t, root, "park.html")
			write(t, root, "other.star", "def known(key):\n    pass\n\ndef _helper(key):\n    return key\n")
			bad := "Process Fact:\n    def lookup(key):\n        answer = " + expression + "\n        return answer\n"
			write(t, root, "fact.star", bad)
			write(t, root, "questions.txt", "Fact.lookup(1)\n")
			code, _, errout := invoke(t, root, "build")
			if code != 2 || !strings.Contains(errout, "unknown function") || !strings.Contains(errout, "fact.star:3:") {
				t.Fatalf("bad member should fail build with location: %d %s", code, errout)
			}
			if read(t, root, "park.html") != html || read(t, root, "fact.star") != bad || read(t, root, "questions.txt") != "Fact.lookup(1)\n" {
				t.Fatal("failed validation changed project files")
			}
		})
	}
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "fact.star", `Process Fact:
    example: lookup("a") == "A"
    example: lookup("b") == "B"
    def _local(Fact):
        return Fact.upper()
    def lookup(key):
        answer = _local(key)
        return answer
`)
	success(t, root, "build")
	if success(t, root, "run", `Fact.lookup("c")`) != "\"C\"\n" {
		t.Fatal("local method call was treated as a process member")
	}
}

func TestReadyDoesNotImplyVerified(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "words.star", `Process Words:
    example: plural("cat") == "cats"
    example: plural("ox") == "oxen"
    def plural(noun):
        word = noun + "s"
        return word
`)
	success(t, root, "build")
	if success(t, root, "run", `Words.plural("ox")`) != "\"oxen\"\n" || success(t, root, "run", `Words.plural("dog")`) != "\"dogs\"\n" {
		t.Fatal("examples must remain authoritative and the ready body may run")
	}
	code, _, errout := invoke(t, root, "test")
	if code != 1 || !strings.Contains(errout, `got "oxs"; expected "oxen"`) {
		t.Fatalf("strict tests must report intentional disagreements: %d %s", code, errout)
	}
}

func TestInitAndExamples(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "double.star", "\"\"\"Double an integer.\"\"\"\ndef double(value):\n    result = value * 2\n    return result\n\ndef label(setting):\n    pass\n")
	success(t, root, "build")
	if !strings.Contains(success(t, root, "list"), "needs examples") {
		t.Fatal("a new implementation should need examples")
	}
	success(t, root, "example", "add", "Double.double(1) == 2")
	success(t, root, "example", "add", "Double.double(2) == 4")
	if !strings.HasPrefix(read(t, root, "double.star"), "\"\"\"Double an integer.\"\"\"\n\nexample: double(1) == 2\nexample: double(2) == 4\n") {
		t.Fatal("examples not placed after the module docstring:\n" + read(t, root, "double.star"))
	}
	if success(t, root, "run", "Double.double(3)") != "6\n" {
		t.Fatal("wrong result")
	}
	success(t, root, "example", "add", `Double.label(None) == {"ok": True}`)
	if success(t, root, "run", "Double.label(setting=None)") != "{\"ok\": True}\n" {
		t.Fatal("null/object lookup failed")
	}
	code, _, _ := invoke(t, root, "init")
	if code != 2 {
		t.Fatal("init overwrote existing project")
	}
}

func TestValidationHasNoSourceWrites(t *testing.T) {
	cases := []struct{ name, old, new, want string }{
		{"duplicate inputs", `example: parse("vi") == 6`, `example: parse("ix") == 6`, "duplicate example"},
		{"float", `parse("ix") == 9`, `parse("ix") == 9.5`, "literal"},
		{"unknown input", `parse("ix") == 9`, `parse(numeral="ix") == 9`, "unknown input numeral"},
		{"unknown function", `example: parse("ix") == 9`, `example: missing("ix") == 9`, "def missing(...): pass"},
		{"keyword parameter", "def parse(text):", "def parse(in):", "roman_parse.star"},
		{"returns an input", "        return value", "        return text", "returns its input"},
		{"bare return", "        return value", "        return", "named variable"},
		{"expression return", "        return value", "        return value + 0", "named variable"},
		{"brace syntax", "Process RomanParse:", "Process RomanParse {", "Process Name:"},
		{"example inside def", "        previous = 0\n", "        previous = 0\n        example: parse(\"i\") == 1\n", "example:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			success(t, root, "build")
			html := read(t, root, "park.html")
			bad := strings.Replace(read(t, root, "roman_parse.star"), tc.old, tc.new, 1)
			if bad == read(t, root, "roman_parse.star") {
				t.Fatal("test case did not change the source")
			}
			write(t, root, "roman_parse.star", bad)
			code, _, err := invoke(t, root, "build")
			if code != 2 || !strings.Contains(err, tc.want) {
				t.Fatalf("%d %s", code, err)
			}
			if read(t, root, "park.html") != html || read(t, root, "roman_parse.star") != bad {
				t.Fatal("validation failure changed source/catalog")
			}
		})
	}
}

func TestSameProcessDispatchCannotBypassReadiness(t *testing.T) {
	for label, source := range map[string]string{
		"regular":   "def inner(value):\n    result = value + 1\n    return result\n\ndef outer(value):\n    result = inner(value)\n    return result\n",
		"bare":      "Process Local:\n    def inner(value):\n        result = value + 1\n        return result\n    def outer(value):\n        result = inner(value)\n        return result\n",
		"qualified": "Process Local:\n    def inner(value):\n        result = value + 1\n        return result\n    def outer(value):\n        result = Local.inner(value)\n        return result\n",
	} {
		t.Run(label, func(t *testing.T) {
			root := t.TempDir()
			success(t, root, "init")
			write(t, root, "local.star", source)
			success(t, root, "build")
			for _, fn := range []string{"inner", "outer"} {
				for _, n := range []int{1, 2} {
					success(t, root, "example", "add", fmt.Sprintf("Local.%s(%d) == %d", fn, n, n+1))
				}
			}
			if p, err := project.LoadWith(root, nil); err != nil || len(p.Processes["Local"].Uses) != 0 {
				t.Fatalf("a process must not depend on itself: %v", err)
			}
			if out := success(t, root, "run", "Local.outer(3)"); out != "4\n" {
				t.Fatal(out)
			}
			success(t, root, "example", "remove", "Local.inner(2)")
			code, _, err := invoke(t, root, "run", "Local.outer(3)")
			if code != 3 || !strings.Contains(err, "Local.inner needs examples") {
				t.Fatalf("inner bypassed dispatch: %d %s", code, err)
			}
			if q := read(t, root, "questions.txt"); !strings.Contains(q, "Local.inner(3)") || strings.Contains(q, "Local.outer(3)") {
				t.Fatal("question must identify the blocked dependency:\n" + q)
			}
			code, _, err = invoke(t, root, "test", "Local.outer")
			if code != 1 || !strings.Contains(err, "Local.inner needs examples") || !strings.Contains(read(t, root, "questions.txt"), "Local.inner(2)") {
				t.Fatalf("test did not queue its unready dependency: %d %s", code, err)
			}
		})
	}
}

func TestProcessDeclarationNamesAndPrivateHelpers(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "arithmetic.star", `Process Numbers:
    """Functions on numbers, with no shared state."""

    example: next(1) == 2
    example: next(2) == 3

    def _increment(value):
        items = []
        items.append(value)
        return items[0] + len(items)

    def next(value):
        """Return the following number."""
        following = _increment(value)
        return following
`)
	success(t, root, "build")
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	proc := p.Processes["Numbers"]
	if proc == nil || len(proc.Functions) != 1 || proc.Description != "Functions on numbers, with no shared state." || proc.Functions["next"].Description != "Return the following number." || proc.Functions["next"].Signature != "next(value) following" {
		t.Fatalf("wrong process: %+v", proc)
	}
	for _, n := range []int{3, 4} {
		if out := success(t, root, "run", fmt.Sprintf("Numbers.next(%d)", n)); out != fmt.Sprintln(n+1) {
			t.Fatal(out)
		}
	}
	success(t, root, "test")
	write(t, root, "other.star", "Process Numbers:\n    def f(x):\n        pass\n")
	code, _, errout := invoke(t, root, "build")
	if code != 2 || !strings.Contains(errout, "declared in both") {
		t.Fatalf("accepted duplicate process: %d %s", code, errout)
	}
}

func TestSourceRestrictionsAndStepLimit(t *testing.T) {
	for _, source := range []string{
		"def f(*values):\n    return values\n",
		"def f():\n    result = 1\n    return result\n",
		"def f(a=1):\n    return a\n",
		"load(\"other.star\", \"f\")\n",
		"print(\"initialization ran\")\ndef f(x):\n    pass\n",
		"def _helper(x=print(\"initialization ran\")):\n    return x\ndef f(x):\n    pass\n",
		"Process Bad:\n    state = []\n    def f(x):\n        pass\n",
		"Process Bad:\n    def _helper(x=print(\"initialization ran\")):\n        return x\n    def f(x):\n        pass\n",
	} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			success(t, root, "init")
			before := read(t, root, "park.html")
			write(t, root, "bad.star", source)
			code, _, _ := invoke(t, root, "build")
			if code != 2 {
				t.Fatal("invalid module was accepted", code)
			}
			if read(t, root, "park.html") != before || read(t, root, "bad.star") != source {
				t.Fatal("failed build changed files")
			}
		})
	}
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "loop.star", "example: f(1) == 0\nexample: f(2) == 1\n\ndef f(n):\n    total = 0\n    for i in range(n):\n        total += i\n    return total\n")
	code, _, err := invoke(t, root, "run", "Loop.f(10000000)")
	if code != 1 || !strings.Contains(err, "steps") {
		t.Fatalf("step limit failed: %d %s", code, err)
	}
	if len(files(t, root, ".cache")) != 0 {
		t.Fatal("failed evaluation was cached")
	}
}

func TestObjectInputCanonicalization(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "objects.star", "def f(request):\n    pass\n")
	success(t, root, "example", "add", `Objects.f({"b": 2, "a": 1}) == None`)
	if out := success(t, root, "run", `Objects.f({"a": 1, "b": 2})`); out != "None\n" {
		t.Fatal(out)
	}
	before := read(t, root, "objects.star")
	code, _, _ := invoke(t, root, "example", "add", `Objects.f(request={"a": 1, "b": 2}) == 1`)
	if code != 2 {
		t.Fatal("duplicate object input accepted")
	}
	if read(t, root, "objects.star") != before {
		t.Fatal("duplicate changed examples")
	}
}

func TestNamedInputs(t *testing.T) {
	root := t.TempDir()
	success(t, root, "init")
	write(t, root, "math.star", "def add(left, right):\n    result = left + right\n    return result\n")
	success(t, root, "example", "add", "Math.add(1, 2) == 3")
	// Named arguments in any order are written positionally, in signature order.
	success(t, root, "example", "add", "Math.add(right=5, left=4) == 9")
	if !strings.Contains(read(t, root, "math.star"), "example: add(4, 5) == 9") {
		t.Fatal("example not written in signature order:\n" + read(t, root, "math.star"))
	}
	if out := success(t, root, "run", "Math.add(10, 20)"); out != "30\n" {
		t.Fatal(out)
	}
	for call, want := range map[string]string{
		"Math.add(1)":             "missing input right",
		"Math.add(1, 2, carry=0)": "unknown input carry",
		"Math.add(1, 2, 3)":       "takes 2 inputs",
		"Math.add(1, left=2)":     "given more than once",
		"add(1, 2)":               "name the process",
		"Math.add(one, 2)":        "not a literal",
	} {
		code, _, err := invoke(t, root, "run", call)
		if code != 2 || !strings.Contains(err, want) {
			t.Errorf("%s: %d %s", call, code, err)
		}
	}
	if !strings.Contains(read(t, root, "park.html"), "left: 1\nright: 2\nresult: 3") {
		t.Fatal("catalog does not show each named input")
	}
}

func TestExamplesAreProjectWide(t *testing.T) {
	root := fixture(t)
	p, err := project.LoadWith(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	parse := p.Processes["RomanParse"].Functions["parse"]
	if len(parse.Examples) != 3 || parse.Examples[2].Path != "roman_add.star" {
		t.Fatalf("cross-file example missing: %+v", parse.Examples)
	}
	if len(p.Processes["RomanAdd"].Uses) != 1 {
		t.Fatal("examples must not create dependencies, but code references must")
	}
	write(t, root, "extra.star", "example: RomanParse.parse(\"xii\") == 12\n\ndef unused(x):\n    pass\n")
	code, _, errout := invoke(t, root, "build")
	if code != 2 || !strings.Contains(errout, `duplicate example for RomanParse.parse(text="xii"); first written at`) {
		t.Fatalf("duplicate across files accepted: %d %s", code, errout)
	}
}

func TestHelpWithoutProject(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	for _, args := range [][]string{{"run", "--help"}, {"list", "--help"}, {"test", "--help"}, {"example", "add", "--help"}, {"example", "remove", "--help"}} {
		code, _, err := invoke(t, root, args...)
		if code != 0 || !strings.Contains(err, "Usage of") {
			t.Fatalf("help failed %v: %d %s", args, code, err)
		}
	}
}
