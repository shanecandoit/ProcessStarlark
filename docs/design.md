# Design decisions

A running log of Park's design decisions: what was decided, why, and what it
costs. Add new entries at the end of the relevant section. When a decision
changes, mark the old entry **Superseded** and link to the new one rather than
deleting it.

Status values: **Implemented**, **Agreed** (decided, not built yet),
**Provisional** (built or planned, but open to change), **Open** (not decided).

Longer explorations live in their own pages, such as
[ideas-function-values.md](ideas-function-values.md).

## Functions

### D1. Named inputs, one output
**Implemented** · 2026-11-05

A function takes one or more named inputs and returns one value. A function
that needs several results returns an object. Each function's examples read
like a table: one column per input, plus the output column.

- Why: rows stay flat (`{left: 9, right: 6, sum: 15}`) with no invented wrapper
  names. Canonical JSON with sorted keys makes the order of named inputs
  irrelevant to example and cache keys, so this costs the machine almost
  nothing.
- Cost: changing a function's inputs means editing its examples too. Cache
  invalidation handles the rest.
- See [ideas-function-values.md](ideas-function-values.md) for the options
  considered.

### D2. Input names match the `def`, in the same order
**Implemented** · 2026-11-05

`add(left, right)` must be implemented by `def add(left, right)`. Callers may
pass inputs by position (in that order) or by name; both are the same call.

- Why: one name per value everywhere (code, examples, catalog, CLI). A fixed
  order means positional calls mean the same thing for every function.
- Cost: renaming a parameter means renaming it in examples. Accepted.

### D3. Descriptive names, never keywords
**Implemented** · 2026-11-05

Names are identifiers that aren't Starlark keywords. `in` is therefore
impossible. Prefer names that say what the value is (`text`, `numeral`,
`tokens`) over `input`/`output`. The output name must differ from every input
name.

### D4. The output name comes from `return <name>`
**Implemented** · 2026-11-05

Every `return` in an implementation returns a plain variable, and all of a
function's returns use the same name. That name is the output's name:
`def parse(text)` ending in `return value` has the signature
`parse(text) value`. `return left + right` and a bare `return` are rejected.
The conventional default name is `result`.

- Why: the output follows the same "names match the code" rule as the inputs,
  so the signature can be read from source with nothing else to keep in sync.
- Cost: implementations need one extra assignment line.

### D5. Example-only functions are body-less `def`s
**Implemented** · 2026-11-05

A function with no implementation is declared as `def tokenize(text): pass`
(a docstring-only body also counts). Its inputs come from the parameters and
its output is named `result`. Examples answer its calls; unknown inputs are
unresolved.

## Examples

### D6. Examples outrank code
**Implemented** · v1

A call whose inputs match an authored example returns that example's output,
even when an implementation exists. `park test` checks implementations against
every example.

### D7. Two distinct examples before a body runs
**Implemented** · v1

An implementation runs only after its function has two or more distinct
authored examples. This is an authoring requirement, not a proof of correctness
or a record of passing tests. `park run` does not run `park test` or require it
to have passed. Examples remain authoritative even when the body disagrees.

### D8. One project-wide example set
**Implemented** · 2026-11-05

All examples go into one set for the whole project. An example can be written
in any file and can name any function in the project, such as
`example: RomanParse.parse("xii") == 12` inside `roman_add.star`. It counts
exactly like an example written beside the function, including toward
readiness. There are no caller-local overrides: the same call always has the
same answer.

- Convention: write examples where they make sense to a reader, usually beside
  the function or where it is used.
- Examples for functions that are never called are welcome: low friction, and
  more examples are better.
- `park example add` writes to the target function's
  own file, after its last example.
- A function's examples list those in its own module first, then those written
  elsewhere. This order decides which examples the catalog shows.

## Source format

### D9. Indented `Process Name:` blocks
**Implemented** · 2026-11-05, supersedes the brace syntax `Process Name { ... }`

```python
Process RomanParse:
    """Convert lowercase Roman numerals."""

    example: parse("ix") == 9

    def parse(text):
        ...
        return value
```

A Process is a stateless namespace: a name, a docstring, functions, and
examples. Plain `.star` files with top-level functions remain supported; their
process name comes from the file name (`roman_parse.star` becomes
`RomanParse`).

- Why: looks like Python, not braces.

### D10. Example lines use call syntax
**Implemented** · 2026-11-05

`example: parse("ix") == 9` and `example: add(left=9, right=6) == 15`. A
function in the same process can be named bare; other processes are qualified.
Arguments and expected values are Starlark literals.

### D11. No YAML sidecars
**Implemented** · 2026-11-05, supersedes the v1 YAML sidecar format

Everything about a process lives in its `.star` file. Signatures come from the
code (D2, D4), descriptions from docstrings, examples from `example:` lines,
and dependencies from references to other processes in the code.

- Cost: `park example add` edits source files rather than data
  files. The `register`, `process`, and `function` commands are no longer
  needed.

## Command line

### D12. Calls are one Starlark expression
**Implemented** · 2026-11-05

```sh
park run 'RomanParse.parse("ix")'
park run 'Math.add(right=6, left=9)'
park example add 'RomanParse.parse("xiv") == 14'
```

The call is parsed with Starlark's expression parser and accepts only
literals, so typing needs no rules of its own: `9` is an integer, `"ix"` a
string, and `None`, `True`, lists, and dicts mean what they do in Starlark.
Named arguments use `=` (not `:`), as in Python.

- Why: the same syntax as `example:` lines, and Starlark literals answer the
  "is `9` a number or a string?" question.
- Cost: strings need quotes, so the whole call goes in single quotes for the
  shell.

## Runtime

### D13. Pure and deterministic
**Implemented** · v1

No filesystem, network, clock, environment, or randomness. Step, size, and
time limits. Results are safe to cache.

### D14. Disposable, revisioned cache
**Implemented** · v1

Computed results are cached under a project-wide revision: a hash of all
sources and the codec/engine version. Any edit invalidates the whole cache,
which is always safe to delete. There is no per-function versioning yet.

### D15. No lock or recovery journal
**Implemented** · 2026-11-05, supersedes the v1 journal and project lock

Edits validate the whole proposed project first, then write the edited module
and then `park.html`, each with an atomic rename. If something fails after the
module is saved, only the catalog can be stale, and `park build` regenerates it.

- Why: an example edit changes one source file, and the catalog is derived
  from source. Atomic file replacement and rebuilding derived files are enough
  for the current local tool. Locks and recovery journals are out of scope.
- Cost: two `park` commands editing the same project at the same moment are not
  serialized; the later write wins. Acceptable for a local, single-user tool.

### D16. Unanswered calls become queued questions
**Implemented** · 2026-11-05, supersedes `park run --interactive`

When a call has no matching example and no ready implementation, Park adds it to
`questions.txt` and prints the `park example add` command that answers it. Open
questions appear at the top of `park.html` and `park list`. A question is
answered by an example for that call or by an implementation with at least two
distinct examples, and is then removed automatically. A body with fewer examples
leaves the question open, and calls blocked by that requirement are queued too.
Removing a question because a body is ready does not execute or verify the body
or its dependencies. `park test` queues dependency misses too. Errors from
executing ready implementations remain evaluation errors, not questions.

- Why: a queue is plain files, so it's simple to build and to test. Holding the
  terminal open to ask meant stdin handling, cancellation, and a reload-and-retry
  loop, and tests needed pipes and timers. The queue also collects questions
  from runs nobody is watching.
- Name: "questions", because each one waits for the user to answer it, with an
  example or with code.
- Later: questions and examples move to a database behind a REST API. The
  queue file is the local stand-in.
- Cost: answering is a second command rather than typing at a prompt.

### D17. Every example is a strict implementation check
**Implemented** · 2026-11-05

`park test` checks an implementation against every authored example, including
deliberate exceptions to a general rule. There is no exception annotation or
automatic exemption. A matching example still answers a runtime call even if
the body disagrees; the disagreement fails the implementation check.

- Why: an audit should expose disagreements between examples and code. To pass
  the tests, the implementation must handle exceptional cases too.
- Cost: adding an intentional runtime override can make `park test` fail until
  the implementation is updated. Accepted for the current version.

## Resolved questions

- **Deliberate exceptions — resolved by [D17](#d17-every-example-is-a-strict-implementation-check).**
  Keep strict checks; no exception annotation in the current version.
- **Purpose of the two-example rule — clarified in [D7](#d7-two-distinct-examples-before-a-body-runs).**
  Keep it as an authoring requirement; verification is a separate command.

## Open questions

- **Audience.** The README addresses non-programmers; the docs and CLI assume
  programmers. Pick one.
- **Examples for undeclared functions.** Currently an example must name a
  declared function (with or without a body), and Park reports an error
  otherwise; declaring one is a one-line `def f(x): pass`. Should examples be
  allowed to name functions that don't exist yet?
- **Per-function versions.** Version functions by a hash of their bodies, so
  that editing one function doesn't invalidate every cached result?
