// Package engine runs pure Starlark functions through example-aware dispatch.
package engine

import (
	"context"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
	"park/internal/project"
	"park/internal/samples"
	"park/internal/source"
)

const MaxSteps = 1_000_000
const MaxCalls = 128

type Unresolved struct {
	Function string
	Inputs   []string       // Input names in signature order, for display.
	Input    map[string]any // Named inputs.
}

func (u *Unresolved) Error() string {
	return "unresolved call " + FormatCall(u.Function, u.Inputs, u.Input)
}

// FormatCall renders a call as Process.fn(name=value, ...) in signature order.
func FormatCall(qualified string, names []string, input map[string]any) string {
	return project.FormatCall(qualified, names, input)
}

type Engine struct {
	Project    *project.Project
	bodies     map[string]*starlark.Function
	loaded     map[string]bool
	unresolved *Unresolved
	active     map[string]bool
	depth      int
	Testing    bool
	Bypass     string
}

func New(p *project.Project) (*Engine, error) {
	e := &Engine{Project: p, bodies: map[string]*starlark.Function{}, loaded: map[string]bool{}, active: map[string]bool{}}
	for _, name := range project.Names(p.Processes) {
		if err := e.load(name); err != nil {
			return nil, err
		}
	}
	return e, nil
}
func (e *Engine) wrapper(qualified string) *starlark.Builtin {
	return starlark.NewBuiltin(qualified, func(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		_, f, err := e.Project.Function(qualified)
		if err != nil {
			return nil, err
		}
		input, err := bindArgs(qualified, f, args, kwargs)
		if err != nil {
			return nil, err
		}
		out, err := e.dispatch(t, qualified, input)
		if err != nil {
			return nil, err
		}
		return samples.ToStar(out), nil
	})
}

// bindArgs converts Starlark call arguments and binds them to named inputs.
func bindArgs(qualified string, f *project.Function, args starlark.Tuple, kwargs []starlark.Tuple) (map[string]any, error) {
	positional := make([]any, len(args))
	for i, arg := range args {
		v, err := samples.FromStar(arg, 0)
		if err != nil {
			return nil, fmt.Errorf("%s argument %d: %w", qualified, i+1, err)
		}
		positional[i] = v
	}
	names := make([]string, len(kwargs))
	values := make([]any, len(kwargs))
	for i, kw := range kwargs {
		key, _ := kw[0].(starlark.String)
		names[i] = string(key)
		v, err := samples.FromStar(kw[1], 0)
		if err != nil {
			return nil, fmt.Errorf("%s input %s: %w", qualified, names[i], err)
		}
		values[i] = v
	}
	return f.BindArgs(qualified, positional, names, values)
}

func (e *Engine) load(name string) error {
	if e.loaded[name] {
		return nil
	}
	proc := e.Project.Processes[name]
	for _, dep := range proc.Uses {
		if err := e.load(dep); err != nil {
			return err
		}
	}
	path := proc.Module
	parsed, err := source.Parse(path, e.Project.Files[path])
	if err != nil {
		return err
	}
	file := parsed.File
	// Example-only declarations (`def f(x): pass`) have no body to run; calls
	// to them dispatch to examples.
	kept := file.Stmts[:0]
	for _, stmt := range file.Stmts {
		if def, ok := stmt.(*syntax.DefStmt); ok {
			if f := proc.Functions[def.Name.Name]; f != nil && !f.Implemented {
				continue
			}
		}
		kept = append(kept, stmt)
	}
	file.Stmts = kept
	pre := starlark.StringDict{}
	disabled := func(_ *starlark.Thread, b *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return nil, fmt.Errorf("%s is unavailable in Park", b.Name())
	}
	for _, n := range []string{"print", "float", "bytes", "set"} {
		pre[n] = starlark.NewBuiltin(n, disabled)
	}
	for _, dep := range proc.Uses {
		members := starlark.StringDict{}
		for _, fn := range project.Names(e.Project.Processes[dep].Functions) {
			members[fn] = e.wrapper(dep + "." + fn)
		}
		pre[dep] = starlarkstruct.FromStringDict(starlark.String(dep), members)
	}
	// The process's own namespace, so `RomanAdd.eval(...)` works inside RomanAdd.
	if _, exists := pre[name]; exists {
		return fmt.Errorf("%s: Process name %s collides with a dependency or reserved builtin", path, name)
	}
	members := starlark.StringDict{}
	for _, fn := range project.Names(proc.Functions) {
		members[fn] = e.wrapper(name + "." + fn)
	}
	pre[name] = starlarkstruct.FromStringDict(starlark.String(name), members)
	aliases := map[string]string{}
	for _, fn := range project.Names(proc.Functions) {
		if _, ok := pre[fn]; ok {
			return fmt.Errorf("%s: function %s collides with a process name or reserved builtin", path, fn)
		}
		aliases[fn] = name + "." + fn
	}
	for alias, qualified := range aliases {
		pre[alias] = e.wrapper(qualified)
	}
	var walkErr error
	syntax.Walk(file, func(node syntax.Node) bool {
		if walkErr != nil {
			return false
		}
		if id, ok := node.(*syntax.Ident); ok && strings.HasPrefix(id.Name, "__park_") {
			walkErr = fmt.Errorf("%s: reserved identifier prefix __park_", path)
			return false
		}
		if _, ok := node.(*syntax.LoadStmt); ok {
			walkErr = fmt.Errorf("%s: load() is unavailable; call other processes as Process.function(...)", path)
			return false
		}
		// Defaults execute at initialization, including defaults in private helpers.
		if def, ok := node.(*syntax.DefStmt); ok {
			for _, param := range def.Params {
				if b, ok := param.(*syntax.BinaryExpr); ok && b.Op == syntax.EQ && !literal(b.Y) {
					walkErr = fmt.Errorf("%s: defaults must be literal constants", path)
					return false
				}
			}
		}
		return true
	})
	if walkErr != nil {
		return walkErr
	}
	definitions := map[string]*syntax.DefStmt{}
	for i, stmt := range file.Stmts {
		switch s := stmt.(type) {
		case *syntax.ExprStmt:
			lit, ok := s.X.(*syntax.Literal)
			if i != 0 || !ok || lit.Token != syntax.STRING {
				return fmt.Errorf("%s: top-level expressions are unavailable", path)
			}
		case *syntax.DefStmt:
			if definitions[s.Name.Name] != nil {
				return fmt.Errorf("%s: duplicate definition %s", path, s.Name.Name)
			}
			definitions[s.Name.Name] = s
			if _, exists := pre[s.Name.Name]; exists && aliases[s.Name.Name] == "" {
				return fmt.Errorf("%s: definition shadows reserved namespace %s", path, s.Name.Name)
			}
		case *syntax.AssignStmt:
			id, ok := s.LHS.(*syntax.Ident)
			if !ok || s.Op != syntax.EQ || !literal(s.RHS) {
				return fmt.Errorf("%s: top-level assignments must bind a name to a literal constant", path)
			}
			if _, ok := pre[id.Name]; ok {
				return fmt.Errorf("%s: constant shadows export/namespace %s", path, id.Name)
			}
		default:
			return fmt.Errorf("%s: top level permits only function definitions and literal constants", path)
		}
	}
	for _, fn := range project.Names(proc.Functions) {
		if proc.Functions[fn].Implemented {
			definitions[fn].Name.Name = "__park_body_" + fn
		}
	}
	// Unregistered same-process implementation names must not shadow dispatch.
	for alias, qualified := range aliases {
		if def := definitions[alias]; def != nil && !strings.HasPrefix(def.Name.Name, "__park_body_") {
			return fmt.Errorf("%s: definition %s shadows dispatch for %s", path, alias, qualified)
		}
	}
	program, err := starlark.FileProgram(file, pre.Has)
	if err != nil {
		return err
	}
	thread := &starlark.Thread{Name: "initialize " + name}
	thread.SetMaxExecutionSteps(MaxSteps)
	globals, err := program.Init(thread, pre)
	if err != nil {
		return err
	}
	globals.Freeze()
	pre.Freeze()
	for _, fn := range project.Names(proc.Functions) {
		if proc.Functions[fn].Implemented {
			body, ok := globals["__park_body_"+fn].(*starlark.Function)
			if !ok {
				return fmt.Errorf("%s.%s: implementation is not a function", name, fn)
			}
			e.bodies[name+"."+fn] = body
		}
	}
	e.loaded[name] = true
	return nil
}

func literal(expr syntax.Expr) bool {
	switch x := expr.(type) {
	case *syntax.Literal:
		return x.Token == syntax.INT || x.Token == syntax.STRING
	case *syntax.Ident:
		return x.Name == "None" || x.Name == "True" || x.Name == "False"
	case *syntax.ParenExpr:
		return literal(x.X)
	case *syntax.UnaryExpr:
		return (x.Op == syntax.MINUS || x.Op == syntax.PLUS) && literal(x.X)
	case *syntax.ListExpr:
		for _, v := range x.List {
			if !literal(v) {
				return false
			}
		}
		return true
	case *syntax.TupleExpr:
		for _, v := range x.List {
			if !literal(v) {
				return false
			}
		}
		return true
	case *syntax.DictExpr:
		for _, v := range x.List {
			entry, ok := v.(*syntax.DictEntry)
			if !ok || !literal(entry.Key) || !literal(entry.Value) {
				return false
			}
		}
		return true
	}
	return false
}

func (e *Engine) Run(ctx context.Context, qualified string, input any) (any, error) {
	e.unresolved = nil
	thread := &starlark.Thread{Name: qualified}
	thread.SetMaxExecutionSteps(MaxSteps)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			thread.Cancel(ctx.Err().Error())
		case <-done:
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output, err := e.dispatch(thread, qualified, input)
	if err != nil && e.unresolved != nil {
		return nil, e.unresolved
	}
	return output, err
}
func (e *Engine) dispatch(thread *starlark.Thread, qualified string, raw any) (any, error) {
	_, f, err := e.Project.Function(qualified)
	if err != nil {
		return nil, err
	}
	input, err := f.BindInput(raw)
	if err != nil {
		return nil, err
	}
	canonical, err := samples.Canonical(input)
	if err != nil {
		return nil, err
	}
	key := string(canonical)
	bypass := e.Testing && e.Bypass == qualified
	if !bypass {
		if out, ok := f.Index[key]; ok {
			return out, nil
		}
	}
	cacheDir, err := project.SafePath(e.Project.Root, ".cache")
	if err != nil {
		return nil, err
	}
	if !e.Testing {
		if out, ok, err := samples.ReadCache(cacheDir, qualified, e.Project.Revision, input); err != nil {
			return nil, err
		} else if ok {
			return out, nil
		}
	}
	body := e.bodies[qualified]
	if body == nil {
		u := &Unresolved{Function: qualified, Inputs: f.Inputs, Input: input}
		e.unresolved = u
		return nil, u
	}
	if len(f.Examples) < 2 {
		return nil, fmt.Errorf("%s needs examples: body requires two distinct authored inputs", qualified)
	}
	callKey := qualified + "\x00" + key
	if e.active[callKey] {
		return nil, fmt.Errorf("recursive call repeats %s(%s)", qualified, key)
	}
	if e.depth >= MaxCalls {
		return nil, fmt.Errorf("call depth exceeds %d", MaxCalls)
	}
	e.active[callKey] = true
	e.depth++
	defer func() { delete(e.active, callKey); e.depth-- }()
	args := make(starlark.Tuple, len(f.Inputs))
	for i, name := range f.Inputs {
		args[i] = samples.ToStar(input[name])
		args[i].Freeze()
	}
	result, err := starlark.Call(thread, body, args, nil)
	if err != nil {
		return nil, err
	}
	output, err := samples.FromStar(result, 0)
	if err != nil {
		return nil, fmt.Errorf("%s output: %w", qualified, err)
	}
	if !e.Testing {
		if err := samples.WriteCache(cacheDir, qualified, e.Project.Revision, input, output); err != nil {
			return nil, err
		}
	}
	return output, nil
}
