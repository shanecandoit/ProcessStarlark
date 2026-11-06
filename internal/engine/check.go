package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
)

// Timeout bounds each top-level evaluation and each test example.
const Timeout = 5 * time.Second

// CheckLine is one line of a test report. Failed lines are failures; the rest
// describe example-only records.
type CheckLine struct {
	Failed bool
	Text   string
}

// Report is the outcome of checking implementations against their examples.
type Report struct {
	Checks, Records, Failures int
	Lines                     []CheckLine
	// Questions raised while checking: dependency misses and unconfirmed
	// answers from dependencies, for the caller to queue.
	Questions []project.Question
}

// Summary is the report's last line, as the CLI prints it.
func (r Report) Summary() string {
	return fmt.Sprintf("%d implementation checks, %d example-only records, %d failures", r.Checks, r.Records, r.Failures)
}

// Check runs every implementation against each of its authored examples,
// bypassing the function's own examples and the computed cache. With only
// set to Process.function, it checks just that function.
func Check(ctx context.Context, p *project.Project, only string) (Report, error) {
	var r Report
	if only != "" {
		if _, _, err := p.Function(only); err != nil {
			return r, err
		}
	}
	e, err := New(p)
	if err != nil {
		return r, err
	}
	e.Testing = true
	for _, name := range project.Names(p.Processes) {
		proc := p.Processes[name]
		for _, fn := range project.Names(proc.Functions) {
			qualified := name + "." + fn
			if only != "" && only != qualified {
				continue
			}
			f := proc.Functions[fn]
			if !f.Implemented {
				r.Records += len(f.Examples)
				r.Lines = append(r.Lines, CheckLine{Text: fmt.Sprintf("DATA %s: %d example-only records", qualified, len(f.Examples))})
				continue
			}
			if !f.Ready() {
				r.Failures++
				r.Lines = append(r.Lines, CheckLine{Failed: true, Text: fmt.Sprintf("FAIL %s: needs examples", qualified)})
				continue
			}
			e.Bypass = qualified
			for _, pair := range f.Examples {
				r.Checks++
				callCtx, cancel := context.WithTimeout(ctx, Timeout)
				output, err := e.Run(callCtx, qualified, pair.Input)
				cancel()
				var unresolved *Unresolved
				if errors.As(err, &unresolved) {
					if q, qerr := project.NewQuestion(p, unresolved.Function, unresolved.Input); qerr == nil {
						r.Questions = append(r.Questions, q)
					}
				}
				for _, pr := range e.Proposals() {
					if q, qerr := project.NewProposal(p, pr.Function, pr.Input, pr.Output); qerr == nil {
						r.Questions = append(r.Questions, q)
					}
				}
				call := FormatCall(qualified, f.Inputs, pair.Input)
				if err != nil {
					r.Failures++
					r.Lines = append(r.Lines, CheckLine{Failed: true, Text: fmt.Sprintf("FAIL %s (%s:%d): %v", call, pair.Path, pair.Line, err)})
				} else if samples.Key(output) != samples.Key(pair.Output) {
					r.Failures++
					r.Lines = append(r.Lines, CheckLine{Failed: true, Text: fmt.Sprintf("FAIL %s (%s:%d): got %s; expected %s", call, pair.Path, pair.Line, samples.Literal(output), samples.Literal(pair.Output))})
				}
			}
		}
	}
	return r, nil
}
