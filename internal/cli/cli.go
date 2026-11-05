// Package cli implements the park command line contract.
package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
	"park/internal/catalog"
	"park/internal/engine"
	"park/internal/project"
	"park/internal/samples"
)

const usage = `park — example-driven Starlark functions

Usage: park [--root PATH] COMMAND [FLAGS] [OPERANDS]

  init [DIR]                    Create an empty project and catalog
  register FILE.star            Generate/refresh one YAML sidecar
  process add|remove            Register or remove a process
  function add|remove           Register or remove an exported function
  example add|remove            Add or remove a named input/output pair
  list                          List processes and function readiness
  run --input JSON P.fn         Run a function (optional --interactive)
  test [P.fn]                   Check authored outputs against bodies
  build                         Validate and regenerate park.html

Use park COMMAND --help for flags. Flags precede positional operands.
`

type failure struct {
	code int
	err  error
}

func (f *failure) Error() string { return f.err.Error() }
func fail(code int, format string, args ...any) error {
	return &failure{code, fmt.Errorf(format, args...)}
}

type CLI struct {
	In       io.Reader
	Out, Err io.Writer
	// Terminal permits embedding and lets tests exercise prompt behavior explicitly.
	Terminal bool
}

func Main(args []string, in io.Reader, out, errout io.Writer) int {
	terminal := false
	if file, ok := in.(*os.File); ok {
		terminal = term.IsTerminal(int(file.Fd()))
	}
	c := &CLI{In: in, Out: out, Err: errout, Terminal: terminal}
	return c.Run(context.Background(), args)
}
func (c *CLI) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.Err)
	return fs
}
func (c *CLI) Run(ctx context.Context, args []string) int {
	err := c.execute(ctx, args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(c.Err, "park:", err)
		var f *failure
		if errors.As(err, &f) {
			return f.code
		}
		var unresolved *engine.Unresolved
		if errors.As(err, &unresolved) {
			return 3
		}
		return 2
	}
	return 0
}
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fail(2, "%v", err)
	}
	return nil
}
func exact(fs *flag.FlagSet, n int) error {
	if fs.NArg() != n {
		return fail(2, "%s: expected %d positional operands, got %d", fs.Name(), n, fs.NArg())
	}
	return nil
}
func supplied(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
func value(fs *flag.FlagSet, name, raw string) (any, error) {
	if !supplied(fs, name) {
		return nil, fail(2, "missing --%s JSON", name)
	}
	v, err := samples.ParseJSON([]byte(raw))
	if err != nil {
		return nil, fail(2, "--%s: %v", name, err)
	}
	return v, nil
}
func (c *CLI) execute(ctx context.Context, args []string) error {
	global := c.flags("park")
	rootFlag := global.String("root", "", "Project root (defaults to current directory)")
	global.Usage = func() { fmt.Fprint(c.Err, usage); global.PrintDefaults() }
	if err := parse(global, args); err != nil {
		return err
	}
	args = global.Args()
	if len(args) == 0 {
		return fail(2, "%s", usage)
	}
	command := args[0]
	args = args[1:]
	if command == "help" {
		fmt.Fprint(c.Out, usage)
		return nil
	}
	if command == "init" {
		return c.init(args, *rootFlag)
	}
	switch command {
	case "list", "run", "test", "build", "register", "process", "function", "example":
	default:
		return fail(2, "unknown command %q", command)
	}
	// Command help must work before discovery or validation of project files.
	helpRequested := len(args) > 0 && (args[0] == "--help" || args[0] == "-h")
	nestedHelp := len(args) > 1 && (args[1] == "--help" || args[1] == "-h")
	if helpRequested || nestedHelp {
		switch command {
		case "list":
			return c.list(nil, args)
		case "run":
			return c.run(ctx, nil, args)
		case "test":
			return c.test(ctx, nil, args)
		case "register":
			return c.register(nil, args)
		case "build":
			return parse(c.flags("build"), args)
		case "process", "function", "example":
			if helpRequested {
				fmt.Fprintf(c.Out, "Usage: park %s add|remove [FLAGS] [OPERANDS]\nUse park %s add --help or park %s remove --help.\n", command, command, command)
				return nil
			}
			switch command {
			case "process":
				return c.process(nil, args)
			case "function":
				return c.function(nil, args)
			case "example":
				return c.example(nil, args)
			}
		}
	}
	var root string
	var err error
	if *rootFlag != "" {
		root, err = project.Root(*rootFlag)
	} else {
		cwd, e := os.Getwd()
		if e != nil {
			return e
		}
		root, err = project.Discover(cwd)
		if err != nil && command == "register" {
			root, err = project.Root(cwd)
		}
	}
	if err != nil {
		return err
	}
	mutating := command == "build" || command == "register" || command == "process" || command == "function" || command == "example"
	// Interactive runs may persist an answer and therefore hold the mutation lock.
	exclusive := mutating
	if command == "run" {
		for _, arg := range args {
			if arg == "--interactive" || arg == "--interactive=true" {
				exclusive = true
			}
		}
	}
	unlock, err := project.Lock(root, exclusive)
	if err != nil {
		return err
	}
	defer unlock()
	if mutating {
		if err := project.Recover(root); err != nil {
			return err
		}
	} else {
		if err := project.Pending(root); err != nil {
			return err
		}
	}
	p, err := project.LoadWith(root, nil)
	if err != nil {
		return err
	}
	switch command {
	case "build":
		fs := c.flags("build")
		if err := parse(fs, args); err != nil {
			return err
		}
		if err := exact(fs, 0); err != nil {
			return err
		}
		if err := c.publish(p, nil); err != nil {
			return err
		}
		fmt.Fprintln(c.Out, "Built park.html")
		return nil
	case "register":
		return c.register(p, args)
	case "list":
		return c.list(p, args)
	case "process":
		return c.process(p, args)
	case "function":
		return c.function(p, args)
	case "example":
		return c.example(p, args)
	case "run":
		return c.run(ctx, p, args)
	case "test":
		return c.test(ctx, p, args)
	}
	return nil
}

func (c *CLI) init(args []string, rootFlag string) error {
	fs := c.flags("init")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fail(2, "init accepts at most one directory")
	}
	target := "."
	if rootFlag != "" {
		target = rootFlag
	}
	if fs.NArg() == 1 {
		if rootFlag != "" {
			return fail(2, "use either --root or an init directory")
		}
		target = fs.Arg(0)
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return err
	}
	root, err := project.Root(target)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml") {
			return fail(2, "init refuses to overwrite existing sidecar %s", entry.Name())
		}
	}
	for _, name := range []string{"park.html", ".park"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return fail(2, "init refuses to overwrite existing %s", name)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	unlock, err := project.Lock(root, true)
	if err != nil {
		return err
	}
	defer unlock()
	p, err := project.LoadWith(root, nil)
	if err != nil {
		return err
	}
	if err := c.publish(p, nil); err != nil {
		return err
	}
	fmt.Fprintf(c.Out, "Initialized %s\n", root)
	return nil
}
func (c *CLI) publish(p *project.Project, edits map[string][]byte) error {
	if edits == nil {
		edits = map[string][]byte{}
	}
	proposed, err := project.LoadWith(p.Root, edits)
	if err != nil {
		return err
	}
	if _, err := engine.New(proposed); err != nil {
		return err
	}
	html, err := catalog.Render(proposed)
	if err != nil {
		return err
	}
	edits["park.html"] = html
	return project.Publish(p.Root, edits)
}
func (c *CLI) list(p *project.Project, args []string) error {
	fs := c.flags("list")
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := exact(fs, 0); err != nil {
		return err
	}
	if _, err := engine.New(p); err != nil {
		return err
	}
	for _, name := range project.Names(p.Processes) {
		proc := p.Processes[name]
		fmt.Fprintln(c.Out, name)
		for _, name := range project.Names(proc.Functions) {
			f := proc.Functions[name]
			fmt.Fprintf(c.Out, "  %s.%s  %s  %d examples\n", proc.Name, name, f.State(), len(f.Examples))
		}
	}
	return nil
}
func (c *CLI) process(p *project.Project, args []string) error {
	if len(args) == 0 {
		return fail(2, "process requires add or remove")
	}
	fs := c.flags("process " + args[0])
	switch args[0] {
	case "add":
		name := fs.String("name", "", "Process name")
		module := fs.String("module", "", "Project-relative .star source path")
		uses := fs.String("uses", "", "Comma-separated dependency names")
		description := fs.String("description", "", "Process description")
		if err := parse(fs, args[1:]); err != nil {
			return err
		}
		if err := exact(fs, 0); err != nil {
			return err
		}
		if !project.ValidName(*name) {
			return fail(2, "invalid process name %q", *name)
		}
		if p.Processes[*name] != nil {
			return fail(2, "process already exists: %s", *name)
		}
		rel := *name + ".yaml"
		if _, ok := p.Files[rel]; ok {
			return fail(2, "manifest path already exists: %s", rel)
		}
		doc := samples.ValueNode(map[string]any{})
		project.SetField(doc, "version", samples.ValueNode(1))
		project.SetField(doc, "name", samples.ValueNode(*name))
		if *description != "" {
			project.SetField(doc, "description", samples.ValueNode(*description))
		}
		project.SetField(doc, "uses", samples.ValueNode([]string{}))
		project.SetField(doc, "functions", samples.ValueNode([]any{}))
		if *module != "" {
			if filepath.IsAbs(*module) {
				return fail(2, "--module must be project-relative")
			}
			path, err := project.SafePath(p.Root, *module)
			if err != nil {
				return err
			}
			normalized, err := filepath.Rel(p.Root, path)
			if err != nil {
				return err
			}
			project.SetField(doc, "module", samples.ValueNode(normalized))
			rel = strings.TrimSuffix(normalized, ".star") + ".yaml"
		}
		if _, exists := p.Files[rel]; exists {
			return fail(2, "sidecar already exists: %s", rel)
		}
		if *uses != "" {
			project.SetField(doc, "uses", samples.ValueNode(strings.Split(*uses, ",")))
		}
		b, err := samples.EncodeYAML(doc)
		if err != nil {
			return err
		}
		if err := c.publish(p, map[string][]byte{rel: b}); err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Added process %s\n", *name)
		return nil
	case "remove":
		if err := parse(fs, args[1:]); err != nil {
			return err
		}
		if err := exact(fs, 1); err != nil {
			return err
		}
		name := fs.Arg(0)
		proc := p.Processes[name]
		if proc == nil {
			return fail(2, "unknown process %s", name)
		}
		for _, other := range p.Processes {
			for _, dep := range other.Uses {
				if dep == name {
					return fail(2, "%s is used by %s", name, other.Name)
				}
			}
		}
		edits := map[string][]byte{proc.ManifestPath: nil}
		if err := c.publish(p, edits); err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Removed process %s\n", name)
		return nil
	default:
		return fail(2, "unknown process operation %q", args[0])
	}
}
func (c *CLI) function(p *project.Project, args []string) error {
	if len(args) == 0 {
		return fail(2, "function requires add or remove")
	}
	fs := c.flags("function " + args[0])
	switch args[0] {
	case "add":
		signature := fs.String("signature", "", "Signature: name(input) output")
		implementation := fs.String("implementation", "", "Starlark implementation name")
		description := fs.String("description", "", "Function description")
		if err := parse(fs, args[1:]); err != nil {
			return err
		}
		if err := exact(fs, 1); err != nil {
			return err
		}
		parts := strings.Split(fs.Arg(0), ".")
		if len(parts) != 2 {
			return fail(2, "expected Process.function")
		}
		proc := p.Processes[parts[0]]
		if proc == nil {
			return fail(2, "unknown process %s", parts[0])
		}
		name, _, _, err := project.Signature(*signature)
		if err != nil {
			return err
		}
		if name != parts[1] {
			return fail(2, "signature does not match export %s", parts[1])
		}
		if proc.Functions[name] != nil {
			return fail(2, "function already exists: %s", fs.Arg(0))
		}
		functions := project.Field(proc.Document, "functions")
		functions.Content = append(functions.Content, project.FunctionNode(*signature, *description, *implementation))
		manifest, err := samples.EncodeYAML(proc.Document)
		if err != nil {
			return err
		}
		if err := c.publish(p, map[string][]byte{proc.ManifestPath: manifest}); err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Added function %s\n", fs.Arg(0))
		return nil
	case "remove":
		if err := parse(fs, args[1:]); err != nil {
			return err
		}
		if err := exact(fs, 1); err != nil {
			return err
		}
		proc, f, err := p.Function(fs.Arg(0))
		if err != nil {
			return err
		}
		functions := project.Field(proc.Document, "functions")
		for i, n := range functions.Content {
			if n == f.Node {
				functions.Content = append(functions.Content[:i], functions.Content[i+1:]...)
				break
			}
		}
		b, err := samples.EncodeYAML(proc.Document)
		if err != nil {
			return err
		}
		if err := c.publish(p, map[string][]byte{proc.ManifestPath: b}); err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Removed function %s\n", fs.Arg(0))
		return nil
	default:
		return fail(2, "unknown function operation %q", args[0])
	}
}
func (c *CLI) example(p *project.Project, args []string) error {
	if len(args) == 0 {
		return fail(2, "example requires add or remove")
	}
	op := args[0]
	if op != "add" && op != "remove" {
		return fail(2, "unknown example operation %q", op)
	}
	fs := c.flags("example " + op)
	in := fs.String("input", "", "JSON input")
	var out *string
	if op == "add" {
		out = fs.String("output", "", "JSON output")
	}
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	if err := exact(fs, 1); err != nil {
		return err
	}
	input, err := value(fs, "input", *in)
	if err != nil {
		return err
	}
	_, f, err := p.Function(fs.Arg(0))
	if err != nil {
		return err
	}
	var b []byte
	if op == "add" {
		output, e := value(fs, "output", *out)
		if e != nil {
			return e
		}
		b, err = project.AddExample(f, input, output)
	} else {
		b, err = project.RemoveExample(f, input)
	}
	if err != nil {
		return err
	}
	if err := c.publish(p, map[string][]byte{f.ExamplePath: b}); err != nil {
		return err
	}
	fmt.Fprintf(c.Out, "%s example for %s\n", map[string]string{"add": "Added", "remove": "Removed"}[op], fs.Arg(0))
	return nil
}
func (c *CLI) run(ctx context.Context, p *project.Project, args []string) error {
	fs := c.flags("run")
	raw := fs.String("input", "", "JSON input")
	interactive := fs.Bool("interactive", false, "Prompt for unresolved outputs when stdin is a terminal")
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := exact(fs, 1); err != nil {
		return err
	}
	input, err := value(fs, "input", *raw)
	if err != nil {
		return err
	}
	qualified := fs.Arg(0)
	if _, _, err := p.Function(qualified); err != nil {
		return err
	}
	scanner := bufio.NewScanner(c.In)
	scanner.Buffer(make([]byte, 4096), samples.MaxValueBytes+1)
	for {
		e, err := engine.New(p)
		if err != nil {
			return err
		}
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		output, err := e.Run(callCtx, qualified, input)
		cancel()
		if err == nil {
			b, err := samples.Canonical(output)
			if err != nil {
				return fail(1, "%v", err)
			}
			fmt.Fprintln(c.Out, string(b))
			return nil
		}
		var unresolved *engine.Unresolved
		if !errors.As(err, &unresolved) {
			return fail(1, "%v", err)
		}
		if !*interactive || !c.Terminal {
			return unresolved
		}
		_, f, err := p.Function(unresolved.Function)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Err, "%s\nEnter JSON %s (EOF cancels): ", unresolved.Error(), f.OutputLabel)
		type lineResult struct {
			bytes []byte
			ok    bool
			err   error
		}
		line := make(chan lineResult, 1)
		go func() {
			ok := scanner.Scan()
			line <- lineResult{bytes: append([]byte(nil), scanner.Bytes()...), ok: ok, err: scanner.Err()}
		}()
		var answer lineResult
		select {
		case <-ctx.Done():
			return fail(1, "%v", ctx.Err())
		case answer = <-line:
		}
		if !answer.ok {
			if answer.err != nil {
				return fail(1, "prompt: %v", answer.err)
			}
			return unresolved
		}
		output, err = samples.ParseJSON(answer.bytes)
		if err != nil {
			fmt.Fprintln(c.Err, "Invalid output:", err)
			continue
		}
		if err := ctx.Err(); err != nil {
			return fail(1, "%v", err)
		}
		b, err := project.AddExample(f, unresolved.Input, output)
		if err != nil {
			return err
		}
		if err := c.publish(p, map[string][]byte{f.ExamplePath: b}); err != nil {
			return err
		}
		p, err = project.LoadWith(p.Root, nil)
		if err != nil {
			return err
		}
	}
}
func (c *CLI) test(ctx context.Context, p *project.Project, args []string) error {
	fs := c.flags("test")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fail(2, "test accepts at most one Process.function")
	}
	if fs.NArg() == 1 {
		if _, _, err := p.Function(fs.Arg(0)); err != nil {
			return err
		}
	}
	e, err := engine.New(p)
	if err != nil {
		return err
	}
	e.Testing = true
	checks, records, failures := 0, 0, 0
	for _, name := range project.Names(p.Processes) {
		proc := p.Processes[name]
		for _, fn := range project.Names(proc.Functions) {
			qualified := name + "." + fn
			if fs.NArg() == 1 && fs.Arg(0) != qualified {
				continue
			}
			f := proc.Functions[fn]
			if f.Implementation == "" {
				records += len(f.Examples)
				fmt.Fprintf(c.Out, "DATA %s: %d example-only records\n", qualified, len(f.Examples))
				continue
			}
			if len(f.Examples) < 2 {
				failures++
				fmt.Fprintf(c.Err, "FAIL %s: needs examples\n", qualified)
				continue
			}
			e.Bypass = qualified
			for _, pair := range f.Examples {
				checks++
				callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				output, err := e.Run(callCtx, qualified, pair.Input)
				cancel()
				if err != nil {
					failures++
					fmt.Fprintf(c.Err, "FAIL %s(%s): %v\n", qualified, samples.Key(pair.Input), err)
				} else if samples.Key(output) != samples.Key(pair.Output) {
					failures++
					fmt.Fprintf(c.Err, "FAIL %s(%s): got %s; expected %s\n", qualified, samples.Key(pair.Input), samples.Key(output), samples.Key(pair.Output))
				}
			}
		}
	}
	fmt.Fprintf(c.Out, "%d implementation checks, %d example-only records, %d failures\n", checks, records, failures)
	if failures > 0 {
		return fail(1, "tests failed")
	}
	return nil
}

func (c *CLI) register(p *project.Project, args []string) error {
	fs := c.flags("register")
	name := fs.String("name", "", "Process name (default: module basename in PascalCase)")
	uses := fs.String("uses", "", "Additional comma-separated dependencies")
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := exact(fs, 1); err != nil {
		return err
	}
	var deps []string
	if *uses != "" {
		deps = strings.Split(*uses, ",")
	}
	path, b, err := project.Generate(p, fs.Arg(0), *name, deps)
	if err != nil {
		return err
	}
	if err := c.publish(p, map[string][]byte{path: b}); err != nil {
		return err
	}
	fmt.Fprintf(c.Out, "Registered %s\n", path)
	return nil
}
