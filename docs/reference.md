# Park reference

The full contract is in the [implementation plan](../plan.md). This page covers
what you need to use the `park` CLI.

## Core concepts

- **Process:** a named group of functions, such as `RomanParse`. It's one `.star`
  module plus one `.yaml` sidecar, and it declares which other processes it
  `uses`.
- **Function:** takes exactly one input value and returns one output value. It's
  either *implemented* (it has a Starlark body) or *example-only*.
- **Example:** an authored input/output pair in YAML. Examples are
  authoritative.
- **Readiness:** an implemented function runs its body only after it has two or
  more distinct authored examples. Until then, it shows as `needs examples`.
- **Cache:** computed results go in `.cache/`. It's disposable and is
  invalidated whenever code, sidecars, or runtime semantics change.

## How a call is resolved

For each call, the runtime:

1. Returns the authored output if the input matches an example.
2. Otherwise checks `.cache/` for the current project revision.
3. Otherwise executes the implementation, if it's present and ready, and caches
   a successful output.
4. Otherwise reports an unresolved call. With `--interactive` and a terminal,
   an example-only function prompts for the output and saves the new pair.

Examples for a dependency count only toward that dependency's own readiness, and
computed results never count toward readiness. Execution has no filesystem,
network, clock, environment, or randomness APIs.

`park test` runs each implementation against its authored outputs. It bypasses
example lookup for the function under test, disables the computed cache, and
writes nothing to it. Example-only functions are validated as data and reported
separately.

## Project format

Each module sits next to its single YAML sidecar in a flat project directory:

```text
roman_parse.star
roman_parse.yaml    # Registration, descriptions, and authored examples
roman_add.star
roman_add.yaml
.cache/             # Disposable computed results (canonical JSON); safe to delete
.park/              # CLI lock and interrupted-edit recovery state
park.html           # Generated offline catalog
```

Commands find the project by searching upward from the current directory.
`--root PATH` selects another project.

### Sidecar (`roman_parse.yaml`)

```yaml
version: 1
name: RomanParse
description: Convert lowercase Roman numerals using i, v, and x.
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
    description: An example-only token vocabulary for teaching calls.
    examples:
      - {tx: "ix + vi", res: ["ix", "+", "vi"]}
      - {tx: "vi + ix", res: ["vi", "+", "ix"]}
```

- Each function entry has a signature, an optional description, an optional
  `implementation`, and an `examples` list.
- The signature `name(input_label) output_label` names the two fields of each
  example. The labels are data names only: `parse(in) out` can call
  `def parse(text)`. The whole input is passed as one positional argument.
- Function names are unique within a process. Examples don't need IDs; the
  function plus its canonical input identifies a pair.
- Omit `implementation` for an example-only function. Omit `module` when the
  whole process is example-only.
- Declare dependencies with `uses: [RomanParse]`, then call
  `RomanParse.parse(text)` from Starlark.

Larger values can use YAML block style:

```yaml
- function: add(request) response
  examples:
    - request:
        left: 9
        right: 6
      response:
        sum: 15
```

### Values

Inputs and outputs can be `null`, booleans, strings, signed 64-bit integers,
lists, or string-keyed objects, nested to any depth. Floats are outside v1. A
missing field differs from an explicit `null`. Object key order doesn't affect
example lookup.

### Modules

Each implementation is an ordinary top-level Starlark function that takes
exactly one positional parameter, with no defaults, `*args`, or keyword
arguments. Names beginning with `_` are private helpers. Module and function
docstrings become descriptions.

## Starting a project

```sh
park init my-project
cd my-project
# Write roman_parse.star, then generate its sidecar:
park register roman_parse.star
park example add --input '"ix"' --output '9' RomanParse.parse
park example add --input '"vi"' --output '6' RomanParse.parse
park run --input '"iv"' RomanParse.parse
park list
```

## Working with sidecars

`park register FILE.star` generates or refreshes the matching sidecar:

- Public functions become exports with `name(in) out` signatures and empty
  example lists.
- The process name comes from the file name (`roman_parse.star` becomes
  `RomanParse`); `--name` overrides it.
- References to already registered processes are detected as dependencies, so
  register dependencies first. `--uses A,B` adds dependencies explicitly.
- Rerunning `register` refreshes metadata without discarding signatures,
  examples, example order, comments, or example-only functions. If an
  implementation is removed from the source, its entry becomes example-only and
  keeps its examples.

`park build` validates the sidecars and regenerates `park.html`. It doesn't
register new source files, so run `register` after adding functions.

## Commands

```text
init [DIR]                    Create an empty project and catalog
register FILE.star            Generate/refresh one YAML sidecar
process add|remove            Register or remove a process
function add|remove           Register or remove an exported function
example add|remove            Add or remove a named input/output pair
list                          List processes and function readiness
run --input JSON P.fn         Run a function (optional --interactive)
test [P.fn]                   Check authored outputs against bodies
build                         Validate and regenerate park.html
```

- Value flags (`--input`, `--output`) take JSON.
- Flags come before positional operands. Run `park COMMAND --help` for flags.
- Normal runs never prompt. Pass `--interactive` to answer unresolved calls to
  example-only functions.
- `register` and the `example` commands regenerate `park.html` when they
  succeed. After editing YAML by hand, run `park build`. Runtime cache writes
  don't touch the catalog.
- The catalog shows each function's signature, its readiness, and its first two
  and last two authored examples, without duplicates.
- Keep `.park/` until an interrupted edit has been recovered with `park build`.

## Roadmap

- **v2 call samples:** keep each function's first few and most recent few
  calls as canonical JSON records, to show real usage. These samples are
  separate from authored examples and never count toward readiness.
- **Future language design:** possibly `Process { ... }` blocks, inline
  `example` assertions, return labels in source syntax, nested processes, and
  synthesizing implementations from examples. In v1, processes are flat YAML
  sidecars plus ordinary `.star` functions, and the interactive prompt accepts
  only an output value, not code.
