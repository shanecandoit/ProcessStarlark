package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
)

// MaxImportRows bounds one import, so a stray file can't flood the queue.
const MaxImportRows = 1000

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
	plan, err := engine.PlanImport(ctx, p, qualified, rows)
	if err != nil {
		return err
	}
	raised := plan.Raised
	report := func() {
		fmt.Fprintf(c.Out, "Read %d %s for %s: %d new, %d already in the examples, %d conflicting.\n", plan.Rows, plural(plan.Rows, "row"), qualified, len(plan.Fresh), plan.Known, len(plan.Conflicts))
		if plan.Agree+plan.Disagree > 0 {
			fmt.Fprintf(c.Out, "The code agrees with %d new %s and disagrees with %d.\n", plan.Agree, plural(plan.Agree, "row"), plan.Disagree)
		}
		for _, note := range plan.Notes() {
			fmt.Fprintf(c.Err, "  %s\n", note)
		}
		if len(raised) > 0 {
			fmt.Fprintf(c.Err, "Queued %d %s the code raised in %s; see park list.\n", len(raised), plural(len(raised), "question"), project.QuestionsFile)
		}
	}
	// queueRaised publishes the code's questions when nothing else is written.
	queueRaised := func() error {
		if len(raised) == 0 {
			return nil
		}
		return c.publish(p, nil, raised...)
	}

	if *add {
		edits, err := addAll(p, qualified, plan.Fresh)
		if err != nil {
			return err
		}
		if len(edits) == 0 {
			if err := queueRaised(); err != nil {
				return err
			}
			report()
			fmt.Fprintln(c.Out, "Nothing to add.")
			return nil
		}
		if err := c.publish(p, edits, raised...); err != nil {
			return err
		}
		report()
		fmt.Fprintf(c.Out, "Added %d %s to %s.\n", len(plan.Fresh), plural(len(plan.Fresh), "example"), f.Path)
		return nil
	}

	// Proposals for a ready function would leave the queue at once: its code
	// answers every call. Only the disagreements need a decision then.
	if f.Ready() {
		if err := queueRaised(); err != nil {
			return err
		}
		report()
		fmt.Fprintf(c.Out, "%s is ready, so its code already answers these calls and nothing was queued.\n", qualified)
		if plan.Disagree > 0 {
			fmt.Fprintf(c.Out, "To keep the file's answers where the code disagrees, fix the code or rerun with --add (examples outrank code).\n")
		}
		return nil
	}
	questions, err := plan.Proposals(p)
	if err != nil {
		return err
	}
	if len(questions) == 0 {
		if err := queueRaised(); err != nil {
			return err
		}
		report()
		fmt.Fprintln(c.Out, "Nothing to propose.")
		return nil
	}
	// The file's answers go last so they replace any proposal the code made
	// for the same call.
	if err := c.publish(p, nil, append(raised, questions...)...); err != nil {
		return err
	}
	report()
	fmt.Fprintf(c.Out, "Proposed %d %s in %s. Confirm each that is right:\n  park example add '<line>'\n", len(questions), plural(len(questions), "answer"), project.QuestionsFile)
	if f.Implemented {
		fmt.Fprintf(c.Out, "Once %s has two examples its code answers these calls, and their proposals leave the queue.\n", qualified)
	}
	return nil
}

// addAll writes each new row as an example in the function's module.
func addAll(p *project.Project, qualified string, fresh []*engine.ImportRow) (map[string][]byte, error) {
	edits := map[string][]byte{}
	cur := p
	for _, r := range fresh {
		path, b, err := project.AddExample(cur, qualified, r.Row.Input, r.Row.Output)
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
