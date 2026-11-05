package source

import (
	"strings"
	"testing"

	"go.starlark.net/syntax"
)

func TestProcessParsingPreservesStarlarkAndLocations(t *testing.T) {
	for _, indent := range []string{"", "    ", "\t"} {
		t.Run("indent="+indent, func(t *testing.T) {
			contents := "# Namespace comment\nProcess RomanAdd{ # header\n" +
				indent + "\"\"\"Process documentation with { braces }.\"\"\"\n" +
				indent + "def eval(text):\n" +
				indent + "    # A dictionary is ordinary Starlark, not a namespace.\n" +
				indent + "    values = {\"}\": r\"{\", \"quote\": \"\\\"\"}\n" +
				indent + "    return values[text]\n" +
				"} # namespace end\n# trailing comment\n"
			module, err := Parse("roman_add.star", []byte(contents))
			if err != nil {
				t.Fatal(err)
			}
			if module.Name != "RomanAdd" || len(module.File.Stmts) != 2 {
				t.Fatalf("wrong namespace: %+v", module)
			}
			def := module.File.Stmts[1].(*syntax.DefStmt)
			if def.Name.Name != "eval" || def.Def.Line != 4 || def.Def.Col != int32(len(indent)+1) {
				t.Fatalf("source position changed: %s", def.Def)
			}
		})
	}
}

func TestProcessRejectsStateAndInvalidStructure(t *testing.T) {
	for _, contents := range []string{
		"Process Bad {\n    counter = 0\n}\n",
		"Process Bad {\n    items = []\n    def f(x):\n        return x\n}\n",
		"Process Bad {\n    print(1)\n}\n",
		"Process Bad {\n    Process Nested {\n    }\n}\n",
		"Process Bad {\n    def f(x):\n        return x\n",
		"Process Bad {\n    def f(x):\n        return x\n}\ndef outside(x):\n    return x\n",
		"Process Bad {\n    def f(x):\n        return x\ndef outside(x):\n    return x\n}\n",
		"Process Bad {\n}\nProcess Other {\n}\n",
		"Process 123 {\n}\n",
	} {
		t.Run(contents, func(t *testing.T) {
			if _, err := Parse("bad.star", []byte(contents)); err == nil {
				t.Fatal("accepted invalid Process")
			}
		})
	}
	_, err := Parse("bad.star", []byte("Process Bad {\n    def f(x):\n        return (\n}\n"))
	if err == nil || !strings.Contains(err.Error(), "bad.star:") {
		t.Fatalf("missing source diagnostic: %v", err)
	}
}

func TestEmptyAndRegularModules(t *testing.T) {
	for _, contents := range []string{"Process Empty {\n}\n", "Process Empty {\n    # no functions yet\n}\n", "def f(x):\n    return x\n", "# comment only\n", ""} {
		if _, err := Parse("module.star", []byte(contents)); err != nil {
			t.Fatalf("%q: %v", contents, err)
		}
	}
}
