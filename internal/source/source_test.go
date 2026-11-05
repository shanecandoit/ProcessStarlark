package source

import (
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/syntax"
)

func TestProcessParsingPreservesStarlarkAndLocations(t *testing.T) {
	for _, indent := range []string{"    ", "\t"} {
		t.Run("indent="+indent, func(t *testing.T) {
			contents := "# Namespace comment\nProcess RomanAdd: # header\n" +
				indent + "\"\"\"Process documentation with { braces }.\"\"\"\n" +
				indent + "example: eval(\"}\") == 1\n" +
				indent + "def eval(text):\n" +
				indent + "    # A dictionary is ordinary Starlark, not a namespace.\n" +
				indent + "    values = {\"}\": 1, \"quote\": 2}\n" +
				indent + "    result = values[text]\n" +
				indent + "    return result\n" +
				"# trailing comment\n"
			module, err := Parse("roman_add.star", []byte(contents))
			if err != nil {
				t.Fatal(err)
			}
			if module.Name != "RomanAdd" || len(module.File.Stmts) != 2 || module.Indent != indent {
				t.Fatalf("wrong namespace: %+v", module)
			}
			def := module.File.Stmts[1].(*syntax.DefStmt)
			if def.Name.Name != "eval" || def.Def.Line != 5 || def.Def.Col != int32(len(indent)+1) {
				t.Fatalf("source position changed: %s", def.Def)
			}
			if len(module.Examples) != 1 || module.Examples[0].Line != 4 || module.Examples[0].Want != int64(1) {
				t.Fatalf("wrong examples: %+v", module.Examples)
			}
		})
	}
}

func TestExamples(t *testing.T) {
	contents := `Process Math:
    example: add(1, 2) == 3
    example: add(right=6, left=9) == 15
    example: Other.f({"b": [1, None], "a": True}) == -4
    example: add(
        1,
        right = 2,
    ) == 3

    def add(left, right):
        result = left + right
        return result
`
	module, err := Parse("math.star", []byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	if len(module.Examples) != 4 || len(module.File.Stmts) != 1 {
		t.Fatalf("got %d examples, %d statements", len(module.Examples), len(module.File.Stmts))
	}
	second := module.Examples[1].Call
	if !reflect.DeepEqual(second.Names, []string{"right", "left"}) || !reflect.DeepEqual(second.Values, []any{int64(6), int64(9)}) {
		t.Fatalf("keywords: %+v", second)
	}
	third := module.Examples[2]
	if third.Call.Process != "Other" || third.Call.Function != "f" || third.Want != int64(-4) {
		t.Fatalf("qualified: %+v", third)
	}
	want := map[string]any{"b": []any{int64(1), nil}, "a": true}
	if !reflect.DeepEqual(third.Call.Args[0], want) {
		t.Fatalf("dict literal: %#v", third.Call.Args[0])
	}
	if last := module.Examples[3]; last.Line != 5 || last.EndLine != 8 {
		t.Fatalf("multi-line span: %d-%d", last.Line, last.EndLine)
	}
}

func TestInvalidExamples(t *testing.T) {
	for _, line := range []string{
		`example: add(1, 2)`,           // no expected value
		`example: add(1, 2) != 3`,      // wrong operator
		`example: add(x, 2) == 3`,      // not a literal
		`example: add(1.5, 2) == 3`,    // float
		`example: 3 == add(1, 2)`,      // reversed
		`example: add(left=1, 2) == 3`, // positional after keyword
		`example: add(*[1, 2]) == 3`,   // star args
		`example: f({1: 2}) == 3`,      // non-string key
		`example: f(99999999999999999999) == 3`,
	} {
		t.Run(line, func(t *testing.T) {
			src := "Process P:\n    " + line + "\n    def add(left, right):\n        return left\n"
			if _, err := Parse("p.star", []byte(src)); err == nil {
				t.Fatal("accepted invalid example")
			}
		})
	}
	for name, src := range map[string]string{
		"inside a def":    "Process P:\n    def f(x):\n        example: f(1) == 1\n        return x\n",
		"inside a string": "Process P:\n    \"\"\"Docs\nexample: f(1) == 1\n\"\"\"\n    def f(x):\n        return x\n",
	} {
		if _, err := Parse("p.star", []byte(src)); err == nil || !strings.Contains(err.Error(), "example:") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestProcessRejectsStateAndInvalidStructure(t *testing.T) {
	for _, contents := range []string{
		"Process Bad:\n    counter = 0\n",
		"Process Bad:\n    items = []\n    def f(x):\n        return x\n",
		"Process Bad:\n    print(1)\n",
		"Process Bad:\n    Process Nested:\n        pass\n",
		"Process Bad:\n    def f(x):\n        return x\ndef outside(x):\n    return x\n",
		"Process Bad:\n    pass\nProcess Other:\n    pass\n",
		"Process Bad:\n",
		"Process Bad:\ndef f(x):\n    return x\n",
		"Process Bad {\n    def f(x):\n        return x\n}\n",
		"Process 123:\n    pass\n",
	} {
		t.Run(contents, func(t *testing.T) {
			if _, err := Parse("bad.star", []byte(contents)); err == nil {
				t.Fatal("accepted invalid Process")
			}
		})
	}
	_, err := Parse("bad.star", []byte("Process Bad:\n    def f(x):\n        return (\n"))
	if err == nil || !strings.Contains(err.Error(), "bad.star:") {
		t.Fatalf("missing source diagnostic: %v", err)
	}
}

func TestRegularModules(t *testing.T) {
	for _, contents := range []string{"def f(x):\n    return x\n", "example: f(1) == 1\ndef f(x):\n    return x\n", "# comment only\n", ""} {
		if _, err := Parse("module.star", []byte(contents)); err != nil {
			t.Fatalf("%q: %v", contents, err)
		}
	}
}

func TestParseCall(t *testing.T) {
	call, err := ParseCall(`Math.add(9, right=6)`)
	if err != nil || call.Process != "Math" || call.Function != "add" || call.Args[0] != int64(9) || call.Names[0] != "right" {
		t.Fatalf("%+v %v", call, err)
	}
	for _, bad := range []string{`Math.add`, `Math.add(9) == 1`, `Math.add(ix)`, `a.b.c(1)`} {
		if _, err := ParseCall(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	call, want, err := ParseExample(`RomanParse.parse("xiv") == 14`)
	if err != nil || call.Args[0] != "xiv" || want != int64(14) {
		t.Fatalf("%+v %v %v", call, want, err)
	}
}
