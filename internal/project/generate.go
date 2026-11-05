package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/syntax"
	"go.yaml.in/yaml/v3"
	"park/internal/samples"
	"park/internal/source"
)

// Generate creates or refreshes a sidecar while retaining authored function data.
// Public Process functions become exports; underscore-prefixed helpers stay private.
func Generate(p *Project, module, name string, explicitUses []string) (string, []byte, error) {
	module = filepath.Clean(module)
	if filepath.Dir(module) != "." || filepath.Ext(module) != ".star" {
		return "", nil, fmt.Errorf("register expects a .star file in the project directory")
	}
	path, err := SafePath(p.Root, module)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
		return "", nil, fmt.Errorf("invalid module file %s", module)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	parsed, err := source.Parse(module, contents)
	if err != nil {
		return "", nil, err
	}
	file := parsed.File
	rel := strings.TrimSuffix(module, ".star") + ".yaml"
	var old *Process
	for _, proc := range p.Processes {
		if proc.ManifestPath == rel {
			old = proc
			break
		}
	}
	if old != nil {
		if name != "" && name != old.Name {
			return "", nil, fmt.Errorf("cannot rename existing process %s during registration", old.Name)
		}
		name = old.Name
	}
	if parsed.Name != "" {
		if name != "" && name != parsed.Name {
			return "", nil, fmt.Errorf("%s: Process %s does not match registered/requested name %s", module, parsed.Name, name)
		}
		name = parsed.Name
	}
	if name == "" {
		for _, part := range strings.Split(strings.TrimSuffix(module, ".star"), "_") {
			if part != "" {
				name += strings.ToUpper(part[:1]) + part[1:]
			}
		}
	}
	if !ValidName(name) {
		return "", nil, fmt.Errorf("invalid process name %q; supply --name", name)
	}
	if other := p.Processes[name]; other != nil && other.ManifestPath != rel {
		return "", nil, fmt.Errorf("process %s already registered in %s", name, other.ManifestPath)
	}
	var doc *yaml.Node
	if old != nil {
		doc = old.Document
	} else {
		doc = samples.ValueNode(map[string]any{})
		SetField(doc, "version", samples.ValueNode(1))
		SetField(doc, "name", samples.ValueNode(name))
		if description := docString(file.Stmts); description != "" {
			SetField(doc, "description", samples.ValueNode(description))
		}
		SetField(doc, "module", samples.ValueNode(module))
		SetField(doc, "uses", samples.ValueNode([]string{}))
		SetField(doc, "functions", samples.ValueNode([]any{}))
	}
	SetField(doc, "module", samples.ValueNode(module))
	if description := docString(file.Stmts); description != "" {
		SetField(doc, "description", samples.ValueNode(description))
	}
	definitions := map[string]*syntax.DefStmt{}
	for _, stmt := range file.Stmts {
		if def, ok := stmt.(*syntax.DefStmt); ok {
			if definitions[def.Name.Name] != nil {
				return "", nil, fmt.Errorf("duplicate definition %s", def.Name.Name)
			}
			definitions[def.Name.Name] = def
		}
	}
	used := map[string]bool{}
	if old != nil {
		for _, fn := range Names(old.Functions) {
			f := old.Functions[fn]
			impl := f.Implementation
			if impl == "" {
				impl = f.Name
			}
			if def := definitions[impl]; def != nil {
				SetField(f.Node, "implementation", samples.ValueNode(impl))
				used[impl] = true
				if description := docString(def.Body); description != "" {
					SetField(f.Node, "description", samples.ValueNode(description))
				}
			} else {
				RemoveField(f.Node, "implementation")
			}
		}
	}
	seq := Field(doc, "functions")
	for _, fn := range Names(definitions) {
		if strings.HasPrefix(fn, "_") || used[fn] {
			continue
		}
		entry := FunctionNode(fn+"(in) out", docString(definitions[fn].Body), fn)
		seq.Content = append(seq.Content, entry)
	}
	uses := map[string]bool{}
	if old != nil {
		for _, dep := range old.Uses {
			uses[dep] = true
		}
	}
	for _, dep := range explicitUses {
		if dep != "" {
			uses[dep] = true
		}
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		if dot, ok := n.(*syntax.DotExpr); ok {
			if id, ok := dot.X.(*syntax.Ident); ok && id.Name != name && p.Processes[id.Name] != nil {
				uses[id.Name] = true
			}
		}
		return true
	})
	SetField(doc, "uses", samples.ValueNode(Names(uses)))
	b, err := samples.EncodeYAML(doc)
	return rel, b, err
}
func docString(stmts []syntax.Stmt) string {
	if len(stmts) > 0 {
		if expr, ok := stmts[0].(*syntax.ExprStmt); ok {
			if lit, ok := expr.X.(*syntax.Literal); ok && lit.Token == syntax.STRING {
				if s, ok := lit.Value.(string); ok {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

// FunctionNode keeps generated metadata in the same readable order as examples.
func FunctionNode(signature, description, implementation string) *yaml.Node {
	node := samples.ValueNode(map[string]any{})
	SetField(node, "function", samples.ValueNode(signature))
	if description != "" {
		SetField(node, "description", samples.ValueNode(description))
	}
	if implementation != "" {
		SetField(node, "implementation", samples.ValueNode(implementation))
	}
	SetField(node, "examples", samples.ValueNode([]any{}))
	return node
}
