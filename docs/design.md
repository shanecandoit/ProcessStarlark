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
**Superseded** by [D18](#d18-code-without-two-examples-runs-unconfirmed) · v1

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

The cache sits behind the engine's `Cache` interface. The CLI uses the disk
cache in `.cache/`; projects loaded from memory (the browser playground) run
with no cache. See D19.

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

When a call has no matching example and no implementation, Park adds it to
`questions.txt` and prints the `park example add` command that answers it. Open
questions appear at the top of `park.html` and `park list`. A question is
answered by an example for that call, or by its function becoming ready (two
examples), and is then removed automatically. `park test` queues dependency
misses too. Errors from running implementations remain evaluation errors, not
questions. Unconfirmed answers share this queue as proposals; see D18.

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

### D18. Code without two examples runs, unconfirmed
**Implemented** · 2026-11-05, supersedes D7

A function is *ready* once it has two distinct examples. Before that, its code
still runs, but each answer is unconfirmed: `park run` prints it, notes that
it's unconfirmed, and queues it in `questions.txt` as a proposal,
`Words.shout("hi") == "HI!"`. Confirming one is `park example add` with that
whole line. A newer answer for the same call replaces an older proposal.
Unconfirmed results are never cached, and nor is any result in the same call
that depended on one. Proposals are removed once confirmed or once the
function is ready.

- Why: people who write code first should end up with examples without
  writing them by hand. Proposals give them examples to confirm rather than
  invent.
- Why not record outputs straight into examples: an example recorded from the
  code checks nothing (it always passes `park test`), and because examples
  outrank code, a recorded wrong answer would silently override a later fix. A
  person confirming each answer is what turns it into a real example.
- Two examples are still required: the rule now means "two answers a person
  has checked" rather than "the code won't run".
- Cost: an unready function's callers get unconfirmed answers. The CLI says so
  on every such run.

### D20. `park run --why` shows where an answer came from
**Implemented** · 2026-11-06

```text
$ park run --why 'Shipping.quote("FR", 800)'
2799
  Shipping.quote(country="FR", grams=800) = 2799  code     shipping.star:8
    Zones.zone(country="FR") = "international"    example  zones.star:7
```

With `--why`, the engine records every dispatched call as a tree: the call,
its answer, and its source, with a location. The sources are `example` (where
the example is written), `code` (the function's `def`), `unconfirmed code`
(code with fewer than two examples, D18), and `unknown` (it becomes a
question, D16). A call that fails shows `?`, and only the innermost failure
carries the error text. Starlark has no `try`, so a failing callee always fails
its caller. The tree goes to stdout after the answer, or to stderr before the
question or error.

- Why: examples outrank code (D6), so a wrong example wins silently. The tree
  makes every winning example visible, with a place to go and fix it.
- **`--why` doesn't read the cache.** It recomputes, so every answer's tree is
  complete. A cache hit would be a leaf with no subtree. Results are pure
  (D13), so the answer is the same; only the time differs. Cache writes still
  happen.
- Cost: none without the flag; dispatch skips recording entirely.
- The playground always records the tree and shows it under each answer
  ("Why this answer"), where each location opens the file at that line.

### D22. Resolvers answer misses, only as proposals
**Implemented (engine only)** · 2026-11-06

`engine.Resolver` is a hook for answers from outside the project: a model,
a default, an external program. When a call has no example, no cached result,
and no code, the engine offers it to each of `Engine.Resolvers` in turn. A
resolver returns an answer, `ErrUnknown` to pass, or an error that fails the
run. If none answers, the call is unresolved as before (D16).

- **A resolver's answer is a proposal** (D18): returned for the run, queued
  for a person to confirm, never cached, and never counted as an example.
  Nothing that used it is cached either.
- `park test` never asks resolvers, so a missing answer still fails a test.
- `--why` shows the source as `proposed by <name>`.
- Answers are normalized through canonical JSON, so a resolver returns
  ordinary Park values or fails clearly.
- Why: without it, a run stops at the first miss, and a deep call chain takes
  one ask-answer-rerun round per miss. With a resolver the run finishes, and
  every guess shows up together for review. It also keeps models out of the
  core (see [Direction](ideas-direction.md#left-open-on-purpose-synthesis)).
- Not built yet: any resolver, or a CLI flag to plug one in (for example, an
  external command that reads a call as JSON and writes an answer).

## Playground

### D19. The browser runs the same engine, on files in memory
**Implemented** · 2026-11-06

The playground compiles the Go packages to WebAssembly rather than
reimplementing Park in JavaScript. The page owns the project as a map from file
name to text; every operation (`internal/workspace`) takes the whole map and
returns the new map plus a snapshot. Go keeps no state between calls.

- Why: one implementation means one semantics. A test checks that the CLI
  and the workspace write identical `questions.txt` and `park.html` for the
  same commands, and that a downloaded project opens in the CLI.
- How: project loading, the result cache, and the question queue each got an
  in-memory seam (`project.FromFiles`, `engine.Cache`,
  `project.ParseQuestions`), and `park test` moved into `engine.Check`.
- Cost: a 2.4 MB (gzipped) download, and no cache in the browser.

### D21. Tables are a view of example lines
**Implemented** · 2026-11-06

The playground shows each function as a table (inputs, then the output) and
lets people edit it like a spreadsheet: type a row, click a row to change it,
or paste rows copied from Excel or Sheets. Every edit becomes an ordinary
`example:` line in the function's own module. A new row is added the way
`park example add` adds it, and an edited row is rewritten in place. The
`.star` file stays the only source of truth.

Cells are typed as plain text and take their column's type. In a column
whose values are all strings, `US` means `"US"`, and `02134` stays a string.
In a column of whole numbers, text is rejected. In a new or mixed column, a
cell is read as a Starlark literal if it is one, and as a string if not.
Quotes keep a string exact, as in `" Germany "`.

- Why: rule owners should never have to write Starlark syntax. The type of a
  value comes from the examples already in the table, which is what a
  spreadsheet user expects.
- A paste is checked as a whole before anything is written. Rows already in
  the table are skipped. Rows that would change an existing answer are listed
  and left alone, because a paste shouldn't silently overwrite a decision.
- After an edit, if the function has code, the code is run against its
  examples, and any row it answers differently is noted under the table. The
  row still stands (D6), and `park test` will flag it (D17).
- Cost: in an empty or mixed column, `True`, `None`, and numbers are read as
  values, not text. Quote them to keep them as text.

### D23. `park import` proposes a table's rows
**Implemented** · 2026-11-06

`park import [--add] P.fn FILE` reads a CSV or TSV file whose first row names
the function's inputs and output, in any order. Cells are typed as in table
edits (D21). The whole file must read cleanly before anything is written.

- Rows matching an example are skipped. Rows contradicting an example, or
  answered two ways in the file, are listed and never imported.
- If the function has code, each new row is run against it (without the cache
  or resolvers) and agreements and disagreements are reported.
- **By default new rows become proposals** in `questions.txt`, and confirming
  one is `park example add`. Imported data from logs or other systems is
  untrusted until a person looks at it, the same rule as D18. `--add` writes
  them as examples instead, for a table the team already owns.
- A ready function's code answers every call, so its proposals would leave the
  queue at once (D16). Import queues nothing for it and reports only the
  disagreements; keeping the file's answers there takes `--add` or a code fix.
- Cost: proposals for code with fewer than two examples leave the queue once it
  gets its second example, including ones the code disagrees with. Import
  says so.

## Resolved questions

- **Audience — resolved 2026-11-06.** The idea is explained for business
  readers (users who'd write examples, and buyers); the technical docs and CLI
  are for programmers. The aim is a B2B product. See
  [Direction](ideas-direction.md#positioning) and
  [Park for business teams](for-business.md).

- **Deliberate exceptions — resolved by [D17](#d17-every-example-is-a-strict-implementation-check).**
  Keep strict checks; no exception annotation in the current version.
- **Purpose of the two-example rule — revised in [D18](#d18-code-without-two-examples-runs-unconfirmed).**
  Keep requiring two examples; code runs before then, but its answers are
  unconfirmed proposals until a person confirms them.

## Open questions

- **Examples for undeclared functions.** Currently an example must name a
  declared function (with or without a body), and Park reports an error
  otherwise; declaring one is a one-line `def f(x): pass`. Should examples be
  allowed to name functions that don't exist yet?
- **Per-function versions.** Version functions by a hash of their bodies, so
  that editing one function doesn't invalidate every cached result? Explored
  as Unison-style content hashing in
  [Direction](ideas-direction.md#3-per-function-content-hashing-exploring-unison-ideas).
