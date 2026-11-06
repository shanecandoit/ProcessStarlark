// Package source parses Park modules: an optional indented `Process Name:`
// block, Starlark function definitions, and `example:` lines.
package source

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"go.starlark.net/syntax"
)

var header = regexp.MustCompile(`^Process[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*:[ \t]*(#.*)?\r?$`)
var exampleLine = regexp.MustCompile(`^([ \t]*)example:`)

// exampleMarker replaces `example:` so that each example line parses as an
// ordinary Starlark assignment; the parser then handles strings, brackets, and
// line continuations.
const exampleMarker = "__park_example__ ="

// Call is a function call written with literal arguments, such as
// RomanParse.parse("ix") or add(left=9, right=6).
type Call struct {
	Process  string // Empty for a bare function name.
	Function string
	Args     []any
	Names    []string // Keyword argument names, parallel to Values.
	Values   []any
}

// Example is one `example: call == value` line.
type Example struct {
	Call          *Call
	Want          any
	Line, EndLine int // 1-based, inclusive.
	Indent        string
}

type Module struct {
	Name     string // Empty for regular modules without a Process declaration.
	File     *syntax.File
	Examples []Example
	Indent   string // Indentation of the Process body ("" for regular modules).
	Header   int    // 1-based line of `Process Name:`, or 0.
}

// Parse reads a module. The Process body is parsed as a temporary function
// suite and lifted into module scope, so line numbers match the source.
// Example lines are removed from File and returned separately.
func Parse(path string, contents []byte) (*Module, error) {
	lines := strings.Split(string(contents), "\n")
	marked := map[int]bool{}
	for i, line := range lines {
		if m := exampleLine.FindStringSubmatch(line); m != nil {
			lines[i] = m[1] + exampleMarker + line[len(m[0]):]
			marked[i+1] = true
		}
	}
	module := &Module{}
	first := significant(lines, 0)
	if first < len(lines) && strings.HasPrefix(lines[first], "Process") && len(strings.Fields(lines[first])) > 0 && strings.Fields(lines[first])[0] == "Process" {
		m := header.FindStringSubmatch(lines[first])
		if m == nil {
			return nil, fmt.Errorf("%s:%d: expected `Process Name:` at column 1", path, first+1)
		}
		module.Name = m[1]
		module.Header = first + 1
		body := significant(lines, first+1)
		if body == len(lines) {
			return nil, fmt.Errorf("%s:%d: Process %s has an empty body", path, first+1, m[1])
		}
		module.Indent = leading(lines[body])
		if module.Indent == "" {
			return nil, fmt.Errorf("%s:%d: the body of Process %s must be indented", path, body+1, m[1])
		}
		lines[first] = "def " + m[1] + "():"
	}
	file, err := (&syntax.FileOptions{}).Parse(path, strings.Join(lines, "\n"), 0)
	if err != nil {
		return nil, err
	}
	if module.Name != "" {
		if len(file.Stmts) != 1 {
			pos, _ := file.Stmts[1].Span()
			return nil, fmt.Errorf("%s: everything after `Process %s:` must be indented inside it", pos, module.Name)
		}
		file.Stmts = file.Stmts[0].(*syntax.DefStmt).Body
	}
	kept := file.Stmts[:0]
	for i, stmt := range file.Stmts {
		assign, ok := stmt.(*syntax.AssignStmt)
		if id, isIdent := lhs(assign); ok && isIdent && id == strings.TrimSuffix(exampleMarker, " =") {
			ex, err := parseExample(assign)
			if err != nil {
				return nil, err
			}
			start, end := assign.Span()
			ex.Line, ex.EndLine = int(start.Line), int(end.Line)
			ex.Indent = leading(lines[start.Line-1])
			delete(marked, ex.Line)
			module.Examples = append(module.Examples, *ex)
			continue
		}
		if module.Name != "" {
			if _, ok := stmt.(*syntax.DefStmt); !ok && !(i == 0 && docstring(stmt)) {
				pos, _ := stmt.Span()
				return nil, fmt.Errorf("%s: Process permits only a docstring, function definitions, and example: lines; shared state is unsupported", pos)
			}
		}
		kept = append(kept, stmt)
	}
	file.Stmts = kept
	module.File = file
	for line := range marked {
		return nil, fmt.Errorf("%s:%d: example: lines belong directly in the Process (or module) body, not inside a function or string", path, line)
	}
	return module, nil
}

func lhs(a *syntax.AssignStmt) (string, bool) {
	if a == nil || a.Op != syntax.EQ {
		return "", false
	}
	id, ok := a.LHS.(*syntax.Ident)
	if !ok {
		return "", false
	}
	return id.Name, true
}

func parseExample(a *syntax.AssignStmt) (*Example, error) {
	pos, _ := a.Span()
	cmp, ok := a.RHS.(*syntax.BinaryExpr)
	if !ok || cmp.Op != syntax.EQL {
		return nil, fmt.Errorf("%s: expected `example: function(arguments) == value`", pos)
	}
	call, err := CallOf(cmp.X)
	if err != nil {
		return nil, err
	}
	want, err := Value(cmp.Y)
	if err != nil {
		return nil, err
	}
	return &Example{Call: call, Want: want}, nil
}

// ParseCall parses a call written as one expression, e.g. `Math.add(9, right=6)`.
func ParseCall(text string) (*Call, error) {
	expr, err := (&syntax.FileOptions{}).ParseExpr("call", text, 0)
	if err != nil {
		return nil, err
	}
	return CallOf(expr)
}

// ParseValue parses one literal value, e.g. `["iv", "+", "i"]`.
func ParseValue(text string) (any, error) {
	expr, err := (&syntax.FileOptions{}).ParseExpr("value", text, 0)
	if err != nil {
		return nil, err
	}
	return Value(expr)
}

// ParseQuestion parses a queued question: a call, optionally followed by
// `== value` when the code proposed an answer.
func ParseQuestion(text string) (call *Call, proposed any, hasProposal bool, err error) {
	expr, err := (&syntax.FileOptions{}).ParseExpr("question", text, 0)
	if err != nil {
		return nil, nil, false, err
	}
	if cmp, ok := expr.(*syntax.BinaryExpr); ok && cmp.Op == syntax.EQL {
		if call, err = CallOf(cmp.X); err != nil {
			return nil, nil, false, err
		}
		proposed, err = Value(cmp.Y)
		return call, proposed, true, err
	}
	call, err = CallOf(expr)
	return call, nil, false, err
}

// ParseExample parses `call == value`, the form used by `example:` lines.
func ParseExample(text string) (*Call, any, error) {
	expr, err := (&syntax.FileOptions{}).ParseExpr("example", text, 0)
	if err != nil {
		return nil, nil, err
	}
	cmp, ok := expr.(*syntax.BinaryExpr)
	if !ok || cmp.Op != syntax.EQL {
		return nil, nil, fmt.Errorf("expected `Process.function(arguments) == value`")
	}
	call, err := CallOf(cmp.X)
	if err != nil {
		return nil, nil, err
	}
	want, err := Value(cmp.Y)
	return call, want, err
}

// CallOf reads a call expression whose arguments are all literals.
func CallOf(expr syntax.Expr) (*Call, error) {
	pos, _ := expr.Span()
	c, ok := expr.(*syntax.CallExpr)
	if !ok {
		return nil, fmt.Errorf("%s: expected a function call such as parse(\"ix\")", pos)
	}
	call := &Call{}
	switch fn := c.Fn.(type) {
	case *syntax.Ident:
		call.Function = fn.Name
	case *syntax.DotExpr:
		process, ok := fn.X.(*syntax.Ident)
		if !ok {
			return nil, fmt.Errorf("%s: expected Process.function", pos)
		}
		call.Process, call.Function = process.Name, fn.Name.Name
	default:
		return nil, fmt.Errorf("%s: expected function or Process.function", pos)
	}
	for _, arg := range c.Args {
		if kw, ok := arg.(*syntax.BinaryExpr); ok && kw.Op == syntax.EQ {
			name, ok := kw.X.(*syntax.Ident)
			if !ok {
				return nil, fmt.Errorf("%s: invalid argument name", pos)
			}
			v, err := Value(kw.Y)
			if err != nil {
				return nil, err
			}
			call.Names = append(call.Names, name.Name)
			call.Values = append(call.Values, v)
			continue
		}
		if len(call.Names) > 0 {
			return nil, fmt.Errorf("%s: positional argument after named argument", pos)
		}
		if _, ok := arg.(*syntax.UnaryExpr); ok && (arg.(*syntax.UnaryExpr).Op == syntax.STAR || arg.(*syntax.UnaryExpr).Op == syntax.STARSTAR) {
			return nil, fmt.Errorf("%s: *args and **kwargs are unsupported in calls", pos)
		}
		v, err := Value(arg)
		if err != nil {
			return nil, err
		}
		call.Args = append(call.Args, v)
	}
	return call, nil
}

// Value evaluates a literal: None, True, False, integers, strings, and lists,
// tuples, and dicts of literals. Tuples become lists.
func Value(expr syntax.Expr) (any, error) {
	v, err := value(expr, 0)
	if err != nil {
		return nil, err
	}
	if _, err := samples.Canonical(v); err != nil {
		pos, _ := expr.Span()
		return nil, fmt.Errorf("%s: %v", pos, err)
	}
	return v, nil
}

func value(expr syntax.Expr, depth int) (any, error) {
	pos, _ := expr.Span()
	if depth > samples.MaxDepth {
		return nil, fmt.Errorf("%s: value nesting exceeds %d", pos, samples.MaxDepth)
	}
	switch x := expr.(type) {
	case *syntax.Literal:
		switch x.Token {
		case syntax.STRING:
			return x.Value.(string), nil
		case syntax.INT:
			return integer(x, false)
		}
	case *syntax.Ident:
		switch x.Name {
		case "None":
			return nil, nil
		case "True":
			return true, nil
		case "False":
			return false, nil
		}
		return nil, fmt.Errorf("%s: %s is not a literal value; quote strings, e.g. \"%s\"", pos, x.Name, x.Name)
	case *syntax.ParenExpr:
		return value(x.X, depth)
	case *syntax.UnaryExpr:
		if lit, ok := x.X.(*syntax.Literal); ok && lit.Token == syntax.INT && (x.Op == syntax.MINUS || x.Op == syntax.PLUS) {
			return integer(lit, x.Op == syntax.MINUS)
		}
	case *syntax.ListExpr:
		return list(x.List, depth)
	case *syntax.TupleExpr:
		return list(x.List, depth)
	case *syntax.DictExpr:
		m := map[string]any{}
		for _, item := range x.List {
			entry := item.(*syntax.DictEntry)
			k, ok := entry.Key.(*syntax.Literal)
			if !ok || k.Token != syntax.STRING {
				return nil, fmt.Errorf("%s: dict keys must be strings", pos)
			}
			key := k.Value.(string)
			if _, dup := m[key]; dup {
				return nil, fmt.Errorf("%s: duplicate key %q", pos, key)
			}
			v, err := value(entry.Value, depth+1)
			if err != nil {
				return nil, err
			}
			m[key] = v
		}
		return m, nil
	}
	return nil, fmt.Errorf("%s: expected a literal value (None, True, False, integer, string, list, or dict)", pos)
}

func list(items []syntax.Expr, depth int) (any, error) {
	a := []any{}
	for _, item := range items {
		v, err := value(item, depth+1)
		if err != nil {
			return nil, err
		}
		a = append(a, v)
	}
	return a, nil
}

func integer(lit *syntax.Literal, negate bool) (any, error) {
	n := new(big.Int)
	switch v := lit.Value.(type) {
	case int64:
		n.SetInt64(v)
	case *big.Int:
		n.Set(v)
	default:
		pos, _ := lit.Span()
		return nil, fmt.Errorf("%s: unsupported integer literal", pos)
	}
	if negate {
		n.Neg(n)
	}
	if !n.IsInt64() {
		pos, _ := lit.Span()
		return nil, fmt.Errorf("%s: integer outside signed 64-bit range", pos)
	}
	return n.Int64(), nil
}

func docstring(stmt syntax.Stmt) bool {
	expr, ok := stmt.(*syntax.ExprStmt)
	if !ok {
		return false
	}
	lit, ok := expr.X.(*syntax.Literal)
	return ok && lit.Token == syntax.STRING
}

func leading(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
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
