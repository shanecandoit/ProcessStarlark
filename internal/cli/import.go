package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

// MaxImportRows bounds one import, so a stray file can't flood the queue.
const MaxImportRows = 1000

// imported is one new row, with what the function's code says about it.
type imported struct {
	row      project.Row
	call     string
	code     any    // The code's answer, when ran and err == "".
	ran      bool   // The function has code and it was run.
	codeErr  string // Why the code gave no answer.
	disagree bool
}

// importRows reads a CSV or TSV table for one function. By default each new
// row becomes a proposed answer in the question queue, for a person to
// confirm; with add, each becomes an example. Rows matching an example are
// skipped and rows contradicting one are reported, never applied.
func (c *CLI) importRows(ctx context.Context, p *project.Project, args []string) error {
	fs := c.flags("import")
	add := fs.Bool("add", false, "add new rows as examples instead of proposing them")
	fs.Usage = func() {
		fmt.Fprintln(c.Err, "Usage: park import [--add] P.fn FILE\n\nFILE is CSV or TSV; its first row names the function's inputs and output.")
		fs.PrintDefaults()
	}
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := exact(fs, 2); err != nil {
		return err
	}
	qualified := fs.Arg(0)
	_, f, err := p.Function(qualified)
	if err != nil {
		return fail(2, "%v", err)
	}
	b, err := readLimited(fs.Arg(1))
	if err != nil {
		return fail(2, "%v", err)
	}
	rows, err := project.ReadTable(f, string(b))
	if err != nil {
		return fail(2, "%s: %v", fs.Arg(1), err)
	}
	if len(rows) == 0 {
		return fail(2, "%s: no rows under the header", fs.Arg(1))
	}
	if len(rows) > MaxImportRows {
		return fail(2, "%s: %d rows; import at most %d at a time", fs.Arg(1), len(rows), MaxImportRows)
	}

	// Sort rows against the examples and against each other.
	var fresh []*imported
	var conflicts []string
	known := 0
	first := map[string]project.Row{}
	for _, r := range rows {
		key := samples.Key(r.Input)
		call := project.CallText(qualified, f, r.Input)
		if prev, ok := first[key]; ok {
			if samples.Key(prev.Output) != samples.Key(r.Output) {
				conflicts = append(conflicts, fmt.Sprintf("%s is %s on line %d, %s on line %d", call, samples.Literal(prev.Output), prev.Line, samples.Literal(r.Output), r.Line))
			}
			continue
		}
		first[key] = r
		if existing, ok := f.Index[key]; ok {
			if samples.Key(existing) == samples.Key(r.Output) {
				known++
			} else {
				conflicts = append(conflicts, fmt.Sprintf("line %d: %s is %s in the examples, %s in the file", r.Line, call, samples.Literal(existing), samples.Literal(r.Output)))
			}
			continue
		}
		fresh = append(fresh, &imported{row: r, call: call})
	}
	// A call that two lines answer differently is not imported at all.
	fresh = dropContested(fresh, rows, first)

	if f.Implemented {
		if err := compare(ctx, p, qualified, fresh); err != nil {
			return err
		}
	}

	agree, disagree := 0, 0
	for _, r := range fresh {
		if r.disagree {
			disagree++
		} else if r.ran && r.codeErr == "" {
			agree++
		}
	}
	report := func() {
		fmt.Fprintf(c.Out, "Read %d %s for %s: %d new, %d already in the examples, %d conflicting.\n", len(rows), plural(len(rows), "row"), qualified, len(fresh), known, len(conflicts))
		if f.Implemented && len(fresh) > 0 {
			fmt.Fprintf(c.Out, "The code agrees with %d new %s and disagrees with %d.\n", agree, plural(agree, "row"), disagree)
		}
		for _, conflict := range conflicts {
			fmt.Fprintf(c.Err, "  conflict, not imported: %s\n", conflict)
		}
		for _, r := range fresh {
			switch {
			case r.disagree:
				fmt.Fprintf(c.Err, "  code disagrees: %s is %s in the file; the code says %s\n", r.call, samples.Literal(r.row.Output), samples.Literal(r.code))
			case r.codeErr != "":
				fmt.Fprintf(c.Err, "  code has no answer: %s: %s\n", r.call, r.codeErr)
			}
		}
	}

	if *add {
		edits, err := addAll(p, qualified, fresh)
		if err != nil {
			return err
		}
		report()
		if len(edits) == 0 {
			fmt.Fprintln(c.Out, "Nothing to add.")
			return nil
		}
		if err := c.publish(p, edits); err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Added %d %s to %s.\n", len(fresh), plural(len(fresh), "example"), f.Path)
		return nil
	}

	// Proposals for a ready function would leave the queue at once: its code
	// answers every call. Only the disagreements need a decision then.
	if f.Ready() {
		report()
		fmt.Fprintf(c.Out, "%s is ready, so its code already answers these calls and nothing was queued.\n", qualified)
		if disagree > 0 {
			fmt.Fprintf(c.Out, "To keep the file's answers where the code disagrees, fix the code or rerun with --add (examples outrank code).\n")
		}
		return nil
	}
	var questions []project.Question
	for _, r := range fresh {
		q, err := project.NewProposal(p, qualified, r.row.Input, r.row.Output)
		if err != nil {
			return err
		}
		questions = append(questions, q)
	}
	report()
	if len(questions) == 0 {
		fmt.Fprintln(c.Out, "Nothing to propose.")
		return nil
	}
	if err := c.publish(p, nil, questions...); err != nil {
		return err
	}
	fmt.Fprintf(c.Out, "Proposed %d %s in %s. Confirm each that is right:\n  park example add '<line>'\n", len(questions), plural(len(questions), "answer"), project.QuestionsFile)
	if f.Implemented {
		fmt.Fprintf(c.Out, "Once %s has two examples its code answers these calls, and their proposals leave the queue.\n", qualified)
	}
	return nil
}

// dropContested removes new rows whose call appears again in the file with a
// different answer.
func dropContested(fresh []*imported, rows []project.Row, first map[string]project.Row) []*imported {
	contested := map[string]bool{}
	for _, r := range rows {
		key := samples.Key(r.Input)
		if samples.Key(first[key].Output) != samples.Key(r.Output) {
			contested[key] = true
		}
	}
	kept := fresh[:0]
	for _, r := range fresh {
		if !contested[samples.Key(r.row.Input)] {
			kept = append(kept, r)
		}
	}
	return kept
}

// compare runs the function's code on each new row, without the cache or
// resolvers, and notes where it disagrees with the file.
func compare(ctx context.Context, p *project.Project, qualified string, fresh []*imported) error {
	e, err := engine.New(p)
	if err != nil {
		return err
	}
	e.Testing = true
	for _, r := range fresh {
		callCtx, cancel := context.WithTimeout(ctx, engine.Timeout)
		out, err := e.Run(callCtx, qualified, r.row.Input)
		cancel()
		r.ran = true
		if err != nil {
			r.codeErr = firstLine(err.Error())
			continue
		}
		r.code = out
		r.disagree = samples.Key(out) != samples.Key(r.row.Output)
	}
	return nil
}

// addAll writes each new row as an example in the function's module.
func addAll(p *project.Project, qualified string, fresh []*imported) (map[string][]byte, error) {
	edits := map[string][]byte{}
	cur := p
	for _, r := range fresh {
		path, b, err := project.AddExample(cur, qualified, r.row.Input, r.row.Output)
		if err != nil {
			return nil, err
		}
		edits[path] = b
		if cur, err = project.LoadWith(p.Root, edits); err != nil {
			return nil, err
		}
	}
	return edits, nil
}

func readLimited(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > project.MaxFileBytes {
		return nil, fmt.Errorf("%s: not a regular file under %d bytes", path, project.MaxFileBytes)
	}
	return os.ReadFile(path)
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
