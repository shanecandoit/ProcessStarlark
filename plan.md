# Park CLI v1 implementation plan

## Outcome and scope

Build a local Go CLI named `park` that registers processes and their exported
functions, runs pure Starlark implementations, manages examples in YAML files,
and maintains an offline `park.html` catalog. `docs/reference.md` documents the
format for users; this document defines the implementation contract, with these
v1 decisions:

- Use `go.starlark.net` and ordinary `.star` files. Represent `Process`, `use`,
  exports, and examples as data instead of implementing the original custom
  `Process { ... }` or `example` syntax yet. Use a small declarative signature
  in each sidecar function entry to name its input and output fields.
- Keep one YAML sidecar beside each `.star` module. It contains registration,
  descriptions, signatures, and authored examples. Store computed results in
  `.cache/` as disposable, revisioned canonical JSON records.
- Require two distinct authored input examples before executing an
  implementation body. Examples for dependencies count toward the dependency
  function only.
- Regenerate `park.html` after CLI changes to processes, functions, or examples.
  Include the first two and last two examples for each function, without
  duplicates.
- Support an interactive prompt for an unknown call to an example-only function.
  Entering an implementation means editing a `.star` file and rerunning; an
  inline code editor, synthesis, nested processes, remote services, and a full
  custom language parser are outside v1.

An initial v1 CLI is implemented in `cmd/park`. Keep the README's status and
quick start, and `docs/reference.md`'s usage details, current. The acceptance checks below describe the expected v1
behavior and should remain covered as the tool evolves.

## Project format

The repository contains `cmd/park` and internal CLI, project, engine, samples,
and catalog packages. Ship a copyable flat project in `fixtures/roman/`:

```text
roman_parse.star
roman_parse.yaml
roman_add.star
roman_add.yaml
.cache/             # Disposable computed results, canonical JSON
.park/              # Project lock and recovery journal
park.html           # Generated offline catalog
```

Use one YAML sidecar per process. All modules and sidecars live directly in the
project root. `roman_parse.yaml` combines registration and examples:

```yaml
version: 1
name: RomanParse
description: Convert Roman numerals to integers.
module: roman_parse.star
uses: []
functions:
  - function: parse(in) out
    description: Parse a lowercase Roman numeral.
    implementation: parse
    examples:
      - {in: "ix", out: 9}
      - {in: "vi", out: 6}
  - function: tokenize(tx) res
    description: An example-only token vocabulary.
    examples:
      - {tx: "ix + vi", res: ["ix", "+", "vi"]}
      - {tx: "vi + ix", res: ["vi", "+", "ix"]}
```

The module path is relative to the sidecar and has the same basename. An
example-only process may omit `module`; an example-only function omits
`implementation`. Every function entry requires a signature and an `examples`
list, even if initially empty. Function descriptions live in their own entries.

Signatures have the restricted form `name(input_label) output_label`: one input
and one output, distinct identifier labels, and no types or expressions. Each
example contains exactly those two fields. Labels are data names and need not
match implementation parameters: `parse(in) out` calls `def parse(text)` with
one positional value. Objects and lists are passed whole, never expanded into
arguments. Explicit null differs from an omitted field. There are no example
IDs; fully qualified function plus canonical input identifies a pair.

Use compact YAML rows for short pairs and block formatting for larger values.
The following list item belongs under `functions` in a sidecar:

```yaml
- function: add(request) response
  examples:
    - request:
        left: 9
        right: 6
      response:
        sum: 15
```

Keep YAML for authoring; JSONL import/export is outside v1. The Roman fixture
handles only lowercase `i`, `v`, and `x` and does not validate Roman grammar.

### Registration and discovery

`park register FILE.star` generates or refreshes the matching sidecar:

- Register public top-level definitions. Names beginning with `_` are private.
- Default `roman_parse.star` to process name `RomanParse`; allow `--name`.
- Give new exports `name(in) out` signatures and empty example lists.
- Read module and function docstrings as descriptions.
- Detect references to already registered process namespaces. Register
  dependencies first; accept explicit additions through `--uses A,B`.
- Preserve authored examples, signatures, example order, comments, and
  example-only entries when refreshing metadata.
- When a registered implementation disappears from source, retain its entry and
  examples as an example-only function. Do not silently discard authored data.
- Require exactly one positional parameter for each implementation, without
  defaults, variadic parameters, or keyword arguments.

Manually registered exports are also supported. Only registered functions are
public. Calls between exports, including within one process, use dispatch
wrappers; private helpers are ordinary Starlark functions.

Use the current directory or walk upward to a directory with `.park/`, `.star`
modules, or YAML sidecars. `--root PATH` explicitly selects the project root. An
initial `register` can start in the current directory. `park init [DIR]` creates
an empty catalog and internal project state without overwriting existing
sidecars or generated project state. `park build` validates registered data and
regenerates HTML; it never automatically re-registers removed exports.

Validate unknown fields, duplicate YAML keys/processes/functions, signatures,
missing references, duplicate inputs, and dependency cycles. Restrict process
and function names to safe identifiers. Keep referenced files inside the root;
reject symlinked source and sidecar files. Preserve comments and example row
flow/block style during edits.

## CLI contract

Use Go's standard `flag` package. Global `--root PATH` precedes the command;
command flags precede positional operands. Value flags contain one JSON value.
Errors and prompts go to stderr; a successful run prints one JSON output.

- `park init [DIR]`: create an empty project and catalog.
- `park register roman_parse.star`: generate or refresh a module's sidecar.
  Optional flags: `--name RomanParse` and `--uses OtherProcess`.
- `park process add --name Names`: register an empty example-only process.
  Optional `--module file.star` associates source; use `register` to generate
  exports from it. Both commands accept existing source in the project root.
- `park process remove Names`: remove its sidecar and examples. Refuse while
  another process uses it, and leave Starlark source intact.
- `park function add --signature 'parse(in) out' RomanParse.parse`: append an
  empty example-only function entry. Add `--implementation parse` for a body.
- `park function remove RomanParse.parse`: remove the entry and its examples,
  leaving source intact.
- `park example add --input '"ix"' --output '9' RomanParse.parse`: append a
  validated pair under the function's declared labels.
- `park example remove --input '"ix"' RomanParse.parse`: remove by canonical
  input, without an example ID.
- `park list`: list processes, functions, readiness, and example counts.
- `park run --input '"ix"' RomanParse.parse`: look up or compute one output.
- `park run --interactive --input '"iv + i"' RomanParse.tokenize`: prompt for an
  output if the example-only call is unresolved and stdin is a terminal.
- `park test [Process.function]`: check authored outputs against
  implementations.
- `park build`: validate registered files and regenerate `park.html`.

Permit registration before adding examples. Show an implemented function with
fewer than two distinct authored inputs as `needs examples`; prohibit body
execution until ready. Known authored inputs can still return their outputs.
Removing an example can return a body to that state. Structural validation still
rejects malformed data; `park test` fails for unready implementations.

Exit codes:

- `0`: success.
- `1`: evaluation or test failure.
- `2`: usage or project validation error.
- `3`: unresolved call.

Prompt only with `--interactive` and a terminal. Report the exact unresolved
function and input, including misses inside dependencies.

## Values, example database, and cache

1. Support null/None, booleans, strings, signed 64-bit integers, lists, and
   string-keyed maps recursively. Reject floats, timestamps, custom YAML tags,
   aliases, non-string map keys, and out-of-range integers with a useful
   location. Keep JSON numbers exact rather than decoding them through float64.
2. Define canonical JSON with recursively sorted map keys, preserved list order,
   normalized integer encoding, and consistent string escaping. Canonicalize the
   input value directly, without an argument-array wrapper. Validate that each
   dispatched call supplies exactly one argument before lookup.
3. Index authored examples by fully qualified function plus canonical input.
   Reject duplicate inputs regardless of YAML presentation; conflicting outputs
   must never silently replace an existing expectation. Remove then add to
   change one.
4. Keep authored expectations separate from computed cache entries. YAML always
   wins. Computed evaluations do not count toward the two-example requirement
   and do not appear among the catalog's authored examples.
5. Cache each successful body evaluation in an immutable JSON file keyed by a
   hash of the function, canonical input, and an execution revision. Include a
   schema version and provenance in the record. Reject a conflicting output for
   an existing key; never cache failures or unresolved calls.
6. Use a conservative project-wide execution revision: hash sorted sidecars,
   Starlark sources, and an engine/codec version. This invalidates old results
   when code, dependencies, examples, or runtime semantics change. Old revision
   files may remain unused; cache deletion is always safe.

Use canonical JSON files for v1 rather than SQLite: the authored sidecar set is
small, indexes can be rebuilt in memory, and no schema migration service is
needed.

## Runtime behavior

For each dispatched call:

1. Bind and validate the single input value, then check authored examples.
2. Check the computed cache for the current execution revision.
3. If a body exists, require two distinct authored examples, execute with a step
   limit and cancellation support, validate the result, and cache success.
4. Otherwise return an unresolved-call error. In interactive mode, prompt for a
   JSON output, persist the input/output pair through the same example-add path,
   and retry the top-level call under a fresh project snapshot/revision. Do not
   mix results from before and after that edit. EOF or cancellation makes no
   change.

Keep a deterministic environment: expose only approved builtins and registered
process namespaces, freeze module globals, and expose no filesystem, network,
environment, clock, or randomness APIs. Disallow native `load()` in v1;
dependencies come from manifests. Validate/inspect function definitions before
executing module initialization and ensure initialization cannot call exported
process functions. Permit a module docstring, function definitions, and literal
constants at the top level. Namespace bindings come from the host. Set explicit
step, input-size, and project-size limits:

- 1 MiB per canonical input or output; value nesting at most 64 levels.
- 2 MiB per source or sidecar; 32 MiB of registered project data.
- 2,048 registered files and 32 MiB of generated HTML.
- 1,000,000 Starlark steps and 128 dispatched body calls in the call stack.
- A five-second deadline per top-level evaluation or test example.

`park test` must execute the target implementation while bypassing its own
example and computed-cache shortcuts; otherwise every assertion would pass by
returning its expected value. Dependencies retain normal dispatch, with computed
cache disabled throughout the test run. Report implementation checks separately
from example-only records, which can be validated as data but cannot prove a
body. Do not persist computed results from tests. A dependency miss fails a test
without prompting.

## Generated `park.html`

Render a complete static page using Go `html/template`, with inline CSS and no
server or external assets. Include:

- A process index, descriptions, declared dependencies, and function anchors.
- Every registered function's input/output signature, description, source path,
  implementation/example-only/needs-examples state, and authored example count.
- Each function's first two and last two authored examples in the function's
  YAML list order, showing JSON input and expected output under their declared
  labels. Deduplicate overlapping rows: counts 0–4 show all examples, and counts
  5+ show four plus an omitted-count label.
- A project content revision so the page can be compared with source state.

Sort processes and functions by name; use example list order for first/last
selection. Escape all authored content. Avoid generated timestamps so unchanged
inputs produce byte-identical HTML.

All successful process/function/example additions and removals regenerate the
full catalog from the proposed project state. `park build` handles external file
edits; there is no watcher in v1. Runtime cache writes alone do not regenerate
the page.

Serialize mutating commands with a project lock. Validate and stage source edits
and rendered HTML before publishing. Use temp files and atomic rename per file;
several files cannot be replaced atomically as one operation. Keep a small
recovery journal with original contents and staged changes so interrupted
mutations can be completed or rolled back on the next mutating command/build.
Fail before changes on validation/render errors; report publication failures
explicitly. Never print success while known source/catalog inconsistency
remains. Read-only commands refuse an unfinished mutation with a recovery
instruction. Generated HTML is never an input database.

Process/function removal leaves `.star` files in place and invalidates caches by
revision; it removes only the relevant sidecar or function entry, including its
examples.

## Implementation sequence and acceptance checks

1. **Project skeleton and loader.** Initialize the Go module, command entry
   point, typed schemas, root discovery, strict YAML parsing, and dependency
   validation. Add the RomanParse fixture and an example-only function. Verify
   malformed schemas, signatures, row fields, and references produce actionable
   errors with no writes.
2. **Example database.** Implement value conversion, canonical keys, sidecar
   add/remove, in-memory indexes, and revisioned JSON cache storage. Verify
   duplicate input rejection, exact integer round trips, map-order independence,
   and invalidation after source, example, and dependency edits.
3. **Starlark engine.** Add exports, dependency namespaces, dispatch wrappers,
   purity restrictions, limits, unresolved-call handling, and body verification.
   Verify cache hits, cache misses, two-example gating, internal/dependency
   calls, and a deliberately incorrect implementation failing `park test`
   despite examples.
4. **Catalog and mutation publication.** Build the template, process/function
   CRUD, full regeneration, lock, and interrupted-write recovery. Verify
   add/remove is reflected in HTML, first/last selection for counts 0–6, escaped
   content, deterministic output, manual edits plus `build`, and failure
   recovery.
5. **End-to-end CLI.** Wire commands, JSON I/O, exit codes, and opt-in prompts.
   Add RomanAdd with two self-examples and a dependency on RomanParse. Verify an
   unseen input executes the body, a second run uses the cache, and an unknown
   example-only call exits in unattended mode or saves an answer interactively.
6. **Documentation and release check.** Keep the README's quick-start
   commands working. Update `docs/reference.md` with commands, formats,
   supported values, catalog refresh behavior, and deferred syntax. Run
   `gofmt`, `go vet ./...`, and `go test ./...`; build `./cmd/park` and exercise
   a temporary project from init through additions, run, test, removals, and
   rebuild.

Done means a user can create a project, register a process and function, add two
YAML examples, execute or teach a call, and open an accurate `park.html` after
each CLI edit. Removing examples/functions/processes and manually rebuilding
must leave the example database and catalog consistent.

## V2 direction: bounded call cache

Extend caching with a bounded sample of each function's runtime calls:

- Retain the first few successful calls and the most recent few calls. Keep the
  early window fixed and advance the recent window as calls succeed.
- Store retained calls as canonical JSON input/output records, with function,
  execution revision, and call order. Store a call shared by both windows once.
- Include successful authored-example hits, computed-cache hits, and body
  evaluations. These are runtime calls, not positions in the YAML example list.
- Keep revision provenance so stale outputs cannot satisfy current lookups. Test
  runs continue to avoid persistent cache writes.
- Keep authored YAML authoritative. Retained calls do not become authored
  examples or count toward the two-example requirement.
- Decide window sizes, repeated-call accounting, revision retention, and any
  catalog display during v2 design. This is outside v1's acceptance criteria; v1
  keeps its revisioned computed-result cache and authored-example catalog.

## Dependency references

Dependencies are pinned in `go.mod`/`go.sum`:

- [Starlark in Go](https://github.com/google/starlark-go): embedded interpreter.
- [Starlark API](https://pkg.go.dev/go.starlark.net/starlark): evaluation,
  freezing, execution limits, and cancellation interfaces.
- [Go YAML v3](https://pkg.go.dev/go.yaml.in/yaml/v3): typed decoding, strict
  field checks, and node-based edits to retain YAML presentation.
