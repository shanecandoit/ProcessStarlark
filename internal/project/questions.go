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

const questionsHeader = `# Questions: calls Park could not answer.
# Answer each one with an example, e.g.
#   park example add 'Process.function(...) == answer'
# or by implementing the function. Answered questions are removed automatically.
`

// Question is one unanswered call.
type Question struct {
	Function string         // Qualified, e.g. Zones.zone.
	Inputs   []string       // Input names, for display.
	Input    map[string]any // Named inputs.
	Call     string         // As written, e.g. Zones.zone("DE").
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

// CallText writes a call with positional literals in signature order.
func CallText(qualified string, f *Function, input map[string]any) string {
	args := make([]string, len(f.Inputs))
	for i, name := range f.Inputs {
		args[i] = samples.Literal(input[name])
	}
	return qualified + "(" + strings.Join(args, ", ") + ")"
}

// Answered reports whether the project can now answer the question: it has an
// example for the call, or the function has an implementation.
func (p *Project) Answered(q Question) bool {
	_, f, err := p.Function(q.Function)
	if err != nil {
		return false
	}
	_, known := f.Index[samples.Key(q.Input)]
	return known || f.Implemented
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
		call, err := source.ParseCall(line)
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
		questions = append(questions, Question{Function: qualified, Inputs: f.Inputs, Input: input, Call: CallText(qualified, f, input)})
	}
	return questions, stale, nil
}

// OpenQuestions merges new questions into the queue, in arrival order without
// duplicates, and drops questions the project can now answer.
func OpenQuestions(p *Project, queued, added []Question) []Question {
	seen := map[string]bool{}
	var open []Question
	for _, q := range append(append([]Question{}, queued...), added...) {
		if seen[q.key()] || p.Answered(q) {
			continue
		}
		seen[q.key()] = true
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
		b.WriteString(q.Call + "\n")
	}
	return []byte(b.String())
}
