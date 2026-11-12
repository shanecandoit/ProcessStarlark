package source

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatPreservesLiteralsAndCollapsesBlankRuns(t *testing.T) {
	input := []byte("\n\nProcess Words:  \n    \"\"\"Docstring  \n\n\n    literal blank lines\n    \"\"\"   \n\n\n    example: word(\"雪  x\") == \"a  b\"   \n\n\n    def word(x):\n        result = r'''a  \n\n\n    b'''  \n        return result   \n\n\n")
	out, err := Format("words.star", input)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "Docstring  \n\n\n    literal") || !strings.Contains(text, "r'''a  \n\n\n    b'''") || !strings.Contains(text, `word("雪  x") == "a  b"`) {
		t.Fatal("literal content changed", text)
	}
	if strings.Contains(text, "\"\"\"   \n") || strings.Contains(text, "\n\n\n    example") || strings.HasPrefix(text, "\n") || !strings.HasSuffix(text, "return result\n") {
		t.Fatal("hygiene not applied", text)
	}
	again, err := Format("words.star", out)
	if err != nil || !bytes.Equal(out, again) {
		t.Fatal("format is not idempotent", err)
	}
}

func TestFormatExamplesBytesContinuationsAndCRLF(t *testing.T) {
	for _, text := range []string{
		"example: value(\"\"\"first\n\n\nlast\"\"\") == 1\n\n\ndef value(x):\n    result = 1\n    return result\n",
		"DATA = b'''x  \n\n\ny'''\n\n\ndef value(x):\n    result = DATA\n    return result\n",
		"def value(x):\n    result = \"a\\\n  b\"\n    return result\n",
		"def value(x):\r\n    result = 1 + \\\r\n        2\r\n    return result  \r\n",
	} {
		out, err := Format("value.star", []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		_, before, err := literalMask("value.star", []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		_, after, err := literalMask("value.star", out)
		if err != nil || strings.Join(before, "\x00") != strings.Join(after, "\x00") {
			t.Fatal("literal bytes changed", err)
		}
	}
}

func TestLintAndInvalidSource(t *testing.T) {
	if _, err := Format("invalid.star", []byte("def wrong(:\n")); err == nil {
		t.Fatal("invalid source repaired")
	}
	diagnostics, err := Lint("value.star", []byte("def value(x):\n\tresult = x\n\treturn result  \n\n\n"))
	if err != nil || len(diagnostics) < 2 {
		t.Fatal(diagnostics, err)
	}
}
