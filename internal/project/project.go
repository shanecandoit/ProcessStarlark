// Package project loads and validates Park projects without executing code.
// A project is a flat directory of .star modules. Each module is one process;
// its functions, signatures, descriptions, and examples all come from source.
package project

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.starlark.net/syntax"
	"park/internal/samples"
	"park/internal/source"
)

const MaxFileBytes = 2 << 20
const MaxProjectBytes = 32 << 20
const MaxFiles = 2048
const CodecVersion = "park-v2-codec-2-engine-4"

// PlaceholderOutput is the output name of example-only functions, and the
// conventional name for implementations to return.
const PlaceholderOutput = "result"

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var keywords = strings.Fields("and as assert break class continue def del elif else except finally for from global if import in is lambda load nonlocal not or pass raise return try while with yield True False None")

func ValidName(name string) bool {
	if !identifier.MatchString(name) || strings.HasPrefix(name, "__park_") {
		return false
	}
	return !slices.Contains(keywords, name)
}

// FormatSignature writes the canonical spelling of a signature.
func FormatSignature(name string, inputs []string, output string) string {
	return name + "(" + strings.Join(inputs, ", ") + ") " + output
}

// FormatCall renders a call as Process.fn(name=value, ...) in signature order.
func FormatCall(qualified string, names []string, input map[string]any) string {
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = name + "=" + samples.Literal(input[name])
	}
	return qualified + "(" + strings.Join(parts, ", ") + ")"
}

// Example is one authored example, located where it was written.
type Example struct {
	Input         map[string]any // Named inputs.
	Output        any
	Path          string
	Line, EndLine int
}

type Function struct {
	Name, Description string
	Implemented       bool // False for example-only functions (`def f(x): pass`).
	Signature, Output string
	Inputs            []string // Named inputs, in parameter order.
	Path              string   // Module that declares the function.
	Line              int
	Examples          []Example
	Index             map[string]any // Canonical input -> output.
}

func (f *Function) State() string {
	if !f.Implemented {
		return "example-only"
	}
	if len(f.Examples) < 2 {
		return "needs examples"
	}
	return "implemented"
}

// BindInput checks that v is an object naming exactly the function's inputs.
func (f *Function) BindInput(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s expects inputs %s", f.Name, strings.Join(f.Inputs, ", "))
	}
	for _, name := range f.Inputs {
		if _, ok := m[name]; !ok {
			return nil, fmt.Errorf("%s: missing input %s", f.Name, name)
		}
	}
	if len(m) != len(f.Inputs) {
		for _, name := range Names(m) {
			if !slices.Contains(f.Inputs, name) {
				return nil, fmt.Errorf("%s: unknown input %s; expected %s", f.Name, name, strings.Join(f.Inputs, ", "))
			}
		}
	}
	return m, nil
}

// BindArgs maps positional arguments (in signature order) and named arguments
// onto the function's inputs. Both styles produce the same input object, so
// they share examples and cached results.
func (f *Function) BindArgs(qualified string, args []any, names []string, values []any) (map[string]any, error) {
	if len(args) > len(f.Inputs) {
		return nil, fmt.Errorf("%s takes %d inputs (%s), got %d positional arguments", qualified, len(f.Inputs), strings.Join(f.Inputs, ", "), len(args))
	}
	input := map[string]any{}
	for i, v := range args {
		input[f.Inputs[i]] = v
	}
	for i, name := range names {
		if !slices.Contains(f.Inputs, name) {
			return nil, fmt.Errorf("%s: unknown input %s; expected %s", qualified, name, strings.Join(f.Inputs, ", "))
		}
		if _, dup := input[name]; dup {
			return nil, fmt.Errorf("%s: input %s given more than once", qualified, name)
		}
		input[name] = values[i]
	}
	for _, name := range f.Inputs {
		if _, ok := input[name]; !ok {
			return nil, fmt.Errorf("%s: missing input %s", qualified, name)
		}
	}
	return input, nil
}

type Process struct {
	Name, Description, Module string
	Uses                      []string
	Functions                 map[string]*Function
	// Layout used when inserting example lines into the module.
	Indent     string // Body indentation ("" for regular modules).
	Header     int    // Line of `Process Name:`, or 0.
	DocEnd     int    // Last line of the process docstring, or 0.
	ExampleEnd int    // Last line of the module's last example, or 0.
}

type Project struct {
	Root, Revision string
	Processes      map[string]*Process
	Files          map[string][]byte
}

func Names[T any](m map[string]T) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// SafePath resolves existing symlinks, including parent directories of new files.
func SafePath(root, relative string) (string, error) {
	canonicalRoot, err := Root(root)
	if err != nil {
		return "", err
	}
	root = canonicalRoot
	if filepath.IsAbs(relative) || relative == "" {
		return "", fmt.Errorf("expected project-relative path: %q", relative)
	}
	relative = filepath.Clean(relative)
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root: %q", relative)
	}
	path := filepath.Join(root, relative)
	probe := path
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			rel, err := filepath.Rel(root, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("symlink escapes project root: %s", relative)
			}
			return path, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if _, err := os.Lstat(probe); err == nil {
			return "", fmt.Errorf("dangling symlink: %s", relative)
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fmt.Errorf("cannot resolve %s", relative)
		}
		probe = parent
	}
}

func Root(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// Discover checks only start for a Park project; it never searches ancestors.
func Discover(start string) (string, error) {
	root, err := Root(start)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.Name() == ".park" && entry.IsDir() {
			return root, nil
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".star") {
			return root, nil
		}
	}
	return "", fmt.Errorf("no Park project found in current directory; use park init or --root PATH")
}

// ProcessName derives a regular module's process name: roman_parse.star -> RomanParse.
func ProcessName(module string) string {
	name := ""
	for _, part := range strings.Split(strings.TrimSuffix(module, ".star"), "_") {
		if part != "" {
			name += strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return name
}

type pending struct {
	process string
	example source.Example
}

// LoadWith validates a proposed flat project snapshot; nil contents remove files.
func LoadWith(root string, overlay map[string][]byte) (*Project, error) {
	canonicalRoot, err := Root(root)
	if err != nil {
		return nil, err
	}
	root = canonicalRoot
	p := &Project{Root: root, Processes: map[string]*Process{}, Files: map[string][]byte{}}
	candidates := map[string]bool{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".star") {
			candidates[entry.Name()] = true
		}
	}
	for path, b := range overlay {
		if _, err := SafePath(root, path); err != nil {
			return nil, err
		}
		if strings.HasSuffix(path, ".star") {
			if filepath.Dir(path) != "." {
				return nil, fmt.Errorf("modules must be in project root: %s", path)
			}
			if b == nil {
				delete(candidates, path)
			} else {
				candidates[path] = true
			}
		}
	}
	if len(candidates) > MaxFiles {
		return nil, fmt.Errorf("project exceeds %d files", MaxFiles)
	}
	total := 0
	var examples []pending
	for _, rel := range Names(candidates) {
		b, ok := overlay[rel]
		if !ok {
			path, err := SafePath(root, rel)
			if err != nil {
				return nil, err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
				return nil, fmt.Errorf("%s: expected regular file no larger than %d bytes", rel, MaxFileBytes)
			}
			if b, err = os.ReadFile(path); err != nil {
				return nil, err
			}
		}
		if len(b) > MaxFileBytes {
			return nil, fmt.Errorf("%s exceeds file limit", rel)
		}
		if total += len(b); total > MaxProjectBytes {
			return nil, fmt.Errorf("project size limit exceeded")
		}
		p.Files[rel] = b
		proc, module, err := parseProcess(rel, b)
		if err != nil {
			return nil, err
		}
		if other := p.Processes[proc.Name]; other != nil {
			return nil, fmt.Errorf("process %s is declared in both %s and %s", proc.Name, other.Module, rel)
		}
		p.Processes[proc.Name] = proc
		for _, ex := range module.Examples {
			examples = append(examples, pending{proc.Name, ex})
		}
	}
	for _, name := range Names(p.Processes) {
		if err := p.inferUses(p.Processes[name]); err != nil {
			return nil, err
		}
	}
	if err := p.checkCycles(); err != nil {
		return nil, err
	}
	// A function's examples list those in its own module first, then those
	// written elsewhere, each in file and line order.
	for _, own := range []bool{true, false} {
		for _, item := range examples {
			if (item.example.Call.Process == "" || item.example.Call.Process == item.process) != own {
				continue
			}
			if err := p.addExample(item); err != nil {
				return nil, err
			}
		}
	}
	h := sha256.New()
	fmt.Fprintln(h, CodecVersion)
	for _, path := range Names(p.Files) {
		b := p.Files[path]
		fmt.Fprintf(h, "%d:%s%d:", len(path), path, len(b))
		h.Write(b)
	}
	p.Revision = fmt.Sprintf("%x", h.Sum(nil))
	return p, nil
}

func parseProcess(path string, b []byte) (*Process, *source.Module, error) {
	module, err := source.Parse(path, b)
	if err != nil {
		return nil, nil, err
	}
	proc := &Process{Name: module.Name, Module: path, Functions: map[string]*Function{}, Indent: module.Indent, Header: module.Header}
	if proc.Name == "" {
		proc.Name = ProcessName(path)
	}
	if !ValidName(proc.Name) {
		return nil, nil, fmt.Errorf("%s: invalid process name %q", path, proc.Name)
	}
	if len(module.File.Stmts) > 0 {
		if doc, ok := docString(module.File.Stmts[:1]); ok {
			proc.Description = doc
			_, end := module.File.Stmts[0].Span()
			proc.DocEnd = int(end.Line)
		}
	}
	for _, ex := range module.Examples {
		proc.ExampleEnd = max(proc.ExampleEnd, ex.EndLine)
	}
	for _, stmt := range module.File.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if !ok || strings.HasPrefix(def.Name.Name, "_") {
			continue
		}
		f, err := parseFunction(path, def)
		if err != nil {
			return nil, nil, err
		}
		if proc.Functions[f.Name] != nil {
			return nil, nil, fmt.Errorf("%s: duplicate definition %s", def.Def, f.Name)
		}
		proc.Functions[f.Name] = f
	}
	return proc, module, nil
}

// parseFunction reads a signature from a def: inputs from its parameters and
// the output from the variable that every return statement returns.
func parseFunction(path string, def *syntax.DefStmt) (*Function, error) {
	name := def.Name.Name
	if !ValidName(name) {
		return nil, fmt.Errorf("%s: invalid function name %q", def.Def, name)
	}
	f := &Function{Name: name, Path: path, Line: int(def.Def.Line), Index: map[string]any{}}
	for _, param := range def.Params {
		id, ok := param.(*syntax.Ident)
		if !ok {
			return nil, fmt.Errorf("%s: %s cannot use defaults, *args, or **kwargs", def.Def, name)
		}
		f.Inputs = append(f.Inputs, id.Name)
	}
	if len(f.Inputs) == 0 {
		return nil, fmt.Errorf("%s: %s needs at least one input", def.Def, name)
	}
	body := def.Body
	if doc, ok := docString(body); ok {
		f.Description = doc
		body = body[1:]
	}
	f.Implemented = slices.ContainsFunc(body, func(s syntax.Stmt) bool {
		b, ok := s.(*syntax.BranchStmt)
		return !ok || b.Token != syntax.PASS
	})
	if !f.Implemented {
		f.Output = PlaceholderOutput
	} else {
		var walkErr error
		visit := func(n syntax.Node) bool {
			if walkErr != nil {
				return false
			}
			switch n := n.(type) {
			case *syntax.DefStmt, *syntax.LambdaExpr:
				return false // Nested functions have their own returns.
			case *syntax.ReturnStmt:
				id, ok := n.Result.(*syntax.Ident)
				if !ok {
					walkErr = fmt.Errorf("%s: %s must return a named variable, e.g. `result = ...` then `return result`; the name becomes the output's name", n.Return, name)
					return false
				}
				if f.Output != "" && f.Output != id.Name {
					walkErr = fmt.Errorf("%s: %s returns both %s and %s; every return must use the same name", n.Return, name, f.Output, id.Name)
					return false
				}
				f.Output = id.Name
			}
			return true
		}
		for _, stmt := range body {
			syntax.Walk(stmt, visit)
		}
		if walkErr != nil {
			return nil, walkErr
		}
		if f.Output == "" {
			return nil, fmt.Errorf("%s: %s must end with `return <name>`, e.g. `return result`; use `pass` for an example-only function", def.Def, name)
		}
	}
	if slices.Contains(f.Inputs, f.Output) {
		return nil, fmt.Errorf("%s: %s returns its input %s; return a new name so the output is named differently", def.Def, name, f.Output)
	}
	f.Signature = FormatSignature(name, f.Inputs, f.Output)
	return f, nil
}

// inferUses records the processes a module's code refers to as Process.member.
func (p *Project) inferUses(proc *Process) error {
	module, err := source.Parse(proc.Module, p.Files[proc.Module])
	if err != nil {
		return err
	}
	uses := map[string]bool{}
	syntax.Walk(module.File, func(n syntax.Node) bool {
		if dot, ok := n.(*syntax.DotExpr); ok {
			if id, ok := dot.X.(*syntax.Ident); ok && id.Name != proc.Name && p.Processes[id.Name] != nil {
				uses[id.Name] = true
			}
		}
		return true
	})
	proc.Uses = Names(uses)
	return nil
}

func (p *Project) checkCycles() error {
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("dependency cycle at %s", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, dep := range p.Processes[name].Uses {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}
	for _, name := range Names(p.Processes) {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

// addExample resolves an example against the project-wide function set. An
// example may be written in any module and name any declared function.
func (p *Project) addExample(item pending) error {
	ex := item.example
	where := fmt.Sprintf("%s:%d", p.Processes[item.process].Module, ex.Line)
	target := ex.Call.Process
	if target == "" {
		target = item.process
	}
	qualified := target + "." + ex.Call.Function
	proc := p.Processes[target]
	if proc == nil {
		return fmt.Errorf("%s: example names unknown process %s", where, target)
	}
	f := proc.Functions[ex.Call.Function]
	if f == nil {
		return fmt.Errorf("%s: example names unknown function %s; declare it, e.g. `def %s(...): pass`", where, qualified, ex.Call.Function)
	}
	input, err := f.BindArgs(qualified, ex.Call.Args, ex.Call.Names, ex.Call.Values)
	if err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	key := samples.Key(input)
	if _, ok := f.Index[key]; ok {
		for _, prior := range f.Examples {
			if samples.Key(prior.Input) == key {
				return fmt.Errorf("%s: duplicate example for %s; first written at %s:%d", where, FormatCall(qualified, f.Inputs, input), prior.Path, prior.Line)
			}
		}
	}
	f.Index[key] = ex.Want
	f.Examples = append(f.Examples, Example{Input: input, Output: ex.Want, Path: p.Processes[item.process].Module, Line: ex.Line, EndLine: ex.EndLine})
	return nil
}

func docString(stmts []syntax.Stmt) (string, bool) {
	if len(stmts) > 0 {
		if expr, ok := stmts[0].(*syntax.ExprStmt); ok {
			if lit, ok := expr.X.(*syntax.Literal); ok && lit.Token == syntax.STRING {
				if s, ok := lit.Value.(string); ok {
					return strings.TrimSpace(s), true
				}
			}
		}
	}
	return "", false
}

func (p *Project) Function(qualified string) (*Process, *Function, error) {
	parts := strings.Split(qualified, ".")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("expected Process.function, got %q", qualified)
	}
	proc := p.Processes[parts[0]]
	if proc == nil {
		return nil, nil, fmt.Errorf("unknown process %s", parts[0])
	}
	f := proc.Functions[parts[1]]
	if f == nil {
		return nil, nil, fmt.Errorf("unknown function %s", qualified)
	}
	return proc, f, nil
}
