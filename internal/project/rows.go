package project

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/source"
)

// Rows typed as text: spreadsheet pastes in the playground and files given to
// park import. Cells are typed against the function's existing examples, so
// `US` in a column of strings means "US", and `02134` stays a string there.

// ParseCell turns typed text into a value. A column whose existing values are
// all strings (or all whole numbers) takes that type; otherwise the cell is
// read as a Starlark literal if it is one, and as a string if not.
func ParseCell(text string, column []any) (any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("empty cell; write \"\" for empty text")
	}
	literal, litErr := source.ParseValue(trimmed)
	switch columnKind(column) {
	case "string":
		if s, ok := literal.(string); ok && litErr == nil {
			return s, nil // Quoted on purpose, e.g. to keep spaces.
		}
		return trimmed, nil
	case "int":
		if n, ok := literal.(int64); ok && litErr == nil {
			return n, nil
		}
		return nil, fmt.Errorf("%q: this column holds whole numbers", trimmed)
	}
	if litErr == nil {
		return literal, nil
	}
	return trimmed, nil
}

func columnKind(column []any) string {
	k := ""
	for _, v := range column {
		var this string
		switch v.(type) {
		case string:
			this = "string"
		case int64:
			this = "int"
		default:
			return ""
		}
		if k != "" && k != this {
			return ""
		}
		k = this
	}
	return k
}

// Columns lists a function's table columns: its inputs, then its output.
func (f *Function) Columns() []string {
	return append(append([]string{}, f.Inputs...), f.Output)
}

// ParseRow binds typed cells (one per input, then the output) to a function.
func ParseRow(f *Function, cells []string) (map[string]any, any, error) {
	if len(cells) != len(f.Inputs)+1 {
		return nil, nil, fmt.Errorf("expected %d cells (%s, then %s), got %d", len(f.Inputs)+1, strings.Join(f.Inputs, ", "), f.Output, len(cells))
	}
	input := map[string]any{}
	for i, name := range f.Inputs {
		var column []any
		for _, ex := range f.Examples {
			column = append(column, ex.Input[name])
		}
		v, err := ParseCell(cells[i], column)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		input[name] = v
	}
	var column []any
	for _, ex := range f.Examples {
		column = append(column, ex.Output)
	}
	out, err := ParseCell(cells[len(cells)-1], column)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", f.Output, err)
	}
	return input, out, nil
}

// ReadRecords splits pasted or file text into cells: tab-separated if any tab
// appears, comma-separated otherwise.
func ReadRecords(text string) ([][]string, error) {
	records, _, err := readRecords(text)
	return records, err
}

// readRecords also returns each record's 1-based line; the reader skips blank
// lines, so positions in the slice are not lines.
func readRecords(text string) ([][]string, []int, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimRight(text, "\r\n")))
	reader.Comma = ','
	if strings.Contains(text, "\t") {
		reader.Comma = '\t'
	}
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	var records [][]string
	var lines []int
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return records, lines, nil
		}
		if err != nil {
			return nil, nil, err
		}
		line, _ := reader.FieldPos(0)
		records = append(records, rec)
		lines = append(lines, line)
	}
}

// Row is one table row read from a file, with its 1-based line in the file.
type Row struct {
	Line   int
	Input  map[string]any
	Output any
}

// ReadTable reads CSV or TSV text whose first row names the columns: each of
// the function's inputs and its output, in any order, matched without regard
// to case. Blank lines are skipped. Every row must read cleanly.
func ReadTable(f *Function, text string) ([]Row, error) {
	records, lines, err := readRecords(text)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("empty file; the first row names the columns: %s", strings.Join(f.Columns(), ", "))
	}
	want := f.Columns()
	order := make([]int, len(want)) // order[i] is the file column for want[i].
	for i := range order {
		order[i] = -1
	}
	for col, name := range records[0] {
		name = strings.TrimSpace(name)
		found := false
		for i, w := range want {
			if strings.EqualFold(name, w) {
				if order[i] >= 0 {
					return nil, fmt.Errorf("column %s appears twice", w)
				}
				order[i], found = col, true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown column %q; %s has columns %s", name, f.Signature, strings.Join(want, ", "))
		}
	}
	for i, at := range order {
		if at < 0 {
			return nil, fmt.Errorf("missing column %s; the first row names the columns: %s", want[i], strings.Join(want, ", "))
		}
	}
	var rows []Row
	for n, rec := range records[1:] {
		line := lines[n+1]
		if blank(rec) {
			continue
		}
		if len(rec) != len(records[0]) {
			return nil, fmt.Errorf("line %d: expected %d cells, got %d", line, len(records[0]), len(rec))
		}
		cells := make([]string, len(want))
		for i, at := range order {
			cells[i] = rec[at]
		}
		input, out, err := ParseRow(f, cells)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rows = append(rows, Row{Line: line, Input: input, Output: out})
	}
	return rows, nil
}

func blank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// ReadRows reads pasted or file text into rows. When the first row names any
// of the function's columns it is a header, read as by ReadTable; otherwise
// every row holds the inputs in signature order, then the output.
func ReadRows(f *Function, text string) ([]Row, error) {
	records, lines, err := readRecords(text)
	if err != nil {
		return nil, err
	}
	if len(records) > 0 && namesColumn(f, records[0]) {
		return ReadTable(f, text)
	}
	var rows []Row
	for n, rec := range records {
		if blank(rec) {
			continue
		}
		input, out, err := ParseRow(f, rec)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lines[n], err)
		}
		rows = append(rows, Row{Line: lines[n], Input: input, Output: out})
	}
	return rows, nil
}

func namesColumn(f *Function, rec []string) bool {
	for _, cell := range rec {
		for _, name := range f.Columns() {
			if strings.EqualFold(strings.TrimSpace(cell), name) {
				return true
			}
		}
	}
	return false
}
