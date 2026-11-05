// Package source parses Park's stateless Process namespaces into Starlark.
package source

import (
	"fmt"
	"regexp"
	"strings"

	"go.starlark.net/syntax"
)

var header = regexp.MustCompile(`^Process[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*\{[ \t]*(#.*)?\r?$`)
var closing = regexp.MustCompile(`^\}[ \t]*(#.*)?\r?$`)

type Module struct {
	Name string // Empty for regular modules without a Process declaration.
	File *syntax.File
}

// Parse accepts one Process Name { ... } block, or a regular Starlark module.
// The block is parsed as a temporary function suite and then lifted into module
// scope. Starlark handles strings, comments, dictionaries, and indentation; source
// line numbers are retained. Only functions and a docstring may inhabit a Process.
func Parse(path string, contents []byte) (*Module, error) {
	lines := strings.Split(string(contents), "\n")
	first := significant(lines, 0)
	if first == len(lines) || strings.Fields(lines[first])[0] != "Process" {
		file, err := (&syntax.FileOptions{}).Parse(path, contents, 0)
		return &Module{File: file}, err
	}
	m := header.FindStringSubmatch(lines[first])
	if m == nil {
		return nil, fmt.Errorf("%s:%d: expected Process Name { at column 1", path, first+1)
	}
	last := len(lines) - 1
	for last > first && insignificant(lines[last]) {
		last--
	}
	if last == first || !closing.MatchString(lines[last]) {
		return nil, fmt.Errorf("%s:%d: Process %s must end with } at column 1", path, last+1, m[1])
	}
	lines[last] = ""
	body := significant(lines, first+1)
	indented := body < last && (strings.HasPrefix(lines[body], " ") || strings.HasPrefix(lines[body], "\t"))
	if indented {
		lines[first] = "def " + m[1] + "():"
	} else {
		// Braces also allow function declarations aligned with Process itself.
		lines[first] = ""
	}
	file, err := (&syntax.FileOptions{}).Parse(path, strings.Join(lines, "\n"), 0)
	if err != nil {
		return nil, err
	}
	if indented {
		if len(file.Stmts) != 1 {
			return nil, fmt.Errorf("%s: all functions must be inside Process %s", path, m[1])
		}
		file.Stmts = file.Stmts[0].(*syntax.DefStmt).Body
	}
	for i, stmt := range file.Stmts {
		if _, ok := stmt.(*syntax.DefStmt); ok {
			continue
		}
		if expr, ok := stmt.(*syntax.ExprStmt); ok && i == 0 {
			if lit, ok := expr.X.(*syntax.Literal); ok && lit.Token == syntax.STRING {
				continue
			}
		}
		pos, _ := stmt.Span()
		return nil, fmt.Errorf("%s: Process permits only function definitions and a docstring; shared state is unsupported", pos)
	}
	return &Module{Name: m[1], File: file}, nil
}

func insignificant(line string) bool {
	line = strings.TrimSpace(line)
	return line == "" || strings.HasPrefix(line, "#")
}

func significant(lines []string, start int) int {
	for start < len(lines) && insignificant(lines[start]) {
		start++
	}
	return start
}
