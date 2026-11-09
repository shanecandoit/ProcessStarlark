package engine

import (
	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
)

// Deps are what an implemented function's code can depend on, found from its
// source. They are the edges per-function versions are built from: a change
// to anything listed here can change the function's answers.
//
// They are conservative. Any mention of a public function counts as a call,
// whether or not it is called on every path, and a process namespace used as
// a value (say, passed to getattr) counts as a call to each of its functions.
type Deps struct {
	// Calls are public functions the code may call, qualified and sorted. A
	// recursive function lists itself. Each call dispatches through examples,
	// so the callee's examples matter as much as its code.
	Calls []string
	// Helpers are private module-level functions the code runs, directly or
	// through other helpers. Their code is part of this function's code.
	Helpers []string
	// Constants are module-level constants the code reads, directly or
	// through helpers.
	Constants []string
}

// Deps returns the edges for an implemented function; ok is false for an
// example-only or unknown function.
func (e *Engine) Deps(qualified string) (Deps, bool) {
	d, ok := e.deps[qualified]
	return d, ok
}

// Reach lists every public function whose examples or code can change the
// answers of qualified: itself, then everything it calls, transitively, in
// sorted order after itself.
func (e *Engine) Reach(qualified string) []string {
	seen := map[string]bool{qualified: true}
	queue := []string{qualified}
	for len(queue) > 0 {
		d := e.deps[queue[0]]
		queue = queue[1:]
		for _, c := range d.Calls {
			if !seen[c] {
				seen[c] = true
				queue = append(queue, c)
			}
		}
	}
	delete(seen, qualified)
	return append([]string{qualified}, project.Names(seen)...)
}

// findDeps records Deps for each implemented function of a process. It runs
// after resolution, so each identifier's binding says what it names:
// a module-level helper or constant (Global), or a dispatch alias or process
// namespace (Predeclared). Definitions have their final names, with public
// bodies renamed to __park_body_<fn>.
func (e *Engine) findDeps(proc *project.Process, file *syntax.File, aliases map[string]string) {
	defs := map[string]*syntax.DefStmt{}
	constants := map[string]bool{}
	for _, stmt := range file.Stmts {
		switch s := stmt.(type) {
		case *syntax.DefStmt:
			defs[s.Name.Name] = s
		case *syntax.AssignStmt:
			if id, ok := s.LHS.(*syntax.Ident); ok {
				constants[id.Name] = true
			}
		}
	}
	// Direct references of each definition, by its global name.
	type refs struct{ calls, globals map[string]bool }
	direct := map[string]refs{}
	for name, def := range defs {
		r := refs{calls: map[string]bool{}, globals: map[string]bool{}}
		// Namespace identifiers used as X in X.name; any other use of a
		// namespace stands for every function in it.
		member := map[*syntax.Ident]bool{}
		syntax.Walk(def, func(n syntax.Node) bool {
			if dot, ok := n.(*syntax.DotExpr); ok {
				if id, ok := dot.X.(*syntax.Ident); ok && predeclared(id) {
					if target := e.Project.Processes[id.Name]; target != nil && target.Functions[dot.Name.Name] != nil {
						member[id] = true
						r.calls[id.Name+"."+dot.Name.Name] = true
					}
				}
			}
			return true
		})
		syntax.Walk(def, func(n syntax.Node) bool {
			id, ok := n.(*syntax.Ident)
			if !ok || id == def.Name {
				return true
			}
			b, ok := id.Binding.(*resolve.Binding)
			if !ok {
				return true
			}
			switch b.Scope {
			case resolve.Global:
				r.globals[id.Name] = true
			case resolve.Predeclared:
				if qualified, ok := aliases[id.Name]; ok {
					r.calls[qualified] = true
				} else if target := e.Project.Processes[id.Name]; target != nil && !member[id] {
					for _, fn := range project.Names(target.Functions) {
						r.calls[id.Name+"."+fn] = true
					}
				}
			}
			return true
		})
		direct[name] = r
	}
	for _, fn := range project.Names(proc.Functions) {
		if !proc.Functions[fn].Implemented {
			continue
		}
		calls, helpers, consts := map[string]bool{}, map[string]bool{}, map[string]bool{}
		// Walk the body and every helper it reaches.
		queue := []string{"__park_body_" + fn}
		seen := map[string]bool{queue[0]: true}
		for len(queue) > 0 {
			r := direct[queue[0]]
			queue = queue[1:]
			for c := range r.calls {
				calls[c] = true
			}
			for g := range r.globals {
				switch {
				case constants[g]:
					consts[g] = true
				case defs[g] != nil && !seen[g]:
					seen[g] = true
					// A public body is reached only through dispatch, never
					// by its renamed global name.
					helpers[g] = true
					queue = append(queue, g)
				}
			}
		}
		e.deps[proc.Name+"."+fn] = Deps{Calls: project.Names(calls), Helpers: project.Names(helpers), Constants: project.Names(consts)}
	}
}

func predeclared(id *syntax.Ident) bool {
	b, ok := id.Binding.(*resolve.Binding)
	return ok && b.Scope == resolve.Predeclared
}
