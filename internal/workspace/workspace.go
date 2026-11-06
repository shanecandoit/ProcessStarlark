// Package workspace runs Park operations on a project held in memory. It is
// the browser playground's engine room: the same validation, dispatch, test,
// question-queue, and catalog rules as the CLI, with no filesystem.
//
// Every operation takes the whole file map and returns a Result holding the
// new file map (example edits and question-queue updates applied) plus a
// snapshot of the project for display. No state is kept between calls.
package workspace

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/catalog"
	"github.com/shanecandoit/ProcessStarlark/internal/engine"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"github.com/shanecandoit/ProcessStarlark/internal/source"
)

// Files maps project file names (*.star and questions.txt) to their text.
type Files map[string]string

// Result statuses.
const (
	OK          = "ok"          // An answer confirmed by examples or ready code.
	Unconfirmed = "unconfirmed" // An answer from code with fewer than two examples.
	Unresolved  = "unresolved"  // No example and no code answered a call.
	Failed      = "failed"      // park test found failures.
	Error       = "error"       // Invalid project, call, or evaluation error.
)

type Result struct {
	Status   string   `json:"status"`
	Output   string   `json:"output,omitempty"`  // Starlark literal, for run.
	Message  string   `json:"message,omitempty"` // Error text or a note for the user.
	Lines    []Line   `json:"lines,omitempty"`   // Test report lines.
	Files    Files    `json:"files"`
	Snapshot Snapshot `json:"snapshot"`
	Archive  []byte   `json:"archive,omitempty"` // Zip bytes, from Archive (base64 in JSON).
	Why      *Step    `json:"why,omitempty"`     // Provenance of a run's answer, like park run --why.
}

// Step is one call in a run's provenance tree, rendered for display.
type Step struct {
	Call   string  `json:"call"`
	Output string  `json:"output"` // Starlark literal, or "?" if the call did not return.
	Source string  `json:"source"` // example, code, unconfirmed code, unknown, or "" if the call never bound.
	Path   string  `json:"path,omitempty"`
	Line   int     `json:"line,omitempty"`
	Error  string  `json:"error,omitempty"` // On the innermost failure only.
	Steps  []*Step `json:"steps,omitempty"`
}

func why(s *engine.Step) *Step {
	if s == nil {
		return nil
	}
	out := &Step{Call: s.Call(), Output: s.Answer(), Source: s.Source, Path: s.Path, Line: s.Line, Error: s.Error}
	for _, child := range s.Steps {
		out.Steps = append(out.Steps, why(child))
	}
	return out
}

type Line struct {
	Failed bool   `json:"failed"`
	Text   string `json:"text"`
}

// Snapshot describes the project as it stands after the operation.
type Snapshot struct {
	Valid     bool       `json:"valid"`
	Error     string     `json:"error,omitempty"` // Why the project is invalid.
	Revision  string     `json:"revision,omitempty"`
	Processes []Process  `json:"processes"`
	Questions []Question `json:"questions"`
	Catalog   string     `json:"catalog,omitempty"` // park.html contents.
}

type Process struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Module      string     `json:"module"`
	Uses        []string   `json:"uses"`
	Functions   []Function `json:"functions"`
}

type Function struct {
	Qualified   string    `json:"qualified"`
	Signature   string    `json:"signature"`
	State       string    `json:"state"`
	Description string    `json:"description,omitempty"`
	Path        string    `json:"path"`
	Line        int       `json:"line"`
	Inputs      []string  `json:"inputs"`
	Output      string    `json:"output"`
	Examples    []Example `json:"examples"`
}

// Example values are Starlark literals; Inputs follow the signature order.
type Example struct {
	Inputs []string `json:"inputs"`
	Output string   `json:"output"`
	Path   string   `json:"path"`
	Line   int      `json:"line"`
}

type Question struct {
	Line        string `json:"line"` // As queued; confirmable when HasProposal.
	Call        string `json:"call"`
	Function    string `json:"function"`
	Proposed    string `json:"proposed,omitempty"`
	HasProposal bool   `json:"hasProposal"`
}

func modules(files Files) map[string][]byte {
	m := map[string][]byte{}
	for name, text := range files {
		if strings.HasSuffix(name, ".star") {
			m[name] = []byte(text)
		}
	}
	return m
}

func load(files Files) (*project.Project, error) {
	p, err := project.FromFiles("", modules(files))
	if err != nil {
		return nil, err
	}
	// Loading the engine checks what project validation can't: module
	// initialization, reserved names, and references to unknown functions.
	if _, err := engine.New(p); err != nil {
		return nil, err
	}
	return p, nil
}

func clone(files Files) Files {
	out := Files{}
	for k, v := range files {
		out[k] = v
	}
	return out
}

// publish mirrors the CLI's publish: apply edits, validate the proposed
// project, merge added questions into the queue and drop answered ones, and
// render the catalog. On error nothing changes.
func publish(files Files, edits Files, added ...project.Question) (Files, Snapshot, error) {
	next := clone(files)
	for k, v := range edits {
		next[k] = v
	}
	p, err := load(next)
	if err != nil {
		return files, Snapshot{}, err
	}
	queued, _, err := project.ParseQuestions(p, []byte(next[project.QuestionsFile]))
	if err != nil {
		return files, Snapshot{}, err
	}
	open := project.OpenQuestions(p, queued, added)
	if b := project.FormatQuestions(open); b == nil {
		delete(next, project.QuestionsFile)
	} else {
		next[project.QuestionsFile] = string(b)
	}
	html, err := catalog.Render(p, open)
	if err != nil {
		return files, Snapshot{}, err
	}
	return next, describe(p, open, string(html)), nil
}

func describe(p *project.Project, open []project.Question, html string) Snapshot {
	s := Snapshot{Valid: true, Revision: p.Revision, Catalog: html, Processes: []Process{}, Questions: []Question{}}
	for _, name := range project.Names(p.Processes) {
		proc := p.Processes[name]
		out := Process{Name: name, Description: proc.Description, Module: proc.Module, Uses: append([]string{}, proc.Uses...), Functions: []Function{}}
		for _, fn := range project.Names(proc.Functions) {
			f := proc.Functions[fn]
			df := Function{Qualified: name + "." + fn, Signature: f.Signature, State: f.State(), Description: f.Description, Path: f.Path, Line: f.Line, Inputs: f.Inputs, Output: f.Output, Examples: []Example{}}
			for _, ex := range f.Examples {
				inputs := make([]string, len(f.Inputs))
				for i, in := range f.Inputs {
					inputs[i] = samples.Literal(ex.Input[in])
				}
				df.Examples = append(df.Examples, Example{Inputs: inputs, Output: samples.Literal(ex.Output), Path: ex.Path, Line: ex.Line})
			}
			out.Functions = append(out.Functions, df)
		}
		s.Processes = append(s.Processes, out)
	}
	for _, q := range open {
		dq := Question{Line: q.Line(), Call: q.Call, Function: q.Function, HasProposal: q.HasProposal}
		if q.HasProposal {
			dq.Proposed = samples.Literal(q.Proposed)
		}
		s.Questions = append(s.Questions, dq)
	}
	return s
}

func failure(files Files, err error) Result {
	return Result{Status: Error, Message: err.Error(), Files: files, Snapshot: invalid(files, err)}
}

// invalid describes a project whose current files don't load, so the page can
// still show the error. If the files do load, the error was about the
// operation, not the project, and the snapshot is the normal one.
func invalid(files Files, opErr error) Snapshot {
	if _, snap, err := publish(files, nil); err == nil {
		return snap
	} else {
		return Snapshot{Error: err.Error(), Processes: []Process{}, Questions: []Question{}}
	}
}

// Load validates the files, prunes answered questions, and describes the
// project. It is the operation to call after every edit to the source.
func Load(files Files) Result {
	next, snap, err := publish(files, nil)
	if err != nil {
		return Result{Status: Error, Message: err.Error(), Files: files, Snapshot: Snapshot{Error: err.Error(), Processes: []Process{}, Questions: []Question{}}}
	}
	return Result{Status: OK, Files: next, Snapshot: snap}
}

func bind(p *project.Project, parsed *source.Call) (string, map[string]any, error) {
	if parsed.Process == "" {
		return "", nil, fmt.Errorf("name the process too, e.g. Process.%s(...)", parsed.Function)
	}
	qualified := parsed.Process + "." + parsed.Function
	_, f, err := p.Function(qualified)
	if err != nil {
		return "", nil, err
	}
	input, err := f.BindArgs(qualified, parsed.Args, parsed.Names, parsed.Values)
	if err != nil {
		return "", nil, err
	}
	return qualified, input, nil
}

// Run evaluates one call, written as in `park run`. An unanswered call is
// queued as a question; answers from unconfirmed code are queued as proposals.
func Run(ctx context.Context, files Files, call string) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	parsed, err := source.ParseCall(call)
	if err != nil {
		return failure(files, err)
	}
	qualified, input, err := bind(p, parsed)
	if err != nil {
		return failure(files, err)
	}
	e, err := engine.New(p)
	if err != nil {
		return failure(files, err)
	}
	e.Why = true
	callCtx, cancel := context.WithTimeout(ctx, engine.Timeout)
	output, err := e.Run(callCtx, qualified, input)
	cancel()
	trace := why(e.Trace())
	var unresolved *engine.Unresolved
	if errors.As(err, &unresolved) {
		q, qerr := project.NewQuestion(p, unresolved.Function, unresolved.Input)
		if qerr != nil {
			return failure(files, qerr)
		}
		next, snap, perr := publish(files, nil, q)
		if perr != nil {
			return failure(files, perr)
		}
		return Result{Status: Unresolved, Message: "no answer for " + q.Call, Files: next, Snapshot: snap, Why: trace}
	}
	if err != nil {
		r := failure(files, err)
		r.Why = trace
		return r
	}
	if _, err := samples.Canonical(output); err != nil {
		return failure(files, err)
	}
	var proposals []project.Question
	for _, pr := range e.Proposals() {
		q, err := project.NewProposal(p, pr.Function, pr.Input, pr.Output)
		if err != nil {
			return failure(files, err)
		}
		proposals = append(proposals, q)
	}
	next, snap, err := publish(files, nil, proposals...)
	if err != nil {
		return failure(files, err)
	}
	r := Result{Status: OK, Output: samples.Literal(output), Files: next, Snapshot: snap, Why: trace}
	if len(proposals) > 0 {
		r.Status = Unconfirmed
		r.Message = "this answer comes from code with fewer than two examples; confirm the proposed answers that are right"
	}
	return r
}

// Test checks implementations against their examples, like `park test`.
// Only may name one Process.function; empty checks everything.
func Test(ctx context.Context, files Files, only string) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	report, err := engine.Check(ctx, p, only)
	if err != nil {
		return failure(files, err)
	}
	next, snap, err := publish(files, nil, report.Questions...)
	if err != nil {
		return failure(files, err)
	}
	r := Result{Status: OK, Message: report.Summary(), Files: next, Snapshot: snap}
	for _, line := range report.Lines {
		r.Lines = append(r.Lines, Line{Failed: line.Failed, Text: line.Text})
	}
	if report.Failures > 0 {
		r.Status = Failed
	}
	return r
}

// AddExample adds `Process.fn(args) == value` to the function's own module,
// like `park example add`. It also confirms a proposal: pass its whole line.
func AddExample(files Files, text string) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	parsed, want, err := source.ParseExample(text)
	if err != nil {
		return failure(files, err)
	}
	qualified, input, err := bind(p, parsed)
	if err != nil {
		return failure(files, err)
	}
	path, b, err := project.AddExample(p, qualified, input, want)
	if err != nil {
		return failure(files, err)
	}
	return edit(files, path, b, "Added example in "+path)
}

// RemoveExample deletes the example for a call, like `park example remove`.
func RemoveExample(files Files, call string) Result {
	p, err := load(files)
	if err != nil {
		return failure(files, err)
	}
	parsed, err := source.ParseCall(call)
	if err != nil {
		return failure(files, err)
	}
	qualified, input, err := bind(p, parsed)
	if err != nil {
		return failure(files, err)
	}
	path, b, err := project.RemoveExample(p, qualified, input)
	if err != nil {
		return failure(files, err)
	}
	return edit(files, path, b, "Removed example from "+path)
}

func edit(files Files, path string, b []byte, message string) Result {
	next, snap, err := publish(files, Files{path: string(b)})
	if err != nil {
		return failure(files, err)
	}
	return Result{Status: OK, Message: message, Files: next, Snapshot: snap}
}

// ArchiveDir is the folder a downloaded project unpacks into.
const ArchiveDir = "park-project"

// Archive packs the project as a zip that the CLI can open: the modules, the
// question queue, and a freshly rendered park.html. An invalid project is
// still packed, without a catalog, so work is never trapped in the browser.
func Archive(files Files) Result {
	r := Load(files)
	out := clone(r.Files)
	if r.Snapshot.Valid {
		out["park.html"] = r.Snapshot.Catalog
	}
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range project.Names(out) {
		if !strings.HasSuffix(name, ".star") && name != project.QuestionsFile && name != "park.html" {
			continue
		}
		f, err := w.CreateHeader(&zip.FileHeader{Name: ArchiveDir + "/" + name, Method: zip.Deflate})
		if err != nil {
			return failure(files, err)
		}
		if _, err := f.Write([]byte(out[name])); err != nil {
			return failure(files, err)
		}
	}
	if err := w.Close(); err != nil {
		return failure(files, err)
	}
	r.Archive = buf.Bytes()
	return r
}
