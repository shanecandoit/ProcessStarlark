package project

import (
	"fmt"
	"os"
	"strings"

	"park/internal/samples"
	"park/internal/source"
)

// QuestionsFile holds calls Park could not answer. Each one waits for the user
// to answer it: either an example for that call, or an implementation of the
// function. It is local state for now; a later version moves it to a database.
const QuestionsFile = "questions.txt"

const questionsHeader = `# Questions: calls Park could not answer on its own.
#
# A line ending in "== value" is an answer proposed by code that doesn't have two
# examples yet. If it's right, confirm it as an example:
#   park example add '<the whole line>'
# If it's wrong, add the correct example or fix the code.
#
# A line without "== value" has no answer at all. Answer it with an example,
#   park example add 'Process.function(...) == answer'
# or by implementing the function.
#
# Answered questions are removed automatically.
`

// Question is one call waiting for the user: either unanswered, or answered
// only by unconfirmed code (a proposal).
type Question struct {
	Function    string         // Qualified, e.g. Zones.zone.
	Inputs      []string       // Input names, for display.
	Input       map[string]any // Named inputs.
	Call        string         // As written, e.g. Zones.zone("DE").
	Proposed    any            // The code's answer, when HasProposal.
	HasProposal bool
}

// Line is the question as written in the queue: the call, plus "== value" for
// a proposal, so the whole line can be confirmed with park example add.
func (q Question) Line() string {
	if q.HasProposal {
		return q.Call + " == " + samples.Literal(q.Proposed)
	}
	return q.Call
}

func (q Question) key() string { return q.Function + "\x00" + samples.Key(q.Input) }

// NewQuestion records a call that no example or implementation could answer.
func NewQuestion(p *Project, qualified string, input map[string]any) (Question, error) {
	_, f, err := p.Function(qualified)
	if err != nil {
		return Question{}, err
	}
	return Question{Function: qualified, Inputs: f.Inputs, Input: input, Call: CallText(qualified, f, input)}, nil
}

// NewProposal records an answer computed by code that is not yet confirmed.
func NewProposal(p *Project, qualified string, input map[string]any, output any) (Question, error) {
	q, err := NewQuestion(p, qualified, input)
	q.Proposed, q.HasProposal = output, err == nil
	return q, err
}

// CallText writes a call with positional literals in signature order.
func CallText(qualified string, f *Function, input map[string]any) string {
	args := make([]string, len(f.Inputs))
	for i, name := range f.Inputs {
		args[i] = samples.Literal(input[name])
	}
	return qualified + "(" + strings.Join(args, ", ") + ")"
}

// Answered reports whether the call has an example or its body is ready to run.
// This does not evaluate the body or verify its result or dependencies.
func (p *Project) Answered(q Question) bool {
	_, f, err := p.Function(q.Function)
	if err != nil {
		return false
	}
	_, known := f.Index[samples.Key(q.Input)]
	return known || f.Ready()
}

// LoadQuestions reads the queue. Lines that no longer name a valid call (for
// example after a function was renamed) are returned as stale.
func LoadQuestions(p *Project) (questions []Question, stale []string, err error) {
	path, err := SafePath(p.Root, QuestionsFile)
	if err != nil {
		return nil, nil, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if len(b) > MaxFileBytes {
		return nil, nil, fmt.Errorf("%s exceeds file limit", QuestionsFile)
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		call, proposed, hasProposal, err := source.ParseQuestion(line)
		if err != nil || call.Process == "" {
			stale = append(stale, line)
			continue
		}
		qualified := call.Process + "." + call.Function
		_, f, err := p.Function(qualified)
		if err != nil {
			stale = append(stale, line)
			continue
		}
		input, err := f.BindArgs(qualified, call.Args, call.Names, call.Values)
		if err != nil {
			stale = append(stale, line)
			continue
		}
		questions = append(questions, Question{Function: qualified, Inputs: f.Inputs, Input: input, Call: CallText(qualified, f, input), Proposed: proposed, HasProposal: hasProposal})
	}
	return questions, stale, nil
}

// OpenQuestions merges new questions into the queue and drops questions the
// project can now answer. Each call appears once, at its first position; a
// newer entry for the same call replaces the older one, so a proposal reflects
// the latest code.
func OpenQuestions(p *Project, queued, added []Question) []Question {
	position := map[string]int{}
	var open []Question
	for _, q := range append(append([]Question{}, queued...), added...) {
		if p.Answered(q) {
			continue
		}
		if i, ok := position[q.key()]; ok {
			open[i] = q
			continue
		}
		position[q.key()] = len(open)
		open = append(open, q)
	}
	return open
}

// FormatQuestions renders the queue file, or nil when it is empty so the file
// is removed.
func FormatQuestions(questions []Question) []byte {
	if len(questions) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(questionsHeader)
	for _, q := range questions {
		b.WriteString(q.Line() + "\n")
	}
	return []byte(b.String())
}
