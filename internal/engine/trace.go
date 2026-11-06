package engine

import (
	"fmt"
	"io"
	"strings"

	"park/internal/samples"
)

// Where an answer came from.
const (
	SourceExample     = "example"          // A person wrote it.
	SourceCode        = "code"             // Computed by code with two or more examples.
	SourceUnconfirmed = "unconfirmed code" // Computed by code with fewer than two examples.
	SourceUnknown     = "unknown"          // Nothing could answer it; it becomes a question.
)

// Step is one dispatched call in a run's provenance tree: the call, its
// answer, where the answer came from, and the calls made to compute it.
type Step struct {
	Function string
	Inputs   []string // Input names in signature order.
	Input    map[string]any
	Output   any
	Source   string // One of the Source constants; empty if the call never bound.
	Path     string // The example's location, or the function's def.
	Line     int
	Failed   bool   // The call did not return.
	Error    string // Set on the innermost failure only.
	Steps    []*Step
}

// Trace returns the provenance tree of the last Run, or nil unless Why is set.
func (e *Engine) Trace() *Step { return e.trace }

// Call renders the step's call as Process.fn(name=value, ...).
func (s *Step) Call() string {
	if s.Input == nil {
		return s.Function + "(...)"
	}
	return FormatCall(s.Function, s.Inputs, s.Input)
}

// Answer renders the step's output, or "?" if it has none.
func (s *Step) Answer() string {
	if s.Failed {
		return "?"
	}
	return samples.Literal(s.Output)
}

// Location renders path:line, or "" when unknown.
func (s *Step) Location() string {
	if s.Path == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", s.Path, s.Line)
}

// WriteTrace prints a provenance tree, one call per line, indented by depth,
// with sources and locations aligned in columns.
func WriteTrace(w io.Writer, root *Step) error {
	type row struct{ call, source, where string }
	var rows []row
	var walk func(*Step, int)
	walk = func(s *Step, depth int) {
		source := s.Source
		if s.Error != "" {
			source = "failed"
		}
		where := s.Location()
		if s.Error != "" {
			where = strings.TrimSpace(where + "  " + firstLine(s.Error))
		}
		rows = append(rows, row{strings.Repeat("  ", depth+1) + s.Call() + " = " + s.Answer(), source, where})
		for _, child := range s.Steps {
			walk(child, depth+1)
		}
	}
	walk(root, 0)
	callWidth, sourceWidth := 0, 0
	for _, r := range rows {
		callWidth = max(callWidth, len(r.call))
		sourceWidth = max(sourceWidth, len(r.source))
	}
	for _, r := range rows {
		line := fmt.Sprintf("%-*s  %-*s  %s", callWidth, r.call, sourceWidth, r.source, r.where)
		if _, err := fmt.Fprintln(w, strings.TrimRight(line, " ")); err != nil {
			return err
		}
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
