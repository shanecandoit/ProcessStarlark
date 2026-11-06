package workspace

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"github.com/shanecandoit/ProcessStarlark/internal/source"
)

// MaxPasteRows bounds one paste, so a stray paste can't flood a module.
const MaxPasteRows = 500

// Table editing: the playground shows each function as a table (inputs, then
// the output) and lets people edit it like a spreadsheet. Cells are typed as
// text, so `US` in a column of strings means "US", and `02134` stays a string
// there. Every edit is an ordinary example line in the function's module.

// cell turns typed text into a value. A column whose existing values are all
// strings (or all whole numbers) takes that type; otherwise the cell is read
// as a Starlark literal if it is one, and as a string if not.
func cell(text string, column []any) (any, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("empty cell; write \"\" for empty text")
	}
	literal, litErr := source.ParseValue(trimmed)
	switch kind(column) {
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

func kind(column []any) string {
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

// row binds typed cells (one per input, then the output) to a function.
func row(f *project.Function, cells []string) (map[string]any, any, error) {
	if len(cells) != len(f.Inputs)+1 {
		return nil, nil, fmt.Errorf("expected %d cells (%s, then %s), got %d", len(f.Inputs)+1, strings.Join(f.Inputs, ", "), f.Output, len(cells))
	}
	input := map[string]any{}
	for i, name := range f.Inputs {
		var column []any
		for _, ex := range f.Examples {
			column = append(column, ex.Input[name])
		}
		v, err := cell(cells[i], column)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		input[name] = v
	}
	var column []any
	for _, ex := range f.Examples {
		column = append(column, ex.Output)
	}
	out, err := cell(cells[len(cells)-1], column)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", f.Output, err)
	}
	return input, out, nil
}

// PutExample adds a table row, or rewrites the row whose call is replace.
// cells holds one typed cell per input, then the output.
func PutExample(ctx context.Context, files Files, function string, cells []string, replace string) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	_, f, err := p.Function(function)
	if err != nil {
		return failure(files, err)
	}
	input, out, err := row(f, cells)
	if err != nil {
		return failure(files, err)
	}
	var path string
	var b []byte
	if replace == "" {
		path, b, err = project.AddExample(p, function, input, out)
	} else {
		var qualified string
		var old map[string]any
		if qualified, old, err = parseCall(p, replace); err == nil && qualified != function {
			err = fmt.Errorf("%s is not a row of %s", replace, function)
		}
		if err == nil {
			path, b, err = project.ReplaceExample(p, function, old, input, out)
		}
	}
	if err != nil {
		return failure(files, err)
	}
	call := project.FormatCall(function, f.Inputs, input)
	r := edit(files, path, b, "Saved "+call+" == "+samples.Literal(out))
	return disagreements(ctx, r, function)
}

// PasteExamples adds rows pasted from a spreadsheet: tab-separated (or
// comma-separated) cells, one row per line, inputs then the output. A first
// row naming the columns is skipped. Rows already in the table are skipped;
// rows that would change an existing answer are reported, not applied. Nothing
// is written unless every row reads cleanly.
func PasteExamples(ctx context.Context, files Files, function, text string) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	_, f, err := p.Function(function)
	if err != nil {
		return failure(files, err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimRight(text, "\r\n")))
	reader.Comma = ','
	if strings.Contains(text, "\t") {
		reader.Comma = '\t'
	}
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	records, err := reader.ReadAll()
	if err != nil {
		return failure(files, fmt.Errorf("pasted rows: %w", err))
	}
	header := append(append([]string{}, f.Inputs...), f.Output)
	if len(records) > 0 && strings.EqualFold(strings.Join(trim(records[0]), "\t"), strings.Join(header, "\t")) {
		records = records[1:]
	}
	if len(records) == 0 {
		return failure(files, fmt.Errorf("nothing to paste"))
	}
	if len(records) > MaxPasteRows {
		return failure(files, fmt.Errorf("%d rows; paste at most %d at a time", len(records), MaxPasteRows))
	}
	type pasted struct {
		input map[string]any
		out   any
	}
	var rows []pasted
	for i, rec := range records {
		if len(trim(rec)) == 1 && trim(rec)[0] == "" {
			continue // blank line
		}
		input, out, err := row(f, rec)
		if err != nil {
			return failure(files, fmt.Errorf("pasted row %d: %w", i+1, err))
		}
		rows = append(rows, pasted{input, out})
	}
	added, same := 0, 0
	var conflicts []string
	seen := map[string]bool{}
	next := files
	for _, r := range rows {
		key := samples.Key(r.input)
		call := project.FormatCall(function, f.Inputs, r.input)
		if existing, ok := f.Index[key]; ok || seen[key] {
			if ok && samples.Key(existing) == samples.Key(r.out) {
				same++
			} else if ok {
				conflicts = append(conflicts, fmt.Sprintf("%s is %s, pasted %s", call, samples.Literal(existing), samples.Literal(r.out)))
			}
			continue
		}
		seen[key] = true
		cur, err := load(next)
		if err != nil {
			return failure(files, err)
		}
		path, b, err := project.AddExample(cur, function, r.input, r.out)
		if err != nil {
			return failure(files, err)
		}
		next = clone(next)
		next[path] = string(b)
		added++
	}
	parts := []string{fmt.Sprintf("Added %d %s to %s.", added, plural(added, "row"), function)}
	if same > 0 {
		parts = append(parts, fmt.Sprintf("%d already there.", same))
	}
	if len(conflicts) > 0 {
		parts = append(parts, fmt.Sprintf("Left %d unchanged, because the table already answers differently: %s.", len(conflicts), strings.Join(conflicts, "; ")))
	}
	published, snap, err := publish(next, nil)
	if err != nil {
		return failure(files, err)
	}
	r := Result{Status: OK, Message: strings.Join(parts, " "), Files: published, Snapshot: snap}
	return disagreements(ctx, r, function)
}

// disagreements runs a function's code against its examples after a table
// edit, and notes any row the code answers differently. The rows still stand:
// examples outrank code.
func disagreements(ctx context.Context, r Result, function string) Result {
	if r.Status == Error {
		return r
	}
	p, err := load(r.Files)
	if err != nil {
		return r
	}
	if _, f, err := p.Function(function); err != nil || !f.Implemented {
		return r
	}
	report, err := engine.Check(ctx, p, function)
	if err != nil {
		return r
	}
	for _, line := range report.Lines {
		if line.Failed {
			r.Lines = append(r.Lines, Line{Failed: true, Text: line.Text})
		}
	}
	if n := len(r.Lines); n > 0 {
		r.Message += fmt.Sprintf(" The code answers %d %s differently. The table still wins, and park test will flag the difference.", n, plural(n, "row"))
	}
	return r
}

func trim(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
