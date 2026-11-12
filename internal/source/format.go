package source

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"go.starlark.net/syntax"
)

// Format applies conservative source hygiene. Literal bytes and indentation are
// preserved; blank runs and trailing horizontal whitespace outside literals are
// removed. Invalid source is never repaired speculatively.
func Format(path string, input []byte) ([]byte, error) {
	mask, literals, err := literalMask(path, input)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	blank, offset := false, 0
	for _, chunk := range bytes.SplitAfter(input, []byte("\n")) {
		if len(chunk) == 0 {
			continue
		}
		end := len(chunk)
		if chunk[end-1] == '\n' {
			end--
		}
		for end > 0 && strings.ContainsRune(" \t\r", rune(chunk[end-1])) && !mask[offset+end-1] {
			end--
		}
		isBlank := end == 0
		// A blank line inside a multiline literal is data, including its newline.
		for i := range chunk {
			if mask[offset+i] {
				isBlank = false
				break
			}
		}
		if !isBlank || (!blank && out.Len() > 0) {
			out.Write(chunk[:end])
			out.WriteByte('\n')
		}
		blank = isBlank
		offset += len(chunk)
	}
	result := bytes.TrimRight(out.Bytes(), "\n")
	result = append(append([]byte{}, result...), '\n')
	_, after, err := literalMask(path, result)
	if err != nil {
		return nil, fmt.Errorf("formatting rejected: %w", err)
	}
	if !reflect.DeepEqual(literals, after) {
		return nil, fmt.Errorf("formatting would change a literal in %s", path)
	}
	return result, nil
}

// literalMask parses the same temporary Park syntax as Parse, retaining example
// expressions so that their literals are protected too. Columns count runes.
func literalMask(path string, input []byte) ([]bool, []string, error) {
	if _, err := Parse(path, input); err != nil {
		return nil, nil, err
	}
	lines := strings.Split(string(input), "\n")
	converted := append([]string{}, lines...)
	shifts := map[int]int{}
	for i, line := range lines {
		if m := exampleLine.FindStringSubmatch(line); m != nil {
			converted[i] = m[1] + exampleMarker + line[len(m[0]):]
			shifts[i] = len(exampleMarker) - len("example:")
		}
	}
	first := significant(lines, 0)
	if first < len(lines) {
		if m := header.FindStringSubmatch(lines[first]); m != nil {
			converted[first] = "def " + m[1] + "():"
		}
	}
	file, err := (&syntax.FileOptions{}).Parse(path, strings.Join(converted, "\n"), 0)
	if err != nil {
		return nil, nil, err
	}
	starts := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		starts[i] = starts[i-1] + len(lines[i-1]) + 1
	}
	position := func(p syntax.Position) int {
		line, col := int(p.Line)-1, int(p.Col)-1
		if line < 0 || line >= len(lines) {
			return len(input)
		}
		col -= shifts[line]
		index := 0
		for col > 0 && index < len(lines[line]) {
			_, size := utf8.DecodeRuneInString(lines[line][index:])
			index += size
			col--
		}
		return starts[line] + index
	}
	mask := make([]bool, len(input))
	var literals []string
	syntax.Walk(file, func(node syntax.Node) bool {
		if lit, ok := node.(*syntax.Literal); ok && (lit.Token == syntax.STRING || lit.Token == syntax.BYTES) {
			start, end := lit.Span()
			a, b := position(start), position(end)
			for i := a; i < b && i < len(mask); i++ {
				mask[i] = true
			}
			literals = append(literals, string(input[a:b]))
		}
		return true
	})
	return mask, literals, nil
}

type Diagnostic struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// Lint reports basic formatting and tab-indentation guidance. Parsing and engine
// validation supply syntax, signature, duplicate-example, and name checks.
func Lint(path string, input []byte) ([]Diagnostic, error) {
	formatted, err := Format(path, input)
	if err != nil {
		return nil, err
	}
	out := []Diagnostic{}
	if !bytes.Equal(input, formatted) {
		out = append(out, Diagnostic{path, 1, "Format source: one blank line at most, no trailing whitespace, and a final newline (literals preserved)."})
	}
	mask, _, _ := literalMask(path, input)
	offset := 0
	for i, line := range strings.SplitAfter(string(input), "\n") {
		for j := 0; j < len(line) && (line[j] == ' ' || line[j] == '\t'); j++ {
			if line[j] == '\t' && !mask[offset+j] {
				out = append(out, Diagnostic{path, i + 1, "Prefer spaces for indentation; formatting does not change indentation."})
				break
			}
		}
		offset += len(line)
	}
	return out, nil
}
