// Package project loads and validates Park projects without executing code.
package project

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
	"park/internal/samples"
)

const MaxFileBytes = 2 << 20
const MaxProjectBytes = 32 << 20
const MaxFiles = 2048
const CodecVersion = "park-v1-codec-1-engine-1"

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var signature = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\(([A-Za-z_][A-Za-z0-9_]*)\) ([A-Za-z_][A-Za-z0-9_]*)$`)
var keywords = strings.Fields("and as assert break class continue def del elif else except finally for from global if import in is lambda load nonlocal not or pass raise return try while with yield True False None")

func ValidName(name string) bool {
	if !identifier.MatchString(name) || strings.HasPrefix(name, "__park_") {
		return false
	}
	for _, k := range keywords {
		if name == k {
			return false
		}
	}
	return true
}

func Signature(s string) (name, input, output string, err error) {
	m := signature.FindStringSubmatch(s)
	if m == nil || !ValidName(m[1]) || m[2] == m[3] {
		return "", "", "", fmt.Errorf("invalid signature %q; expected name(input) output with distinct labels", s)
	}
	return m[1], m[2], m[3], nil
}

type Function struct {
	Name, Description, Implementation               string
	Signature, InputLabel, OutputLabel, ExamplePath string
	Document                                        *yaml.Node
	Node                                            *yaml.Node
	Examples                                        []samples.Pair
	Index                                           map[string]any
}

func (f *Function) State() string {
	if f.Implementation == "" {
		return "example-only"
	}
	if len(f.Examples) < 2 {
		return "needs examples"
	}
	return "implemented"
}

type Process struct {
	Name, Description, Module, ManifestPath string
	Uses                                    []string
	Functions                               map[string]*Function
	Document                                *yaml.Node
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
func Discover(start string) (string, error) {
	root, err := Root(start)
	if err != nil {
		return "", err
	}
	for {
		entries, err := os.ReadDir(root)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if entry.Name() == ".park" && entry.IsDir() {
				return root, nil
			}
			if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".star") || strings.HasSuffix(entry.Name(), ".yaml")) {
				return root, nil
			}
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("no Park project found; use park init or --root PATH")
		}
		root = parent
	}
}

// LoadWith validates a proposed flat project snapshot; nil contents remove files.
func LoadWith(root string, overlay map[string][]byte) (*Project, error) {
	canonicalRoot, rootErr := Root(root)
	if rootErr != nil {
		return nil, rootErr
	}
	root = canonicalRoot
	p := &Project{Root: root, Processes: map[string]*Process{}, Files: map[string][]byte{}}
	candidates := map[string]bool{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			candidates[name] = true
		}
	}
	for path, b := range overlay {
		if _, err := SafePath(root, path); err != nil {
			return nil, err
		}
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			if filepath.Dir(path) != "." {
				return nil, fmt.Errorf("sidecars must be in project root: %s", path)
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
	read := func(rel string) ([]byte, error) {
		if existing, ok := p.Files[rel]; ok {
			return existing, nil
		}
		var b []byte
		if proposed, ok := overlay[rel]; ok {
			if proposed == nil {
				return nil, fmt.Errorf("missing file %s", rel)
			}
			b = proposed
		} else {
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
			b, err = os.ReadFile(path)
			if err != nil {
				return nil, err
			}
		}
		if len(b) > MaxFileBytes {
			return nil, fmt.Errorf("%s exceeds file limit", rel)
		}
		total += len(b)
		if total > MaxProjectBytes || len(p.Files) >= MaxFiles {
			return nil, fmt.Errorf("project size limit exceeded")
		}
		p.Files[rel] = b
		return b, nil
	}
	for _, rel := range Names(candidates) {
		b, err := read(rel)
		if err != nil {
			return nil, err
		}
		proc, err := parseProcess(rel, b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		if _, ok := p.Processes[proc.Name]; ok {
			return nil, fmt.Errorf("duplicate process %s", proc.Name)
		}
		p.Processes[proc.Name] = proc
	}
	for _, name := range Names(p.Processes) {
		proc := p.Processes[name]
		for _, dep := range proc.Uses {
			if p.Processes[dep] == nil {
				return nil, fmt.Errorf("%s uses missing process %s", name, dep)
			}
		}
		implementations := map[string]bool{}
		for _, fnName := range Names(proc.Functions) {
			f := proc.Functions[fnName]
			if f.Implementation != "" {
				if proc.Module == "" {
					return nil, fmt.Errorf("%s.%s: implementation needs a module", name, fnName)
				}
				if implementations[f.Implementation] {
					return nil, fmt.Errorf("%s: implementation %s registered more than once", name, f.Implementation)
				}
				implementations[f.Implementation] = true
			}
		}
		if proc.Module != "" {
			if filepath.Dir(proc.Module) != "." || filepath.Ext(proc.Module) != ".star" {
				return nil, fmt.Errorf("%s: module must be a .star file beside its YAML sidecar", name)
			}
			if strings.TrimSuffix(proc.ManifestPath, filepath.Ext(proc.ManifestPath)) != strings.TrimSuffix(proc.Module, ".star") {
				return nil, fmt.Errorf("%s: YAML sidecar must match module basename", name)
			}
			if _, err := read(proc.Module); err != nil {
				return nil, fmt.Errorf("%s module: %w", name, err)
			}
		}
	}
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
			return nil, err
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

func parseProcess(path string, b []byte) (*Process, error) {
	doc, err := samples.ParseYAML(b)
	if err != nil {
		return nil, err
	}
	fields, err := samples.Fields(doc.Content[0], "version", "name", "description", "module", "uses", "functions")
	if err != nil {
		return nil, err
	}
	version := fields["version"]
	if version == nil || version.Tag != "!!int" || version.Value != "1" {
		return nil, fmt.Errorf("version must be 1")
	}
	name, err := samples.Text(fields["name"])
	if err != nil || !ValidName(name) {
		return nil, fmt.Errorf("invalid process name %q", name)
	}
	p := &Process{Name: name, ManifestPath: path, Document: doc, Functions: map[string]*Function{}}
	if n := fields["description"]; n != nil {
		p.Description, err = samples.Text(n)
		if err != nil {
			return nil, err
		}
	}
	if n := fields["module"]; n != nil {
		p.Module, err = samples.Text(n)
		if err != nil {
			return nil, err
		}
		if filepath.IsAbs(p.Module) {
			return nil, fmt.Errorf("module path must be relative")
		}
	}
	if n := fields["uses"]; n != nil {
		if n.Kind != yaml.SequenceNode {
			return nil, samples.At(n, "uses must be a list")
		}
		seen := map[string]bool{}
		for _, item := range n.Content {
			dep, err := samples.Text(item)
			if err != nil || !ValidName(dep) || seen[dep] {
				return nil, samples.At(item, "invalid or duplicate dependency")
			}
			seen[dep] = true
			p.Uses = append(p.Uses, dep)
		}
	}
	n := fields["functions"]
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("functions must be a list")
	}
	for _, entry := range n.Content {
		f, err := parseFunction(entry, doc, path)
		if err != nil {
			return nil, err
		}
		if p.Functions[f.Name] != nil {
			return nil, samples.At(entry, "duplicate function %s", f.Name)
		}
		p.Functions[f.Name] = f
	}
	return p, nil
}

func parseFunction(n, doc *yaml.Node, path string) (*Function, error) {
	fields, err := samples.Fields(n, "function", "description", "implementation", "examples")
	if err != nil {
		return nil, err
	}
	s, err := samples.Text(fields["function"])
	if err != nil {
		return nil, err
	}
	name, in, out, err := Signature(s)
	if err != nil {
		return nil, err
	}
	f := &Function{Name: name, Signature: s, InputLabel: in, OutputLabel: out, Index: map[string]any{}, Document: doc, Node: n, ExamplePath: path}
	if n := fields["description"]; n != nil {
		f.Description, err = samples.Text(n)
		if err != nil {
			return nil, err
		}
	}
	if n := fields["implementation"]; n != nil {
		f.Implementation, err = samples.Text(n)
		if err != nil || !ValidName(f.Implementation) {
			return nil, samples.At(n, "invalid implementation name")
		}
	}
	seq := fields["examples"]
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil, samples.At(n, "examples must be a list")
	}
	for _, row := range seq.Content {
		pair, err := samples.Fields(row, in, out)
		if err != nil {
			return nil, err
		}
		if len(pair) != 2 {
			return nil, samples.At(row, "example requires %s and %s", in, out)
		}
		input, err := samples.YAMLValue(pair[in])
		if err != nil {
			return nil, err
		}
		output, err := samples.YAMLValue(pair[out])
		if err != nil {
			return nil, err
		}
		if _, err := samples.Canonical(input); err != nil {
			return nil, samples.At(pair[in], "%v", err)
		}
		if _, err := samples.Canonical(output); err != nil {
			return nil, samples.At(pair[out], "%v", err)
		}
		key := samples.Key(input)
		if _, ok := f.Index[key]; ok {
			return nil, samples.At(row, "duplicate input %s", key)
		}
		f.Index[key] = output
		f.Examples = append(f.Examples, samples.Pair{Input: input, Output: output})
	}
	return f, nil
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
