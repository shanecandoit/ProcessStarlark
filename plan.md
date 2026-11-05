# Park CLI implementation plan

## Outcome and scope

Build a local Go CLI named `park` that runs pure Starlark functions, keeps a
project-wide set of authored examples, and maintains an offline `park.html`
catalog. `docs/reference.md` documents the format for users and
`docs/design.md` records the decisions and their reasons; this document
defines the implementation contract:

- Use `go.starlark.net` for function bodies inside one stateless, indented
  `Process Name:` namespace per `.star` file. A small source adapter handles the
  namespace declaration and `example:` lines; regular top-level Starlark
  modules remain supported.
- Keep everything about a process in its `.star` file. Signatures come from
  code: inputs from `def` parameters, the output from the variable every
  `return` returns. Descriptions come from docstrings, dependencies from
  references to other processes, and examples from `example:` lines. There are
  no sidecar files.
- Treat all examples as one project-wide set. An example may be written in any
  module and name any declared function.
- Store computed results in `.cache/` as disposable, revisioned canonical JSON
  records.
- Require two distinct authored examples before executing an implementation
  body. Examples count toward the function they name, wherever they are written.
- Regenerate `park.html` after CLI changes to examples, and on `park build`.
  Include the first two and last two examples for each function, without
  duplicates.
- Queue each unresolved call in `questions.txt` as a question for the user,
  answered by an exact example or an implementation with at least two distinct
  examples. Show open
  questions at the top of `park.html` and in `park list`. Questions are local
  files for now; a later version moves them, with examples, to a database
  behind a REST API. Synthesis, nested processes, and remote services are
  outside this version.

The CLI is implemented in `cmd/park`. Keep the README's status and quick start,
and `docs/reference.md`'s usage details, current. The acceptance checks below
describe the expected behavior and should remain covered as the tool evolves.

## Project format

The repository contains `cmd/park` and internal `cli`, `project`, `source`,
`engine`, `samples`, and `catalog` packages. Ship a copyable flat project in
`fixtures/roman/`:

```text
roman_parse.star
roman_add.star
.cache/             # Disposable computed results, canonical JSON
park.html           # Generated offline catalog
```

All modules live directly in the project root. `roman_parse.star`:

```python
Process RomanParse:
    """Convert lowercase Roman numerals using i, v, and x."""

    example: parse("ix") == 9
    example: parse("vi") == 6
    example: tokenize("ix + vi") == ["ix", "+", "vi"]
    example: tokenize("vi + ix") == ["vi", "+", "ix"]

    def parse(text):
        """Parse a numeral; this small fixture does not validate Roman grammar."""
        digits = {"i": 1, "v": 5, "x": 10}
        value = 0
        previous = 0
        for index in range(len(text) - 1, -1, -1):
            digit = digits[text[index]]
            value += -digit if digit < previous else digit
            previous = digit
        return value

    def tokenize(text):
        """An example-only token vocabulary for teaching calls."""
        pass
```

The Roman fixture handles only lowercase `i`, `v`, and `x` and does not
validate Roman grammar. `roman_add.star` also carries
`example: RomanParse.parse("xii") == 12`, which counts toward `RomanParse.parse`.

### Modules and Process blocks

- `Process Name:` sits at column 1 on the first significant line, and the rest
  of the file is its indented body. The body permits a docstring, `example:`
  lines, and function definitions; no shared state or nested Process blocks.
- A module without `Process` is a regular Starlark module. Its process name
  comes from its file name (`roman_parse.star` is `RomanParse`). It may also
  define literal constants.
- The adapter rewrites `example:` to a reserved assignment and the header to a
  temporary `def`, parses with Starlark, lifts the body to module scope, and
  removes example statements. Line numbers are preserved. An `example:` line
  inside a function or a string is an error.
- Two modules declaring the same process name are an error.

### Signatures

- Public functions (names not starting with `_`) are exported.
- Inputs are the parameters, in order: plain identifiers, at least one, with no
  defaults, `*args`, or `**kwargs`. Names are identifiers and not keywords.
- An implemented function's every `return` is `return <name>` with one shared
  name, which is the output name. A bare `return`, a returned expression, two
  different names, or no `return` at all is an error. Returns inside nested
  `def`s and lambdas are ignored.
- A body of only `pass` and/or a docstring declares an example-only function,
  whose output is named `result`.
- The output name must differ from every input name. The signature is written
  `name(input, ...) output`.

### Examples

- `example: call == value`, where the call is `f(...)` for a function in the
  same process or `Process.f(...)` for any other, and all arguments and the
  value are Starlark literals: None, booleans, signed 64-bit integers, strings,
  lists, tuples (stored as lists), and dicts with string keys.
- Positional arguments bind in signature order; named arguments bind by name.
  Missing, unknown, and repeated inputs are errors.
- An example must name a declared function. Duplicate calls are errors across
  the whole project, reported with both locations.
- A function's examples are ordered with those in its own module first, then
  those written in other modules, each in file and line order.

### Discovery and validation

Use only the current directory, checking for `.star` modules or `park.html` without
searching parent directories. `--root PATH` explicitly selects the project
root. `park init [DIR]` creates an empty catalog and internal project state
without overwriting generated project state. `park build` validates the
project and regenerates HTML.

Validate module syntax, Process structure, signatures, example calls, duplicate
processes, functions, and examples, references to unknown functions, and
dependency cycles. Restrict process and function names to safe identifiers.
Keep files inside the root and reject symlinked modules.

## CLI contract

Use Go's standard `flag` package. Global `--root PATH` precedes the command;
command flags precede positional operands. Calls are one Starlark expression
with literal arguments, parsed by Starlark's expression parser. The CLI never
reads stdin. Errors go to stderr; a successful run prints one output as a Starlark literal.

- `park init [DIR]`: create an empty project and catalog.
- `park list`: list processes, functions, readiness, and example counts.
- `park run 'RomanParse.parse("ix")'`: look up or compute one output. If the
  call is unresolved, add a question to `questions.txt`, regenerate
  `park.html`, print the `park example add` command that answers it, and exit 3.
- `park example add 'RomanParse.parse("xiv") == 14'`: add an example line to
  the target function's own module, after that module's last example (else
  after the process docstring, the Process header, or at the top of the file).
  Examples are written positionally in signature order.
- `park example remove 'RomanParse.parse("xiv")'`: delete that example's lines
  wherever they are written.
- `park test [Process.function]`: check examples against implementations,
  reporting each failure's call and source location.
- `park build`: validate the project and regenerate `park.html`.

Show an implemented function with fewer than two distinct examples as
`needs examples`; prohibit body execution until ready. Known example inputs
still return their outputs. Removing an example can return a body to that
state. `park test` fails for unready implementations.
Readiness is an authoring requirement, not a verification result: `park run`
does not run the tests or require them to have passed. Implementation checks
remain strict for every example, including deliberate exceptions to a rule;
there is no exception annotation or test exemption.

Exit codes:

- `0`: success.
- `1`: evaluation or test failure.
- `2`: usage or project validation error.
- `3`: unresolved call.

Report the exact unresolved call, including misses inside dependencies; the
question is about that innermost call.

### Question queue

- `questions.txt` lists one call per line in call syntax, in arrival order,
  after a fixed comment header. Duplicates are queued once.
- Every publish (`build`, `example add/remove`, and runs or tests that queue
  questions) reloads the queue against the proposed project, drops questions
  that now have an exact example or whose implementation is ready to run, and
  rewrites the file. An empty queue removes the file.
- Lines that no longer parse or bind to a function (after a rename, say) are
  dropped with a notice on stderr.
- The queue is not part of the project revision, so it never invalidates the
  cache.

## Values, example database, and cache

1. Support null/None, booleans, strings, signed 64-bit integers, lists, and
   string-keyed maps recursively. Reject floats, non-string map keys, and
   out-of-range integers with a useful source location. Keep JSON numbers exact rather than decoding them through float64.
2. Define canonical JSON with recursively sorted map keys, preserved list order,
   normalized integer encoding, and consistent string escaping. Canonicalize the
   object of named inputs, so argument order and calling style never change a
   key. Validate that each dispatched call supplies every declared input exactly
   once, and nothing else, before lookup.
3. Index authored examples by fully qualified function plus canonical input.
   Reject duplicate inputs regardless of how the call is written; conflicting outputs
   must never silently replace an existing expectation. Remove then add to
   change one.
4. Keep authored expectations separate from computed cache entries. Authored
   examples always win. Computed evaluations do not count toward the two-example requirement
   and do not appear among the catalog's authored examples.
5. Cache each successful body evaluation in an immutable JSON file keyed by a
   hash of the function, canonical input, and an execution revision. Include a
   schema version and provenance in the record. Reject a conflicting output for
   an existing key; never cache failures or unresolved calls.
6. Use a conservative project-wide execution revision: hash sorted Starlark
   sources and an engine/codec version. This invalidates old results
   when code, dependencies, examples, or runtime semantics change. Old revision
   files may remain unused; cache deletion is always safe.

Use canonical JSON files rather than SQLite: the authored example set is
small, indexes can be rebuilt in memory, and no schema migration service is
needed.

## Runtime behavior

For each dispatched call:

1. Bind positional and keyword arguments to the named inputs and validate
   them, then check authored examples.
2. Check the computed cache for the current execution revision.
3. If a body exists and has two distinct authored examples, execute with a step
   limit and cancellation support, validate the result, and cache success.
4. Otherwise return an unresolved-call error, which the CLI turns into a
   queued question. This includes bodies blocked by the two-example requirement.
   Retain the question until an exact example exists or the body is ready.
   Pruning questions does not execute bodies or verify their dependencies.
   Errors while executing ready bodies remain evaluation errors and aren't queued.

Keep a deterministic environment: expose only approved builtins and project
process namespaces, freeze module globals, and expose no filesystem, network,
environment, clock, or randomness APIs. Disallow native `load()`;
dependencies come from `Process.function` references. Validate/inspect function definitions before
executing module initialization and ensure initialization cannot call exported
process functions. Permit a module docstring, function definitions, and (in
regular modules) literal constants at the top level. Example-only declarations
are removed before compilation so their names dispatch to examples. Namespace bindings come from the host. Set explicit
step, input-size, and project-size limits:

- 1 MiB per canonical input or output; value nesting at most 64 levels.
- 2 MiB per module; 32 MiB of project source.
- 2,048 modules and 32 MiB of generated HTML.
- 1,000,000 Starlark steps and 128 dispatched body calls in the call stack.
- A five-second deadline per top-level evaluation or test example.

`park test` must execute the target implementation while bypassing its own
example and computed-cache shortcuts; otherwise every assertion would pass by
returning its expected value. Dependencies retain normal dispatch, with computed
cache disabled throughout the test run. Report implementation checks separately
from example-only records, which can be validated as data but cannot prove a
body. Do not persist computed results from tests. A dependency miss fails a test
and is queued as a question.

## Generated `park.html`

Render a complete static page using Go `html/template`, with inline CSS and no
server or external assets. Include:

- A process index, descriptions, dependencies, and function anchors.
- Every function's signature, description, source path,
  implementation/example-only/needs-examples state, and authored example count.
- Each function's first two and last two authored examples in example order,
  showing each named input and the expected output as Starlark literals under
  their names. Deduplicate overlapping rows: counts 0–4 show all examples, and counts
  5+ show four plus an omitted-count label.
- A project content revision so the page can be compared with source state.

Sort processes and functions by name; use example list order for first/last
selection. Escape all authored content. Avoid generated timestamps so unchanged
inputs produce byte-identical HTML.

All successful example additions and removals regenerate the full catalog from the proposed project state. `park build` handles external file
edits; there is no watcher in v1. Runtime cache writes alone do not regenerate
the page.

Validate the proposed project and render HTML before writing anything; fail
before changes on validation or render errors. Then write each edited module,
then `park.html`, each through a temp file and atomic rename. Modules are the
source of truth and the catalog is derived, so there is no lock or recovery
journal: if a write fails after a module is saved, report that `park.html` is
stale and that `park build` regenerates it. Generated HTML is never an input
database.

## Acceptance checks

- **Loading:** malformed Process blocks, signatures, return statements,
  example lines, unknown functions, duplicate examples (within and across
  files), duplicate processes, and dependency cycles produce actionable errors
  with no writes.
- **Examples:** positional and named calls share one key; map order doesn't
  matter; integers round-trip exactly; examples written in another module count
  toward the named function; source edits invalidate the cache.
- **Engine:** cache hits and misses, two-example gating, same-process and
  dependency calls, named inputs passed to bodies in order, example-only
  dispatch, and a deliberately incorrect implementation failing `park test`.
- **Catalog and edits:** example add/remove is reflected in source and HTML,
  add then remove restores the module byte for byte, comments survive,
  first/last selection for counts 0–6, escaped content, deterministic output,
  and manual edits plus `build`.
- **CLI:** call parsing and literal typing, exit codes, an unseen input
  executing a body, a second run using the cache, and an unresolved call
  (including inside a dependency or a test) queued once, shown in `list` and
  `park.html`, and removed when answered by an example or an implementation.
- **Release check:** keep the README quick start working; run `gofmt`,
  `go vet ./...`, and `go test ./...`; build `./cmd/park` and exercise a
  temporary project from init through example edits, run, test, and rebuild.

Done means a user can write a `.star` module, add examples from the CLI or by
hand, run a call or answer a queued question, and open an accurate `park.html` after each CLI
edit. Removing examples and manually rebuilding must leave the example set and
catalog consistent.

## V2 direction: bounded call cache

Extend caching with a bounded sample of each function's runtime calls:

- Retain the first few successful calls and the most recent few calls. Keep the
  early window fixed and advance the recent window as calls succeed.
- Store retained calls as canonical JSON input/output records, with function,
  execution revision, and call order. Store a call shared by both windows once.
- Include successful authored-example hits, computed-cache hits, and body
  evaluations. These are runtime calls, not authored examples.
- Keep revision provenance so stale outputs cannot satisfy current lookups. Test
  runs continue to avoid persistent cache writes.
- Keep authored examples authoritative. Retained calls do not become authored
  examples or count toward the two-example requirement.
- Decide window sizes, repeated-call accounting, revision retention, and any
  catalog display during v2 design. This is outside the current acceptance
  criteria.

## Dependency references

Dependencies are pinned in `go.mod`/`go.sum`:

- [Starlark in Go](https://github.com/google/starlark-go): embedded interpreter.
- [Starlark API](https://pkg.go.dev/go.starlark.net/starlark): evaluation,
  freezing, execution limits, and cancellation interfaces.
- [Starlark syntax](https://pkg.go.dev/go.starlark.net/syntax): parsing modules,
  example lines, and CLI call expressions.
