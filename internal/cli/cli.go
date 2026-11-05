// Package cli implements the park command line contract.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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
  run 'P.fn(args)'                  Run a call; unanswered calls go to questions.txt
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
	Out, Err io.Writer
}

func Main(args []string, out, errout io.Writer) int {
	c := &CLI{Out: out, Err: errout}
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
	if _, err := os.Lstat(filepath.Join(root, "park.html")); err == nil {
		return fail(2, "init refuses to overwrite existing park.html")
	} else if !os.IsNotExist(err) {
		return err
	}
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

// publish validates the proposed project, refreshes the question queue (adding
// any new questions and dropping answered ones), regenerates park.html, and
// writes everything.
func (c *CLI) publish(p *project.Project, edits map[string][]byte, added ...project.Question) error {
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
	queued, stale, err := project.LoadQuestions(proposed)
	if err != nil {
		return err
	}
	for _, line := range stale {
		fmt.Fprintf(c.Err, "park: dropped a question that no longer matches a function: %s\n", line)
	}
	open := project.OpenQuestions(proposed, queued, added)
	edits[project.QuestionsFile] = project.FormatQuestions(open)
	html, err := catalog.Render(proposed, open)
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
	queued, _, err := project.LoadQuestions(p)
	if err != nil {
		return err
	}
	if open := project.OpenQuestions(p, queued, nil); len(open) > 0 {
		fmt.Fprintf(c.Out, "Questions (%d): answer each with an example or an implementation\n", len(open))
		for _, q := range open {
			fmt.Fprintf(c.Out, "  %s\n", q.Call)
		}
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
	e, err := engine.New(p)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	output, err := e.Run(callCtx, qualified, input)
	cancel()
	var unresolved *engine.Unresolved
	if errors.As(err, &unresolved) {
		return c.ask(p, unresolved)
	}
	if err != nil {
		return fail(1, "%v", err)
	}
	if _, err := samples.Canonical(output); err != nil {
		return fail(1, "%v", err)
	}
	fmt.Fprintln(c.Out, samples.Literal(output))
	return nil
}

// ask adds an unanswered call to the question queue and explains how to answer it.
func (c *CLI) ask(p *project.Project, unresolved *engine.Unresolved) error {
	q, err := project.NewQuestion(p, unresolved.Function, unresolved.Input)
	if err != nil {
		return err
	}
	if err := c.publish(p, nil, q); err != nil {
		return err
	}
	return fail(3, "no answer for %s; added to %s\nAnswer it with an example:\n  park example add '%s == ...'\nor implement %s.", engine.FormatCall(unresolved.Function, unresolved.Inputs, unresolved.Input), project.QuestionsFile, q.Call, q.Function)
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
	var questions []project.Question
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
				var unresolved *engine.Unresolved
				if errors.As(err, &unresolved) {
					if q, qerr := project.NewQuestion(p, unresolved.Function, unresolved.Input); qerr == nil {
						questions = append(questions, q)
					}
				}
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
	if len(questions) > 0 {
		if err := c.publish(p, nil, questions...); err != nil {
			return err
		}
		fmt.Fprintf(c.Err, "Added %d unanswered calls to %s; see park list.\n", len(questions), project.QuestionsFile)
	}
	if failures > 0 {
		return fail(1, "tests failed")
	}
	return nil
}
