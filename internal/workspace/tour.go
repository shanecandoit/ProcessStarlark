package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

// A Tour is a guided walk through a project: a starting project and steps the
// reader completes. Tours are data; the page plays any tour, and Prepare checks
// at build time that every step can be completed. See docs/plan-guided-tours.md.
type Tour struct {
	ID    string     `json:"id"`
	Title string     `json:"title"`
	Start string     `json:"start"` // Directory of the starting project, at build time.
	Files Files      `json:"files"` // The starting project, filled in by the build.
	Steps []TourStep `json:"steps"`
}

type TourStep struct {
	Role    string   `json:"role,omitempty"`    // "ops", "dev", or empty.
	Text    string   `json:"text"`              // What to do and why.
	Snippet string   `json:"snippet,omitempty"` // Source to type, shown as code.
	Call    string   `json:"call,omitempty"`    // Prefills the call box.
	Open    string   `json:"open,omitempty"`    // file.star or file.star:line to show.
	Point   []string `json:"point,omitempty"`   // Named places on the page to highlight.
	Goal    *Goal    `json:"goal,omitempty"`    // Completes the step; without one the step shows Next.
	Skip    []TourOp `json:"skip,omitempty"`    // Operations that complete the step for the reader.
	// Filled in by Prepare: the project and last run at the start of this step.
	Checkpoint     Files  `json:"checkpoint,omitempty"`
	CheckpointCall string `json:"checkpointCall,omitempty"`
}

// A Goal is a condition on the project and the last run. Set fields must all
// hold.
type Goal struct {
	Ran      string `json:"ran,omitempty"`      // The last run was this call...
	Output   string `json:"output,omitempty"`   // ...and returned this literal (with Ran or Example).
	Status   string `json:"status,omitempty"`   // The last run had this status.
	Question string `json:"question,omitempty"` // This call is an open question.
	Example  string `json:"example,omitempty"`  // Some function has an example for this call.
	Function string `json:"function,omitempty"` // This Process.function is declared.
	Not      *Goal  `json:"not,omitempty"`      // This goal does not hold.
	All      []Goal `json:"all,omitempty"`      // Every goal holds.
}

// TourOp is one skip operation. Exactly one field is set.
type TourOp struct {
	Run        string       `json:"run,omitempty"`
	AddExample string       `json:"addExample,omitempty"`
	Dismiss    string       `json:"dismiss,omitempty"`
	Paste      *Paste       `json:"paste,omitempty"`
	SetFile    *FileText    `json:"setFile,omitempty"`
	Replace    *Replacement `json:"replace,omitempty"`
}

// Paste pastes spreadsheet rows into a function's table.
type Paste struct {
	Function string `json:"function"`
	Text     string `json:"text"`
}

type FileText struct {
	File string `json:"file"`
	Text string `json:"text"`
}

type Replacement struct {
	File string `json:"file"`
	Find string `json:"find"` // Must occur exactly once.
	With string `json:"with"`
}

// LastRun is what the page remembers about the most recent run.
type LastRun struct {
	Call   string `json:"call"`
	Status string `json:"status"`
	Output string `json:"output"`
}

// Met reports whether a goal holds for the files and the last run.
func Met(files Files, last LastRun, g Goal) bool {
	if g.Not != nil && Met(files, last, *g.Not) {
		return false
	}
	for _, sub := range g.All {
		if !Met(files, last, sub) {
			return false
		}
	}
	if g.Status != "" && last.Status != g.Status {
		return false
	}
	if g.Ran != "" {
		if !sameCall(files, last.Call, g.Ran) || (g.Output != "" && last.Output != g.Output) {
			return false
		}
	}
	if g.Question != "" {
		found := false
		for _, q := range Load(files).Snapshot.Questions {
			found = found || sameCall(files, q.Call, g.Question)
		}
		if !found {
			return false
		}
	}
	if g.Function != "" {
		p, err := load(files)
		if err != nil {
			return false
		}
		if _, _, err := p.Function(g.Function); err != nil {
			return false
		}
	}
	if g.Example != "" {
		p, err := load(files)
		if err != nil {
			return false
		}
		qualified, input, err := parseCall(p, g.Example)
		if err != nil {
			return false
		}
		_, f, _ := p.Function(qualified)
		out, ok := f.Index[samples.Key(input)]
		if !ok || (g.Output != "" && samples.Literal(out) != g.Output) {
			return false
		}
	}
	return true
}

// sameCall compares calls after binding, so positional and named arguments
// match. Calls that don't bind compare as text.
func sameCall(files Files, a, b string) bool {
	if strings.TrimSpace(a) == strings.TrimSpace(b) {
		return true
	}
	p, err := load(files)
	if err != nil {
		return false
	}
	qa, ia, err := parseCall(p, a)
	if err != nil {
		return false
	}
	qb, ib, err := parseCall(p, b)
	return err == nil && qa == qb && samples.Key(ia) == samples.Key(ib)
}

// Apply performs one skip operation, returning the new files and last run.
func Apply(ctx context.Context, files Files, last LastRun, op TourOp) (Files, LastRun, error) {
	check := func(r Result) (Files, LastRun, error) {
		if r.Status == Error {
			return files, last, fmt.Errorf("%s", r.Message)
		}
		return r.Files, last, nil
	}
	switch {
	case op.Run != "":
		r := Run(ctx, files, op.Run)
		if r.Status == Error {
			return files, last, fmt.Errorf("run %s: %s", op.Run, r.Message)
		}
		return r.Files, LastRun{Call: op.Run, Status: r.Status, Output: r.Output}, nil
	case op.AddExample != "":
		return check(AddExample(files, op.AddExample))
	case op.Dismiss != "":
		return check(Dismiss(files, op.Dismiss))
	case op.Paste != nil:
		return check(PasteExamples(ctx, files, op.Paste.Function, op.Paste.Text))
	case op.SetFile != nil:
		next := clone(files)
		next[op.SetFile.File] = op.SetFile.Text
		return check(Load(next))
	case op.Replace != nil:
		text, ok := files[op.Replace.File]
		if !ok || strings.Count(text, op.Replace.Find) != 1 {
			return files, last, fmt.Errorf("%s: expected one %q", op.Replace.File, op.Replace.Find)
		}
		next := clone(files)
		next[op.Replace.File] = strings.Replace(text, op.Replace.Find, op.Replace.With, 1)
		return check(Load(next))
	}
	return files, last, fmt.Errorf("empty skip operation")
}

// Prepare replays a tour from its starting files. Each step with a goal must
// have a skip; the goal must not hold before the skip and must hold after it.
// It records each step's checkpoint: the project and last run at its start.
func Prepare(ctx context.Context, t *Tour) error {
	files, last := clone(t.Files), LastRun{}
	if r := Load(files); r.Status == Error {
		return fmt.Errorf("tour %s: start: %s", t.ID, r.Message)
	}
	for i := range t.Steps {
		s := &t.Steps[i]
		s.Checkpoint, s.CheckpointCall = clone(files), last.Call
		where := fmt.Sprintf("tour %s step %d", t.ID, i+1)
		if s.Goal == nil {
			if len(s.Skip) > 0 {
				return fmt.Errorf("%s: a skip needs a goal", where)
			}
			continue
		}
		if len(s.Skip) == 0 {
			return fmt.Errorf("%s: a goal needs a skip", where)
		}
		if Met(files, last, *s.Goal) {
			return fmt.Errorf("%s: goal already met before the step", where)
		}
		for _, op := range s.Skip {
			var err error
			if files, last, err = Apply(ctx, files, last, op); err != nil {
				return fmt.Errorf("%s: %w", where, err)
			}
		}
		if !Met(files, last, *s.Goal) {
			g, _ := json.Marshal(s.Goal)
			return fmt.Errorf("%s: goal %s not met after the skip (last run %+v)", where, g, last)
		}
	}
	return nil
}
