// Package engine runs pure Starlark functions through example-aware dispatch.
package engine

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"github.com/shanecandoit/ProcessStarlark/internal/source"
	"go.starlark.net/syntax"
)

// FunctionVersion represents a content-addressed version of a function.
// The hash is computed from the normalized function body, its authored examples,
// and the hashes of the functions it calls (transitive dependencies).
type FunctionVersion struct {
	Hash        string            // SHA256 of normalized content
	Process     string            // Process name
	Name        string            // Function name
	Body        string            // Normalized Starlark source
	Examples    []ExampleInput    // Authored examples for this function
	DepHashes   map[string]string // Callee function name -> hash
	Signature   string            // Function signature (name(inputs) output)
	Implemented bool              // Has a body (not example-only)
	Ready       bool              // Has >= 2 examples
	CreatedAt   string            // RFC3339 timestamp
}

// ExampleInput is a simplified example for hashing (input + output).
type ExampleInput struct {
	Input  map[string]any
	Output any
}

// ComputeFunctionHash computes the content hash for a function.
// The hash includes:
//   - Normalized function body (whitespace-insensitive, sorted)
//   - Authored examples (canonicalized inputs + outputs)
//   - Dependency hashes (callee function hashes, sorted by name)
func ComputeFunctionHash(proc *project.Process, f *project.Function, files map[string][]byte, depHashes map[string]string) string {
	h := sha256.New()

	// 1. Function identity
	fmt.Fprintf(h, "%s.%s\x00", proc.Name, f.Name)

	// 2. Signature
	fmt.Fprintf(h, "%s\x00", f.Signature)

	// 3. Implementation status
	if f.Implemented {
		fmt.Fprintf(h, "implemented\x00")
	} else {
		fmt.Fprintf(h, "example-only\x00")
	}

	// 4. Normalized body (for implemented functions)
	if f.Implemented {
		body := normalizeBody(proc, f, files)
		fmt.Fprintf(h, "%s\x00", body)
	}

	// 5. Authored examples (sorted by canonical input key)
	exampleKeys := make([]string, 0, len(f.Examples))
	for _, ex := range f.Examples {
		exampleKeys = append(exampleKeys, samples.Key(ex.Input))
	}
	sort.Strings(exampleKeys)
	for _, key := range exampleKeys {
		// Find the example with this key
		for _, ex := range f.Examples {
			if samples.Key(ex.Input) == key {
				// Include both input and output in hash
				inputBytes, _ := samples.Canonical(ex.Input)
				outputBytes, _ := samples.Canonical(ex.Output)
				h.Write(inputBytes)
				h.Write(outputBytes)
				break
			}
		}
	}
	fmt.Fprintf(h, "\x00") // separator

	// 6. Dependency hashes (sorted by callee name for determinism)
	depNames := make([]string, 0, len(depHashes))
	for name := range depHashes {
		depNames = append(depNames, name)
	}
	sort.Strings(depNames)
	for _, name := range depNames {
		fmt.Fprintf(h, "%s:%s\x00", name, depHashes[name])
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// normalizeBody returns a whitespace-normalized, deterministic representation
// of the function's Starlark body. It strips the Process indentation and
// sorts import-like constructs.
func normalizeBody(proc *project.Process, f *project.Function, files map[string][]byte) string {
	lines := strings.Split(string(files[f.Path]), "\n")
	if f.Line < 1 || f.EndLine > len(lines) || f.Line > f.EndLine {
		return ""
	}
	bodyLines := lines[f.Line-1 : f.EndLine]

	// Remove process indentation
	for i, line := range bodyLines {
		bodyLines[i] = strings.TrimPrefix(line, proc.Indent)
	}

	// Join and normalize whitespace: collapse multiple spaces, trim lines
	body := strings.Join(bodyLines, "\n")
	body = normalizeWhitespace(body)

	return body
}

// normalizeWhitespace collapses runs of whitespace to single spaces,
// trims each line, and removes blank lines for deterministic hashing.
func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Collapse internal whitespace
		line = collapseSpaces(line)
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func collapseSpaces(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}

// ComputeAllFunctionHashes computes hashes for all functions in a project,
// respecting dependency order (callees before callers).
func ComputeAllFunctionHashes(p *project.Project) (map[string]*FunctionVersion, error) {
	// Build dependency graph
	depGraph := buildDepGraph(p)

	// Topological sort: process callees first
	order, err := topologicalSort(depGraph)
	if err != nil {
		return nil, err
	}

	versions := make(map[string]*FunctionVersion)
	computed := make(map[string]string) // qualified name -> hash

	for _, qualified := range order {
		parts := strings.Split(qualified, ".")
		procName, fnName := parts[0], parts[1]
		proc := p.Processes[procName]
		fn := proc.Functions[fnName]

		// Collect dependency hashes from the dependency graph
		depHashes := make(map[string]string)
		for depQualified := range depGraph[qualified] {
			if hash, ok := computed[depQualified]; ok {
				depHashes[depQualified] = hash
			}
		}

		hash := ComputeFunctionHash(proc, fn, p.Files, depHashes)

		// Convert examples for version struct
		examples := make([]ExampleInput, len(fn.Examples))
		for i, ex := range fn.Examples {
			examples[i] = ExampleInput{Input: ex.Input, Output: ex.Output}
		}

		version := &FunctionVersion{
			Hash:        hash,
			Process:     procName,
			Name:        fnName,
			Body:        normalizeBody(proc, fn, p.Files),
			Examples:    examples,
			DepHashes:   depHashes,
			Signature:   fn.Signature,
			Implemented: fn.Implemented,
			Ready:       fn.Ready(),
			CreatedAt:   "", // Filled by caller with timestamp
		}

		versions[qualified] = version
		computed[qualified] = hash
	}

	return versions, nil
}

// buildDepGraph builds a graph of function dependencies.
// Returns map[qualified_fn] -> set of qualified callees.
func buildDepGraph(p *project.Project) map[string]map[string]bool {
	graph := make(map[string]map[string]bool)

	for _, procName := range project.Names(p.Processes) {
		proc := p.Processes[procName]
		for fnName := range proc.Functions {
			qualified := procName + "." + fnName
			graph[qualified] = make(map[string]bool)

			// Direct dependencies from process Uses
			for _, depName := range proc.Uses {
				depProc := p.Processes[depName]
				for depFnName := range depProc.Functions {
					depQualified := depName + "." + depFnName
					graph[qualified][depQualified] = true
				}
			}
		}
	}

	// Add intra-process dependencies by analyzing function bodies
	for _, procName := range project.Names(p.Processes) {
		proc := p.Processes[procName]
		for fnName, fn := range proc.Functions {
			if !fn.Implemented {
				continue
			}
			qualified := procName + "." + fnName
			body, err := ParseFunctionBody(proc, fn, p.Files)
			if err != nil {
				continue
			}
			// Walk the body to find calls to other functions in the same process
			for _, stmt := range body {
				walkForCalls(stmt, procName, fnName, graph[qualified], proc.Functions)
			}
		}
	}

	return graph
}

// walkForCalls walks a syntax node to find function calls within the same process.
func walkForCalls(node syntax.Node, procName string, fnName string, deps map[string]bool, procFuncs map[string]*project.Function) {
	switch n := node.(type) {
	case *syntax.CallExpr:
		// Check if this is a call to a function in the same process
		// Can be bare name (inner) or qualified (Local.inner)
		if ident, ok := n.Fn.(*syntax.Ident); ok {
			if procFuncs[ident.Name] != nil && ident.Name != fnName {
				deps[procName+"."+ident.Name] = true
			}
		} else if dot, ok := n.Fn.(*syntax.DotExpr); ok {
			if ident, ok := dot.X.(*syntax.Ident); ok && ident.Name == procName {
				if procFuncs[dot.Name.Name] != nil && dot.Name.Name != fnName {
					deps[procName+"."+dot.Name.Name] = true
				}
			}
		}
		// Recurse into arguments
		for _, arg := range n.Args {
			walkForCalls(arg, procName, fnName, deps, procFuncs)
		}
	case *syntax.DotExpr:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
	case *syntax.IndexExpr:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
		walkForCalls(n.Y, procName, fnName, deps, procFuncs)
	case *syntax.SliceExpr:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
		if n.Lo != nil {
			walkForCalls(n.Lo, procName, fnName, deps, procFuncs)
		}
		if n.Hi != nil {
			walkForCalls(n.Hi, procName, fnName, deps, procFuncs)
		}
		if n.Step != nil {
			walkForCalls(n.Step, procName, fnName, deps, procFuncs)
		}
	case *syntax.ListExpr:
		for _, elem := range n.List {
			walkForCalls(elem, procName, fnName, deps, procFuncs)
		}
	case *syntax.TupleExpr:
		for _, elem := range n.List {
			walkForCalls(elem, procName, fnName, deps, procFuncs)
		}
	case *syntax.DictExpr:
		for _, entry := range n.List {
			if de, ok := entry.(*syntax.DictEntry); ok {
				walkForCalls(de.Key, procName, fnName, deps, procFuncs)
				walkForCalls(de.Value, procName, fnName, deps, procFuncs)
			}
		}
	case *syntax.UnaryExpr:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
	case *syntax.BinaryExpr:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
		walkForCalls(n.Y, procName, fnName, deps, procFuncs)
	case *syntax.ParenExpr:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
	case *syntax.CondExpr:
		walkForCalls(n.Cond, procName, fnName, deps, procFuncs)
		walkForCalls(n.True, procName, fnName, deps, procFuncs)
		walkForCalls(n.False, procName, fnName, deps, procFuncs)
	case *syntax.LambdaExpr:
		walkForCalls(n.Body, procName, fnName, deps, procFuncs)
	case *syntax.DefStmt:
		for _, stmt := range n.Body {
			walkForCalls(stmt, procName, fnName, deps, procFuncs)
		}
	case *syntax.ReturnStmt:
		if n.Result != nil {
			walkForCalls(n.Result, procName, fnName, deps, procFuncs)
		}
	case *syntax.ExprStmt:
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
	case *syntax.AssignStmt:
		walkForCalls(n.RHS, procName, fnName, deps, procFuncs)
	case *syntax.IfStmt:
		walkForCalls(n.Cond, procName, fnName, deps, procFuncs)
		for _, stmt := range n.True {
			walkForCalls(stmt, procName, fnName, deps, procFuncs)
		}
		for _, stmt := range n.False {
			walkForCalls(stmt, procName, fnName, deps, procFuncs)
		}
	case *syntax.ForStmt:
		walkForCalls(n.Vars, procName, fnName, deps, procFuncs)
		walkForCalls(n.X, procName, fnName, deps, procFuncs)
		for _, stmt := range n.Body {
			walkForCalls(stmt, procName, fnName, deps, procFuncs)
		}
	case *syntax.WhileStmt:
		walkForCalls(n.Cond, procName, fnName, deps, procFuncs)
		for _, stmt := range n.Body {
			walkForCalls(stmt, procName, fnName, deps, procFuncs)
		}
	case *syntax.LoadStmt:
		// load() is not allowed in Park
	}
}

// topologicalSort returns functions in dependency order (callees first).
func topologicalSort(graph map[string]map[string]bool) ([]string, error) {
	visited := make(map[string]bool)
	visiting := make(map[string]bool)
	var order []string

	var visit func(string) error
	visit = func(node string) error {
		if visiting[node] {
			return fmt.Errorf("dependency cycle involving %s", node)
		}
		if visited[node] {
			return nil
		}
		visiting[node] = true
		for dep := range graph[node] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[node] = false
		visited[node] = true
		order = append(order, node)
		return nil
	}

	for node := range graph {
		if !visited[node] {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}

	return order, nil
}

// SortedFunctionNames returns function names in deterministic order.
func SortedFunctionNames(hashes map[string]string) []string {
	names := make([]string, 0, len(hashes))
	for name := range hashes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ParseFunctionBody parses a function's body from source and returns
// the body statements for normalization.
func ParseFunctionBody(proc *project.Process, f *project.Function, files map[string][]byte) ([]syntax.Stmt, error) {
	module, err := source.Parse(f.Path, files[f.Path])
	if err != nil {
		return nil, err
	}
	for _, stmt := range module.File.Stmts {
		if def, ok := stmt.(*syntax.DefStmt); ok && def.Name.Name == f.Name {
			return def.Body, nil
		}
	}
	return nil, fmt.Errorf("function %s not found in %s", f.Name, f.Path)
}

// CanonicalCacheKey returns a canonical cache key for a function call.
// Uses the function's content hash + canonical input.
func CanonicalCacheKey(fnHash string, input map[string]any) string {
	inputKey := samples.Key(input)
	h := sha256.New()
	h.Write([]byte(fnHash + "\x00" + inputKey))
	return fmt.Sprintf("%x", h.Sum(nil))
}
