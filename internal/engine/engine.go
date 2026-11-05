// Package engine runs pure Starlark functions through example-aware dispatch.
package engine

import (
	"context"
	"fmt"
	"path/filepath"
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
	Input    any
}

func (u *Unresolved) Error() string {
	return fmt.Sprintf("unresolved call %s(%s)", u.Function, samples.Key(u.Input))
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
		if len(args) != 1 || len(kwargs) != 0 {
			return nil, fmt.Errorf("%s expects exactly one positional input", qualified)
		}
		input, err := fromStar(args[0], 0)
		if err != nil {
			return nil, fmt.Errorf("%s input: %w", qualified, err)
		}
		out, err := e.dispatch(t, qualified, input)
		if err != nil {
			return nil, err
		}
		return toStar(out), nil
	})
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
	if proc.Module == "" {
		e.loaded[name] = true
		return nil
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(proc.ManifestPath), proc.Module))
	parsed, err := source.Parse(path, e.Project.Files[path])
	if err != nil {
		return err
	}
	if parsed.Name != "" && parsed.Name != name {
		return fmt.Errorf("%s: Process %s does not match registered name %s", path, parsed.Name, name)
	}
	file := parsed.File
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
	if parsed.Name != "" {
		if _, exists := pre[name]; exists {
			return fmt.Errorf("%s: Process name %s collides with a dependency or reserved builtin", path, name)
		}
		members := starlark.StringDict{}
		for _, fn := range project.Names(proc.Functions) {
			members[fn] = e.wrapper(name + "." + fn)
		}
		pre[name] = starlarkstruct.FromStringDict(starlark.String(name), members)
	}
	aliases := map[string]string{}
	for _, fn := range project.Names(proc.Functions) {
		f := proc.Functions[fn]
		qualified := name + "." + fn
		for _, alias := range []string{fn, f.Implementation} {
			if alias == "" {
				continue
			}
			if previous, ok := aliases[alias]; ok && previous != qualified {
				return fmt.Errorf("%s: ambiguous implementation/export alias %s", name, alias)
			}
			if _, ok := pre[alias]; ok {
				return fmt.Errorf("%s: export %s collides with a dependency or reserved builtin", name, alias)
			}
			aliases[alias] = qualified
		}
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
			walkErr = fmt.Errorf("%s: load() is unavailable; declare uses in the manifest", path)
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
		f := proc.Functions[fn]
		if f.Implementation == "" {
			if definitions[fn] != nil {
				return fmt.Errorf("%s: %s has a body but no registered implementation", path, fn)
			}
			continue
		}
		def := definitions[f.Implementation]
		if def == nil {
			return fmt.Errorf("%s: missing implementation %s", path, f.Implementation)
		}
		if len(def.Params) != 1 {
			return fmt.Errorf("%s: %s must have one positional parameter", path, f.Implementation)
		}
		if _, ok := def.Params[0].(*syntax.Ident); !ok {
			return fmt.Errorf("%s: %s cannot use defaults or variadic parameters", path, f.Implementation)
		}
		def.Name.Name = "__park_body_" + fn
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
		if proc.Functions[fn].Implementation != "" {
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
func (e *Engine) dispatch(thread *starlark.Thread, qualified string, input any) (any, error) {
	_, f, err := e.Project.Function(qualified)
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
		u := &Unresolved{Function: qualified, Input: input}
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
	arg := toStar(input)
	arg.Freeze()
	result, err := starlark.Call(thread, body, starlark.Tuple{arg}, nil)
	if err != nil {
		return nil, err
	}
	output, err := fromStar(result, 0)
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

func toStar(v any) starlark.Value {
	switch x := v.(type) {
	case nil:
		return starlark.None
	case bool:
		return starlark.Bool(x)
	case string:
		return starlark.String(x)
	case int64:
		return starlark.MakeInt64(x)
	case []any:
		a := make([]starlark.Value, len(x))
		for i, v := range x {
			a[i] = toStar(v)
		}
		return starlark.NewList(a)
	case map[string]any:
		d := starlark.NewDict(len(x))
		for _, k := range project.Names(x) {
			_ = d.SetKey(starlark.String(k), toStar(x[k]))
		}
		return d
	default:
		panic(fmt.Sprintf("unsupported host value %T", v))
	}
}
func fromStar(v starlark.Value, depth int) (any, error) {
	if depth > samples.MaxDepth {
		return nil, fmt.Errorf("value nesting exceeds %d", samples.MaxDepth)
	}
	var out any
	switch x := v.(type) {
	case starlark.NoneType:
		out = nil
	case starlark.Bool:
		out = bool(x)
	case starlark.String:
		out = string(x)
	case starlark.Int:
		n, ok := x.Int64()
		if !ok {
			return nil, fmt.Errorf("integer outside signed 64-bit range")
		}
		out = n
	case *starlark.List:
		if x.Len() > samples.MaxValueBytes/2 {
			return nil, fmt.Errorf("list exceeds size limit")
		}
		a := make([]any, x.Len())
		for i := range a {
			v, err := fromStar(x.Index(i), depth+1)
			if err != nil {
				return nil, err
			}
			a[i] = v
		}
		out = a
	case *starlark.Dict:
		if x.Len() > samples.MaxValueBytes/4 {
			return nil, fmt.Errorf("object exceeds size limit")
		}
		m := map[string]any{}
		for _, item := range x.Items() {
			k, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("map keys must be strings")
			}
			v, err := fromStar(item[1], depth+1)
			if err != nil {
				return nil, err
			}
			m[string(k)] = v
		}
		out = m
	default:
		return nil, fmt.Errorf("unsupported Starlark value %s", v.Type())
	}
	if _, err := samples.Canonical(out); err != nil {
		return nil, err
	}
	return out, nil
}
