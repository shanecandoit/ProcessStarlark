package workspace

import (
	"fmt"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/source"
)

// Format validates and formats a whole working project atomically.
func Format(files Files) Result {
	next := clone(files)
	for _, name := range project.Names(modules(files)) {
		text, err := source.Format(name, []byte(files[name]))
		if err != nil {
			return failure(files, err)
		}
		next[name] = string(text)
	}
	r := Load(next)
	if r.Status == Error {
		return failure(files, fmt.Errorf("%s", r.Message))
	}
	r.Message = "Formatted source; literals and indentation preserved."
	return r
}

func Lint(files Files) Result {
	r := Load(files)
	if r.Status == Error {
		return r
	}
	for _, name := range project.Names(modules(files)) {
		diagnostics, err := source.Lint(name, []byte(files[name]))
		if err != nil {
			return failure(files, err)
		}
		for _, d := range diagnostics {
			r.Lines = append(r.Lines, Line{Text: fmt.Sprintf("%s:%d: %s", d.Path, d.Line, d.Message)})
		}
	}
	r.Message = fmt.Sprintf("%d source style suggestions. Syntax and project validation passed.", len(r.Lines))
	return r
}
