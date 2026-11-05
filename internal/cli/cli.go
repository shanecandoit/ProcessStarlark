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
	"time"

	"golang.org/x/term"
	"park/internal/catalog"
	"park/internal/engine"
	"park/internal/project"
	"park/internal/samples"
	"park/internal/source"
)

const usage = `park — example-driven Starlark functions

Usage: park [--root PATH] COMMAND [FLAGS] [OPERANDS]

  init [DIR]                        Create an empty project and catalog
  list                              List processes and function readiness
  run 'P.fn(args)'                  Run a call (optional --interactive)
  example add 'P.fn(args) == value' Add an example to the function's module
  example remove 'P.fn(args)'       Remove an example wherever it is written
  test [P.fn]                       Check examples against implementations
  build                             Validate and regenerate park.html

Calls use Starlark literals: 'Math.add(9, right=6)', 'RomanParse.parse("ix")'.
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
	case "list", "run", "test", "build", "example":
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
		case "build":
			return parse(c.flags("build"), args)
		case "example":
			if helpRequested {
				fmt.Fprintln(c.Out, "Usage: park example add 'P.fn(args) == value' | park example remove 'P.fn(args)'")
				return nil
			}
			return c.example(nil, args)
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
	}
	if err != nil {
		return err
	}
	mutating := command == "build" || command == "example"
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
	case "list":
		return c.list(p, args)
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

// call parses a call operand and binds it to the named function.
func call(p *project.Project, text string) (string, map[string]any, error) {
	parsed, err := source.ParseCall(text)
	if err != nil {
		return "", nil, fail(2, "%v", err)
	}
	return bind(p, parsed)
}

func bind(p *project.Project, parsed *source.Call) (string, map[string]any, error) {
	if parsed.Process == "" {
		return "", nil, fail(2, "name the process too, e.g. Process.%s(...)", parsed.Function)
	}
	qualified := parsed.Process + "." + parsed.Function
	_, f, err := p.Function(qualified)
	if err != nil {
		return "", nil, fail(2, "%v", err)
	}
	input, err := f.BindArgs(qualified, parsed.Args, parsed.Names, parsed.Values)
	if err != nil {
		return "", nil, fail(2, "%v", err)
	}
	return qualified, input, nil
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
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	if err := exact(fs, 1); err != nil {
		return err
	}
	var path string
	var b []byte
	var err error
	if op == "add" {
		parsed, want, perr := source.ParseExample(fs.Arg(0))
		if perr != nil {
			return fail(2, "%v", perr)
		}
		qualified, input, berr := bind(p, parsed)
		if berr != nil {
			return berr
		}
		path, b, err = project.AddExample(p, qualified, input, want)
	} else {
		qualified, input, cerr := call(p, fs.Arg(0))
		if cerr != nil {
			return cerr
		}
		path, b, err = project.RemoveExample(p, qualified, input)
	}
	if err != nil {
		return err
	}
	if err := c.publish(p, map[string][]byte{path: b}); err != nil {
		return err
	}
	fmt.Fprintf(c.Out, "%s example in %s\n", map[string]string{"add": "Added", "remove": "Removed"}[op], path)
	return nil
}
func (c *CLI) run(ctx context.Context, p *project.Project, args []string) error {
	fs := c.flags("run")
	interactive := fs.Bool("interactive", false, "Prompt for unresolved outputs when stdin is a terminal")
	if err := parse(fs, args); err != nil {
		return err
	}
	if err := exact(fs, 1); err != nil {
		return err
	}
	qualified, input, err := call(p, fs.Arg(0))
	if err != nil {
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
			if _, err := samples.Canonical(output); err != nil {
				return fail(1, "%v", err)
			}
			fmt.Fprintln(c.Out, samples.Literal(output))
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
		fmt.Fprintf(c.Err, "%s\nEnter %s as a literal, e.g. \"text\", 9, or [\"a\"] (EOF cancels): ", unresolved.Error(), f.Output)
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
		output, err = source.ParseValue(string(answer.bytes))
		if err != nil {
			fmt.Fprintln(c.Err, "Invalid output:", err)
			continue
		}
		if err := ctx.Err(); err != nil {
			return fail(1, "%v", err)
		}
		path, b, err := project.AddExample(p, unresolved.Function, unresolved.Input, output)
		if err != nil {
			return err
		}
		if err := c.publish(p, map[string][]byte{path: b}); err != nil {
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
			if !f.Implemented {
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
					fmt.Fprintf(c.Err, "FAIL %s (%s:%d): %v\n", engine.FormatCall(qualified, f.Inputs, pair.Input), pair.Path, pair.Line, err)
				} else if samples.Key(output) != samples.Key(pair.Output) {
					failures++
					fmt.Fprintf(c.Err, "FAIL %s (%s:%d): got %s; expected %s\n", engine.FormatCall(qualified, f.Inputs, pair.Input), pair.Path, pair.Line, samples.Literal(output), samples.Literal(pair.Output))
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
