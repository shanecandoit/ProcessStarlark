package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

// ImportRow is one new row of an import, with what the function's code says
// about it.
type ImportRow struct {
	Row      project.Row
	Call     string
	Code     any    // The code's answer, when Ran and CodeErr == "".
	Ran      bool   // The function has code and it was run.
	CodeErr  string // Why the code gave no answer.
	Disagree bool
}

// ImportPlan sorts a table's rows for one function against its examples and
// against each other, and runs the function's code on the new ones. Park
// import and the playground's paste and file import share it.
type ImportPlan struct {
	Function string
	Rows     int          // Rows read.
	Fresh    []*ImportRow // New calls, answered once in the table.
	Known    int          // Rows that match an example.
	// Conflicts describe rows contradicting an example, or calls the table
	// answers two ways. None of them is imported.
	Conflicts       []string
	Agree, Disagree int // Fresh rows the code agrees or disagrees with.
	// Raised holds questions the code raised while running the fresh rows,
	// as park test would: dependency calls nothing could answer, and
	// dependencies' unconfirmed answers. They should be queued however the
	// rows themselves end up.
	Raised []project.Question
}

// PlanImport builds the plan. It runs code without the cache or resolvers,
// and writes nothing.
func PlanImport(ctx context.Context, p *project.Project, qualified string, rows []project.Row) (ImportPlan, error) {
	plan := ImportPlan{Function: qualified, Rows: len(rows)}
	_, f, err := p.Function(qualified)
	if err != nil {
		return plan, err
	}
	first := map[string]project.Row{}
	contested := map[string]bool{}
	for _, r := range rows {
		key := samples.Key(r.Input)
		call := project.CallText(qualified, f, r.Input)
		if prev, ok := first[key]; ok {
			if samples.Key(prev.Output) != samples.Key(r.Output) {
				contested[key] = true
				plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("%s is %s on line %d, %s on line %d", call, samples.Literal(prev.Output), prev.Line, samples.Literal(r.Output), r.Line))
			}
			continue
		}
		first[key] = r
		if existing, ok := f.Index[key]; ok {
			if samples.Key(existing) == samples.Key(r.Output) {
				plan.Known++
			} else {
				plan.Conflicts = append(plan.Conflicts, fmt.Sprintf("line %d: %s is %s in the examples, %s in the table", r.Line, call, samples.Literal(existing), samples.Literal(r.Output)))
			}
			continue
		}
		plan.Fresh = append(plan.Fresh, &ImportRow{Row: r, Call: call})
	}
	// A call the table answers two ways is not imported at all.
	kept := plan.Fresh[:0]
	for _, r := range plan.Fresh {
		if !contested[samples.Key(r.Row.Input)] {
			kept = append(kept, r)
		}
	}
	plan.Fresh = kept
	if f.Implemented {
		if err := plan.compare(ctx, p); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

// compare runs the function's code on each fresh row and notes where it
// disagrees with the table. The rows' own calls are left out of Raised; the
// table answers those.
func (plan *ImportPlan) compare(ctx context.Context, p *project.Project) error {
	e, err := New(p)
	if err != nil {
		return err
	}
	e.Testing = true
	own := map[string]bool{}
	for _, r := range plan.Fresh {
		own[samples.Key(r.Row.Input)] = true
	}
	for _, r := range plan.Fresh {
		callCtx, cancel := context.WithTimeout(ctx, Timeout)
		out, err := e.Run(callCtx, plan.Function, r.Row.Input)
		cancel()
		r.Ran = true
		var unresolved *Unresolved
		if errors.As(err, &unresolved) {
			if q, qerr := project.NewQuestion(p, unresolved.Function, unresolved.Input); qerr == nil {
				plan.Raised = append(plan.Raised, q)
			}
		}
		for _, pr := range e.Proposals() {
			if pr.Function == plan.Function && own[samples.Key(pr.Input)] {
				continue
			}
			if q, qerr := project.NewProposal(p, pr.Function, pr.Input, pr.Output); qerr == nil {
				plan.Raised = append(plan.Raised, q)
			}
		}
		if err != nil {
			r.CodeErr = firstLine(err.Error())
			continue
		}
		r.Code = out
		r.Disagree = samples.Key(out) != samples.Key(r.Row.Output)
		if r.Disagree {
			plan.Disagree++
		} else {
			plan.Agree++
		}
	}
	return nil
}

// Proposals are the fresh rows as proposed answers for the question queue.
func (plan ImportPlan) Proposals(p *project.Project) ([]project.Question, error) {
	var questions []project.Question
	for _, r := range plan.Fresh {
		q, err := project.NewProposal(p, plan.Function, r.Row.Input, r.Row.Output)
		if err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, nil
}

// Notes describe, one per line, what was not imported and where the code
// disagrees or has no answer.
func (plan ImportPlan) Notes() []string {
	var notes []string
	for _, c := range plan.Conflicts {
		notes = append(notes, "conflict, not imported: "+c)
	}
	for _, r := range plan.Fresh {
		switch {
		case r.Disagree:
			notes = append(notes, fmt.Sprintf("code disagrees: %s is %s in the table; the code says %s", r.Call, samples.Literal(r.Row.Output), samples.Literal(r.Code)))
		case r.CodeErr != "":
			notes = append(notes, fmt.Sprintf("code has no answer: %s: %s", r.Call, r.CodeErr))
		}
	}
	return notes
}
