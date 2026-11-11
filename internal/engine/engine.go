// Package engine runs pure Starlark functions through example-aware dispatch.
package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"github.com/shanecandoit/ProcessStarlark/internal/source"
	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

const MaxSteps = 1_000_000
const MaxCalls = 128

// Unresolved is a call with no example and no implementation to answer it.
type Unresolved struct {
	Function string
	Inputs   []string       // Input names in signature order, for display.
	Input    map[string]any // Named inputs.
}

func (u *Unresolved) Error() string {
	return "unresolved call " + FormatCall(u.Function, u.Inputs, u.Input)
}

// Proposal is an unconfirmed answer: computed by an implementation that does
// not yet have two examples, or given by a Resolver. It is shown to the user to
// confirm as an example, never treated as one, and never cached.
type Proposal struct {
	Function string
	Input    map[string]any
	Output   any
	By       string // The resolver's name; empty for unconfirmed code.
}

// FormatCall renders a call as Process.fn(name=value, ...) in signature order.
func FormatCall(qualified string, names []string, input map[string]any) string {
	return project.FormatCall(qualified, names, input)
}

// Cache stores results computed by confirmed code, keyed by function, function
// hash (content-addressed), and canonical input. Implementations must be safe to discard.
type Cache interface {
	Get(function, functionHash string, input any) (any, bool, error)
	Put(function, functionHash string, input, output any) error
}

// DirCache is the CLI's disk cache: one JSON record per result in .cache/.
type DirCache struct{ Root string }

func (c DirCache) dir() (string, error) { return project.SafePath(c.Root, ".cache") }

func (c DirCache) Get(function, functionHash string, input any) (any, bool, error) {
	dir, err := c.dir()
	if err != nil {
		return nil, false, err
	}
	return samples.ReadCache(dir, function, functionHash, input)
}

func (c DirCache) Put(function, functionHash string, input, output any) error {
	dir, err := c.dir()
	if err != nil {
		return err
	}
	return samples.WriteCache(dir, function, functionHash, input, output)
}

type Engine struct {
	Project        *project.Project
	Cache          Cache             // Nil disables caching.
	FunctionHashes map[string]string // qualified name -> content hash
	// Resolvers answer, in order, calls with no example and no code. Their
	// answers are proposals. Empty means such calls are unresolved.
	Resolvers  []Resolver
	bodies     map[string]*starlark.Function
	deps       map[string]Deps
	loaded     map[string]bool
	unresolved *Unresolved
	proposals  []Proposal
	active     map[string]bool
	depth      int
	Testing    bool
	Bypass     string
	// Why records a provenance tree for each Run (see Trace). It also skips
	// cache reads, so every answer's tree is complete.
	Why   bool
	trace *Step
	stack []*Step
}

func New(p *project.Project) (*Engine, error) {
	e := &Engine{Project: p, bodies: map[string]*starlark.Function{}, deps: map[string]Deps{}, loaded: map[string]bool{}, active: map[string]bool{}, FunctionHashes: map[string]string{}}
	// Projects loaded from disk cache on disk; in-memory ones (root "") don't.
	if p.Root != "" {
		e.Cache = DirCache{Root: p.Root}
	}
	// Compute function hashes for content-addressed caching
	versions, err := ComputeAllFunctionHashes(p)
	if err != nil {
		return nil, err
	}
	for qualified, v := range versions {
		e.FunctionHashes[qualified] = v.Hash
	}
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
	// Resolution distinguishes process namespaces from locals with the same
	// spelling. Validate direct members even in bodies that never execute.
	syntax.Walk(file, func(node syntax.Node) bool {
		if walkErr != nil {
			return false
		}
		dot, ok := node.(*syntax.DotExpr)
		if !ok {
			return true
		}
		id, ok := dot.X.(*syntax.Ident)
		if !ok {
			return true
		}
		binding, ok := id.Binding.(*resolve.Binding)
		if !ok || binding.Scope != resolve.Predeclared {
			return true
		}
		if target := e.Project.Processes[id.Name]; target != nil && target.Functions[dot.Name.Name] == nil {
			walkErr = fmt.Errorf("%s: unknown function %s.%s; declare a public function in %s", dot.Name.NamePos, id.Name, dot.Name.Name, target.Module)
			return false
		}
		return true
	})
	if walkErr != nil {
		return walkErr
	}
	e.findDeps(proc, file, aliases)
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

// Proposals lists the unconfirmed answers computed during the last Run, in
// call order. A result that depends on any of them is itself unconfirmed.
func (e *Engine) Proposals() []Proposal { return e.proposals }

func (e *Engine) Run(ctx context.Context, qualified string, input any) (any, error) {
	e.unresolved = nil
	e.proposals = nil
	e.trace = nil
	e.stack = nil
	thread := &starlark.Thread{Name: qualified}
	thread.SetMaxExecutionSteps(MaxSteps)
	thread.SetLocal(contextKey, ctx)
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
	if !e.Why {
		return e.resolve(thread, qualified, raw, nil)
	}
	step := &Step{Function: qualified}
	if n := len(e.stack); n > 0 {
		parent := e.stack[n-1]
		parent.Steps = append(parent.Steps, step)
	} else {
		e.trace = step
	}
	e.stack = append(e.stack, step)
	out, err := e.resolve(thread, qualified, raw, step)
	e.stack = e.stack[:len(e.stack)-1]
	if err == nil {
		step.Output = out
		return out, nil
	}
	step.Failed = true
	// Starlark has no try/except, so a failing callee always fails its
	// caller. Only the innermost failure carries the error text.
	if k := len(step.Steps); k > 0 && step.Steps[k-1].Failed {
		return out, err
	}
	if step.Source != SourceUnknown {
		step.Error = err.Error()
	}
	return out, err
}

// resolve answers one call: an example, then the cache, then code. When step
// is non-nil it records the input and where the answer came from.
func (e *Engine) resolve(thread *starlark.Thread, qualified string, raw any, step *Step) (any, error) {
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
	if step != nil {
		step.Inputs, step.Input = f.Inputs, input
	}
	bypass := e.Testing && e.Bypass == qualified
	if !bypass {
		if out, ok := f.Index[key]; ok {
			if step != nil {
				step.Source = SourceExample
				for _, ex := range f.Examples {
					if samples.Key(ex.Input) == key {
						step.Path, step.Line = ex.Path, ex.Line
						break
					}
				}
			}
			return out, nil
		}
	}
	if !e.Testing && !e.Why && e.Cache != nil && f.Ready() {
		fnHash := e.FunctionHashes[qualified]
		if out, ok, err := e.Cache.Get(qualified, fnHash, input); err != nil {
			return nil, err
		} else if ok {
			return out, nil
		}
	}
	body := e.bodies[qualified]
	if body == nil {
		out, by, ok, err := e.ask(thread, Call{Function: qualified, Inputs: f.Inputs, Input: input})
		if err != nil {
			return nil, err
		}
		if ok {
			e.proposals = append(e.proposals, Proposal{Function: qualified, Input: input, Output: out, By: by})
			if step != nil {
				step.Source, step.By, step.Path, step.Line = SourceProposed, by, f.Path, f.Line
			}
			return out, nil
		}
		u := &Unresolved{Function: qualified, Inputs: f.Inputs, Input: input}
		e.unresolved = u
		if step != nil {
			step.Source, step.Path, step.Line = SourceUnknown, f.Path, f.Line
		}
		return nil, u
	}
	// Code without two examples still runs, but its answers are only proposals
	// until a person confirms them as examples.
	unconfirmed := !f.Ready()
	if step != nil {
		step.Source, step.Path, step.Line = SourceCode, f.Path, f.Line
		if unconfirmed {
			step.Source = SourceUnconfirmed
		}
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
	if unconfirmed {
		e.proposals = append(e.proposals, Proposal{Function: qualified, Input: input, Output: output})
	}
	// Cache only results that rest entirely on confirmed code.
	if !e.Testing && e.Cache != nil && len(e.proposals) == 0 && f.Ready() {
		fnHash := e.FunctionHashes[qualified]
		if err := e.Cache.Put(qualified, fnHash, input, output); err != nil {
			return nil, err
		}
	}
	return output, nil
}
