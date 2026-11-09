package workspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

// MaxPasteRows bounds one paste, so a stray paste can't flood a module.
const MaxPasteRows = 500

// Table editing: the playground shows each function as a table (inputs, then
// the output) and lets people edit it like a spreadsheet. Cells are typed as
// text (see project.ParseCell). Every edit is an ordinary example line in the
// function's module.

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
	input, out, err := project.ParseRow(f, cells)
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

// PasteExamples adds pasted rows as examples; see ImportRows.
func PasteExamples(ctx context.Context, files Files, function, text string) Result {
	return ImportRows(ctx, files, function, text, false)
}

// ImportRows takes rows pasted from a spreadsheet or read from a CSV/TSV file:
// tab- or comma-separated cells, one row per line. A first row naming the
// columns is a header and may put them in any order (see project.ReadRows);
// without one, rows hold the inputs, then the output.
//
// With propose, new rows become proposed answers in the question queue, as
// `park import` does; otherwise they become examples, as `park import --add`
// does. Either way rows already in the table are skipped, and rows that
// contradict it, or calls answered two ways, are reported and not applied.
// Nothing is written unless every row reads cleanly. Questions the function's
// code raises on the new rows (calls its dependencies can't answer) are
// queued too.
func ImportRows(ctx context.Context, files Files, function, text string, propose bool) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	_, f, err := p.Function(function)
	if err != nil {
		return failure(files, err)
	}
	rows, err := project.ReadRows(f, text)
	if err != nil {
		return failure(files, fmt.Errorf("rows: %w", err))
	}
	if len(rows) == 0 {
		return failure(files, fmt.Errorf("no rows to import"))
	}
	if len(rows) > MaxPasteRows {
		return failure(files, fmt.Errorf("%d rows; import at most %d at a time", len(rows), MaxPasteRows))
	}
	plan, err := engine.PlanImport(ctx, p, function, rows)
	if err != nil {
		return failure(files, err)
	}
	var parts []string
	var edits Files
	added := plan.Raised
	n := len(plan.Fresh)
	switch {
	case !propose:
		next := files
		for _, r := range plan.Fresh {
			cur, err := load(next)
			if err != nil {
				return failure(files, err)
			}
			path, b, err := project.AddExample(cur, function, r.Row.Input, r.Row.Output)
			if err != nil {
				return failure(files, err)
			}
			next = clone(next)
			next[path] = string(b)
		}
		edits = next
		parts = append(parts, fmt.Sprintf("Added %d %s to %s.", n, plural(n, "row"), function))
	case f.Ready():
		// Its code answers every call, so proposals would leave the queue at once.
		parts = append(parts, fmt.Sprintf("Proposed nothing: %s is ready, so its code already answers these calls.", function))
		if plan.Disagree > 0 {
			parts = append(parts, "To keep the table's answers where the code disagrees, fix the code or add them as examples (examples outrank code).")
		}
	default:
		questions, err := plan.Proposals(p)
		if err != nil {
			return failure(files, err)
		}
		// The table's answers go last so they replace the code's proposals.
		added = append(added, questions...)
		parts = append(parts, fmt.Sprintf("Proposed %d %s for %s; confirm each that is right in Questions.", n, plural(n, "answer"), function))
	}
	if plan.Known > 0 {
		parts = append(parts, fmt.Sprintf("%d already there.", plan.Known))
	}
	if c := len(plan.Conflicts); c > 0 {
		parts = append(parts, fmt.Sprintf("Left out %d %s that %s the table.", c, plural(c, "row"), map[bool]string{true: "contradicts", false: "contradict"}[c == 1]))
	}
	if plan.Agree+plan.Disagree > 0 {
		parts = append(parts, fmt.Sprintf("The code agrees with %d new %s and disagrees with %d.", plan.Agree, plural(plan.Agree, "row"), plan.Disagree))
	}
	if q := len(plan.Raised); q > 0 {
		parts = append(parts, fmt.Sprintf("Queued %d %s the code raised.", q, plural(q, "question")))
	}
	base := files
	if edits != nil {
		base = edits
	}
	published, snap, err := publish(base, nil, added...)
	if err != nil {
		return failure(files, err)
	}
	r := Result{Status: OK, Message: strings.Join(parts, " "), Files: published, Snapshot: snap}
	for _, note := range plan.Notes() {
		r.Lines = append(r.Lines, Line{Failed: true, Text: note})
	}
	return r
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

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
