// Package engine runs pure Starlark functions through example-aware dispatch.
package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/samples"
	"github.com/shanecandoit/ProcessStarlark/internal/source"
	"go.starlark.net/syntax"
)

// RuntimeIdentity must change when dispatch, builtins, limits, or the syntax
// encoding changes. The pinned interpreter version is part of the contract.
const RuntimeIdentity = project.CodecVersion + "/function-graph-2/starlark-4f4267fb140d"

// FunctionVersion represents a content-addressed version of a function.
// Identity covers parsed syntax, authored examples, runtime identity, and the
// complete reachable dependency graph, including recursive groups.
type FunctionVersion struct {
	Hash        string            // SHA256 of normalized content
	Process     string            // Process name
	Name        string            // Function name
	Body        string            // Canonical syntax JSON
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

// syntaxValue serializes parsed syntax, rather than source lines. The concrete
// node type and nested statement lists preserve block structure. Positions,
// comments and resolver metadata cannot affect execution and are excluded.
// Literal values retain whitespace, escapes, and exact integer precision.
func syntaxValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		if lit, ok := v.Interface().(*syntax.Literal); ok {
			return map[string]any{"node": "Literal", "token": int(lit.Token), "value": lit.Value}
		}
		return syntaxValue(v.Elem())
	}
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]any{"node": v.Type().Name()}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() || f.Type == reflect.TypeOf(syntax.Position{}) || f.Name == "Binding" || f.Name == "Function" {
				continue
			}
			out[f.Name] = syntaxValue(v.Field(i))
		}
		return out
	case reflect.Slice:
		out := make([]any, v.Len())
		for i := range out {
			out[i] = syntaxValue(v.Index(i))
		}
		return out
	default:
		return v.Interface()
	}
}

func versionDigest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	} // Only syntax and validated canonical values.
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// ComputeAllFunctionHashes uses exactly the bindings used by evaluation.
func ComputeAllFunctionHashes(p *project.Project) (map[string]*FunctionVersion, error) {
	e, err := New(p)
	if err != nil {
		return nil, err
	}
	return e.versions, nil
}

func (e *Engine) computeFunctionVersions() (map[string]*FunctionVersion, error) {
	versions := map[string]*FunctionVersion{}
	local := map[string]string{}
	for _, procName := range project.Names(e.Project.Processes) {
		proc := e.Project.Processes[procName]
		parsed, err := source.Parse(proc.Module, e.Project.Files[proc.Module])
		if err != nil {
			return nil, err
		}
		definitions := map[string]any{}
		for _, stmt := range parsed.File.Stmts {
			switch s := stmt.(type) {
			case *syntax.DefStmt:
				definitions[s.Name.Name] = syntaxValue(reflect.ValueOf(s))
			case *syntax.AssignStmt:
				if id, ok := s.LHS.(*syntax.Ident); ok {
					definitions[id.Name] = syntaxValue(reflect.ValueOf(s))
				}
			}
		}
		for _, name := range project.Names(proc.Functions) {
			fn := proc.Functions[name]
			qualified := procName + "." + name
			body := definitions[name]
			shared := map[string]any{}
			deps := e.deps[qualified]
			for _, helper := range deps.Helpers {
				shared[helper] = definitions[helper]
			}
			for _, constant := range deps.Constants {
				shared[constant] = definitions[constant]
			}
			examples := map[string]string{}
			pairs := make([]ExampleInput, 0, len(fn.Examples))
			for _, ex := range fn.Examples {
				examples[samples.Key(ex.Input)] = samples.Key(ex.Output)
				pairs = append(pairs, ExampleInput{Input: ex.Input, Output: ex.Output})
			}
			local[qualified] = versionDigest(struct {
				Runtime, Name, Signature string
				Implemented              bool
				Body                     any
				Shared                   map[string]any
				Examples                 map[string]string
				Calls                    []string
			}{RuntimeIdentity, qualified, fn.Signature, fn.Implemented, body, shared, examples, deps.Calls})
			encodedBody, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			versions[qualified] = &FunctionVersion{Process: procName, Name: name, Body: string(encodedBody), Examples: pairs,
				Signature: fn.Signature, Implemented: fn.Implemented, Ready: fn.Ready()}
		}
	}
	// A sorted reachable graph handles self and mutual recursion without a
	// cyclic hash equation. Each node includes its edges and local identity.
	for _, qualified := range project.Names(versions) {
		graph := map[string]string{}
		for _, reachable := range e.Reach(qualified) {
			graph[reachable] = local[reachable]
		}
		versions[qualified].Hash = versionDigest(graph)
	}
	for name, v := range versions {
		v.DepHashes = map[string]string{}
		for _, dep := range e.deps[name].Calls {
			v.DepHashes[dep] = versions[dep].Hash
		}
	}
	return versions, nil
}

// SortedFunctionNames returns qualified function names in deterministic order.
func SortedFunctionNames(hashes map[string]string) []string { return project.Names(hashes) }
