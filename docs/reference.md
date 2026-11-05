# Park reference

This page covers what you need to use the `park` CLI. The reasons behind these
rules are in [Design decisions](design.md); the full implementation contract is
in the [implementation plan](../plan.md).

## Core concepts

- **Process:** a stateless namespace of functions and examples, such as
  `RomanParse`. Each process is one `.star` module.
- **Function:** takes one or more named inputs and returns one output value.
  It's either *implemented* (it has a Starlark body) or *example-only*
  (`def f(x): pass`).
- **Example:** a call and its expected answer, such as
  `example: parse("ix") == 9`. Examples are authoritative. All examples in a
  project form one set, wherever they are written.
- **Readiness:** an implemented function runs its body only after it has two or
  more distinct examples. Until then, it shows as `needs examples`.
- **Cache:** computed results go in `.cache/`. It's disposable and is
  invalidated whenever any source file or the runtime changes.

## How a call is resolved

For each call, the runtime:

1. Returns the example's answer if the call matches an example.
2. Otherwise checks `.cache/` for the current project revision.
3. Otherwise executes the implementation, if it's present and ready, and caches
   a successful output.
4. Otherwise reports an unresolved call and adds it to the question queue
   (see below).

Computed results never count toward readiness. Execution has no filesystem,
network, clock, environment, or randomness APIs.

`park test` runs each implementation against its examples. It bypasses example
lookup for the function under test, disables the computed cache, and writes
nothing to it. Example-only functions are validated as data and reported
separately.

## Project format

A project is a flat directory of `.star` modules:

```text
roman_parse.star    # Process RomanParse: code and examples
roman_add.star      # Process RomanAdd
.cache/             # Disposable computed results (canonical JSON); safe to delete
park.html           # Generated offline catalog
```

Commands use only the current directory as the project root; they never search
parent directories. `--root PATH` explicitly selects another project.

### Modules

Write one `Process Name:` block per `.star` file, with everything inside it
indented:

```python
Process RomanParse:
    """Convert lowercase Roman numerals using i, v, and x."""

    example: parse("ix") == 9
    example: parse("vi") == 6
    example: tokenize("ix + vi") == ["ix", "+", "vi"]

    def parse(text):
        """Parse a lowercase Roman numeral."""
        digits = {"i": 1, "v": 5, "x": 10}
        value = 0
        previous = 0
        for index in range(len(text) - 1, -1, -1):
            digit = digits[text[index]]
            value += -digit if digit < previous else digit
            previous = digit
        return value

    def tokenize(text):
        """An example-only token vocabulary."""
        pass
```

- A Process holds a docstring, `example:` lines, and function definitions.
  It has no instances, `self`, fields, inheritance, or shared state; pass any
  evolving state as an input and return its replacement. Nested Process blocks
  are unsupported.
- `Process` is optional. A plain `.star` file with top-level functions and
  `example:` lines is a process named after the file: `roman_parse.star`
  becomes `RomanParse`. Plain files may also define literal constants.
- Docstrings become descriptions in the catalog.
- Public functions are exported. Names beginning with `_` are private helpers,
  called by their bare names.

### Signatures come from the code

- **Inputs** are the `def`'s parameters, in order: `def add(left, right)` has
  inputs `left` and `right`. At least one is required, and defaults, `*args`,
  and `**kwargs` are unsupported.
- **The output** is named by the variable the function returns. Every `return`
  must be `return <name>`, using the same name each time:

  ```python
  def add(left, right):
      total = left + right
      return total          # signature: add(left, right) total
  ```

  `return left + right` and a bare `return` are rejected. The output name must
  differ from every input name. `result` is the conventional default.
- **Example-only functions** have a body of only `pass` and/or a docstring.
  Their output is named `result`.

Names are identifiers and can't be Starlark keywords, so `in` is not a valid
name. Choose names that say what the value is (`text`, `numeral`, `tokens`).
Renaming the returned variable renames the output in the catalog. Renaming or
reordering parameters changes how examples' positional arguments line up, so
update examples with the code.

### Examples

```python
example: parse("ix") == 9                     # positional
example: add(left=9, right=6) == 15           # named, in any order
example: RomanParse.parse("xii") == 12        # a function in another process
example: route({"component": "login", "labels": ["bug"]}) == "auth"
```

- An example line sits directly in the Process (or module) body, not inside a
  function. It may span several lines inside brackets.
- Arguments and the expected value are Starlark literals: `None`, `True`,
  `False`, integers, strings, lists, tuples (stored as lists), and dicts with
  string keys. Floats are outside v1.
- Name a function in the same process bare, and one in another process as
  `Process.function`.
- **Examples are project-wide.** An example can be written in any module and
  name any declared function. It counts exactly like one written beside the
  function, including toward readiness. By convention, write examples where
  they make sense to a reader. Examples for functions that are never called
  are fine.
- An example must name a declared function. To collect examples before writing
  code, declare `def f(...): pass`.
- Positional and named calls with the same values are the same example. Two
  examples for the same call are an error, even in different files.
- Examples don't need IDs; the function plus its canonical inputs identify one.

### Calls between processes

Call another process with `RomanParse.parse(text)`. Park detects these
references as dependencies; there is nothing to declare, and dependency cycles
are rejected. Within a process, both `parse(text)` and `RomanParse.parse(text)`
use example-aware dispatch and enforce readiness. Callers can pass inputs by
position or by name: `Math.add(9, 6)` and `Math.add(right=6, left=9)` are the
same call and share examples and cached results.

Both module forms use Park's pure-function runtime, including its restrictions
on `load()`, top-level execution, and supported value types.

## Starting a project

```sh
park init my-project
cd my-project
# Write roman_parse.star with `def parse(text): pass`, then:
park example add 'RomanParse.parse("ix") == 9'
park example add 'RomanParse.parse("vi") == 6'
park run 'RomanParse.parse("ix")'
park list
```

## Commands

```text
init [DIR]                        Create an empty project and catalog
list                              List processes and function readiness
run 'P.fn(args)'                  Run a call; unanswered calls go to questions.txt
example add 'P.fn(args) == value' Add an example to the function's module
example remove 'P.fn(args)'       Remove an example wherever it is written
test [P.fn]                       Check examples against implementations
build                             Validate and regenerate park.html
```

- Calls are one Starlark expression with literal arguments. Quote the whole
  call for the shell: `park run 'Math.add(9, right=6)'`.
- Output is printed as a Starlark literal, such as `["ix", "+", "vi"]`.
- `example add` writes the line into the target function's own module, after
  that module's last example. `example remove` deletes the example wherever it
  is written.
- Flags come before positional operands. Run `park COMMAND --help` for flags.
- `build` and the `example` commands regenerate `park.html` when they succeed.
  After editing `.star` files by hand, run `park build`. Runtime cache writes
  don't touch the catalog.
- The catalog shows each function's signature, its readiness, and its first two
  and last two examples, without duplicates. Examples from a function's own
  module come first.
- If an edit is interrupted after saving a module, `park build` regenerates the
  catalog. Modules are the source of truth.

## Questions

When Park can't answer a call, because it has no example and no code, it asks
you instead of guessing:

```text
$ park run 'Shipping.quote("JP", 800)'
park: no answer for Zones.zone(country="JP"); added to questions.txt
Answer it with an example:
  park example add 'Zones.zone("JP") == ...'
or implement Zones.zone.
```

- The question is about the call Park couldn't answer, which may be inside a
  dependency, as here. `park test` queues its misses too.
- Open questions are listed in `questions.txt`, at the top of `park list`, and
  at the top of `park.html`.
- Answer a question by adding an example for that call, or by implementing the
  function. Answered questions are removed automatically, and the file is
  deleted when the queue is empty.
- Park never reads from the terminal; answering is always a separate command
  or a source edit.

Exit codes: `0` success, `1` evaluation or test failure, `2` usage or project
validation error, `3` unresolved call.

## Roadmap

- **v2 call samples:** keep each function's first few and most recent few
  calls as canonical JSON records, to show real usage. These samples are
  separate from authored examples and never count toward readiness.
- **Later:** marking examples as deliberate exceptions, per-function versions,
  nested processes, and synthesizing implementations from examples. See the
  open questions in [Design decisions](design.md#open-questions).
