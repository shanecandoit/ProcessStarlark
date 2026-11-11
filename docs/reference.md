# Park reference

This page covers what you need to use the `park` CLI. The reasons behind these
rules are in [Design decisions](design.md); the full implementation contract is
in the [implementation plan](../plan.md).

## Core concepts

- **Process:** a stateless namespace of functions and examples, such as
  `WordAdd`. Each process is one `.star` module.
- **Function:** takes one or more named inputs and returns one output value.
  It's either *implemented* (it has a Starlark body) or *example-only*
  (`def f(x): pass`).
- **Example:** a call and its expected answer, such as
  `example: eval("one plus two") == 3`. Examples are authoritative. All examples in a
  project form one set, wherever they are written.
- **Readiness:** an implemented function is *ready* once it has two or more
  distinct examples. Until then it shows as `needs examples`, and its code
  still runs, but every answer it computes is *unconfirmed*: printed, proposed
  as an example in the question queue, and never cached. Readiness is about
  authoring, not verification; use `park test` to check code against examples.
- **Cache:** the CLI stores computed results in `.cache/`. It's disposable and
  is invalidated whenever any source file or the runtime changes. The browser
  playground doesn't cache.

## How a call is resolved

For each call, the runtime:

1. Returns the example's answer if the call matches an example.
2. Otherwise checks the cache (`.cache/`) for the current project revision.
3. Otherwise executes the implementation, if there is one. If the function is
   ready, a successful output is cached. If not, the output is unconfirmed: it
   is returned, proposed in the question queue, and not cached, and neither is
   any result that used it.
4. Otherwise (no example, no implementation) reports an unresolved call and
   adds it to the question queue (see below).

Errors from an implementation, such as invalid input, a failed operation, or an
execution limit, remain evaluation errors; they are not queued questions.

Computed results never count toward readiness. Execution has no filesystem,
network, clock, environment, or randomness APIs.

`park test` runs each implementation against its examples. It bypasses example
lookup for the function under test, disables the computed cache, and writes
nothing to it. Example-only functions are validated as data and reported
separately.

All examples are checked, including any intended exceptions to a general rule.
Matching examples still answer runtime calls, but a disagreeing implementation
fails `park test`. There is no exception annotation or automatic test exemption;
the implementation must handle those cases too for the tests to pass.

## Project format

A project is a flat directory of `.star` modules:

```text
word_values.star    # Process WordValues: example-only word meanings
number_words.star   # Process NumberWords: number structure
word_add.star       # Process WordAdd: addition
.cache/             # Disposable computed results (canonical JSON); safe to delete
park.html           # Generated offline catalog
```

Commands use only the current directory as the project root; they never search
parent directories. `--root PATH` explicitly selects another project.

### Modules

Write one `Process Name:` block per `.star` file, with everything inside it
indented:

```python
Process WordAdd:
    """Add two English numbers, as in twenty-two plus eleven."""

    example: eval("one plus two") == 3
    example: eval("twenty plus zero") == 20

    def eval(text):
        """Two numbers separated by plus; ignore capitals and extra whitespace."""
        words = text.lower().split()
        if len(words) != 3 or words[1] != "plus":
            fail("expected two number words separated by plus, such as twenty-two plus eleven")
        left = NumberWords.parse(words[0])
        right = NumberWords.parse(words[2])
        total = left + right
        return total
```

- A Process holds a docstring, `example:` lines, and function definitions.
  It has no instances, `self`, fields, inheritance, or shared state; pass any
  evolving state as an input and return its replacement. Nested Process blocks
  are unsupported.
- `Process` is optional. A plain `.star` file with top-level functions and
  `example:` lines is a process named after the file: `word_add.star`
  becomes `WordAdd`. Plain files may also define literal constants.
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
name. Choose names that say what the value is (`text`, `word`, `total`).
Renaming the returned variable renames the output in the catalog. Renaming or
reordering parameters changes how examples' positional arguments line up, so
update examples with the code.

### Examples

```python
example: parse("eleven") == 11                # positional
example: add(left=9, right=6) == 15           # named, in any order
example: WordValues.value("twelve") == 12     # a function in another process
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

Call another process with `NumberWords.parse(text)`. Park detects these
references as dependencies; there is nothing to declare, and dependency cycles
are rejected. Build validation rejects direct references to missing or private
members of process namespaces, even in functions that aren't run. Ordinary
methods on input values and local variables are unaffected. Within a process,
both `parse(text)` and `NumberWords.parse(text)`
use example-aware dispatch and enforce readiness. Callers can pass inputs by
position or by name: `Math.add(9, 6)` and `Math.add(right=6, left=9)` are the
same call and share examples and cached results.

Both module forms use Park's pure-function runtime, including its restrictions
on `load()`, top-level execution, and supported value types.

## Starting a project

```sh
park init my-project
cd my-project
# Write word_values.star with `def value(word): pass`, then:
park example add 'WordValues.value("eleven") == 11'
park example add 'WordValues.value("two") == 2'
park run 'WordValues.value("eleven")'
park list
```

## Commands

```text
init [DIR]                        Create an empty project and catalog
list                              List processes and function readiness
run [--why] 'P.fn(args)'          Run a call; unanswered calls go to questions.txt
example add 'P.fn(args) == value' Add an example to the function's module
example remove 'P.fn(args)'       Remove an example wherever it is written
import [--add] P.fn FILE          Propose a CSV/TSV table's rows (or add them as examples)
test [P.fn]                       Check examples against implementations
build                             Validate and regenerate park.html
```

- Calls are one Starlark expression with literal arguments. Quote the whole
  call for the shell: `park run 'Math.add(9, right=6)'`.
- Output is printed as a Starlark literal, such as `33` or `["twenty-two", "plus", "eleven"]`.
- `run --why` also prints every call the answer depended on, indented by
  depth, with where each answer came from:

  ```text
  $ park run --why 'WordAdd.eval("twenty-two plus eleven")'
  33
    WordAdd.eval(text="twenty-two plus eleven") = 33  code     word_add.star:7
      NumberWords.parse(text="twenty-two") = 22       code     number_words.star:7
        WordValues.value(word="twenty") = 20          example  word_values.star:24
        WordValues.value(word="two") = 2              example  word_values.star:6
      NumberWords.parse(text="eleven") = 11           code     number_words.star:7
        WordValues.value(word="eleven") = 11          example  word_values.star:15
  ```

  Sources are `example`, `code`, `unconfirmed code`, `unknown` (the call
  became a question), and `failed` (with the error). A call that didn't return
  shows `?`. `--why` recomputes rather than reading `.cache/`, so the tree is
  always complete.
- `example add` writes the line into the target function's own module, after
  that module's last example. `example remove` deletes the example wherever it
  is written.
- `import` reads a CSV or TSV file whose first row names the function's
  inputs and output, in any order. For example, in the shipping sample
  (`examples/shipping/`):

  ```text
  $ cat zones.csv
  country,result
  DE,international
  CA,europe
  $ park import Zones.zone zones.csv
  Read 2 rows for Zones.zone: 1 new, 0 already in the examples, 1 conflicting.
    conflict, not imported: line 3: Zones.zone("CA") is "north-america" in the examples, "europe" in the table
  Proposed 1 answer in questions.txt. ...
  ```

  New rows become proposed answers to confirm with `park example add`;
  `--add` writes them as examples instead. Rows that contradict an example are
  never imported. If the function has code, each new row is checked against
  it. A ready function's rows are not queued, because its code already answers
  them; only disagreements are reported. If the code calls a dependency that
  can't answer (say `Zones.zone("ZZ")`), that call is queued as a question,
  as `park test` does.
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

Park queues two kinds of question in `questions.txt`.

**Unanswered calls.** No example matches and there's no code:

```text
$ park run 'WordAdd.eval("twenty-two plus twelve")'
park: no answer for WordValues.value(word="twelve"); added to questions.txt
Answer it with an example:
  park example add 'WordValues.value("twelve") == ...'
or implement WordValues.value with at least two distinct examples.
```

**Proposed answers.** The code answered, but the function has fewer than two
examples, so the answer is unconfirmed:

```text
$ park run 'Words.shout("hi")'
"HI!"
park: unconfirmed: this answer comes from code with fewer than two examples.
Proposed in questions.txt; confirm each that is right:
  park example add 'Words.shout("hi") == "HI!"'
```

A proposal is written as `call == value`, so confirming one is `park example
add` with the whole line. If the answer is wrong, add the right example or fix
the code; a newer answer for the same call replaces the older proposal. Write
code first, run it, and confirm two answers, and the function is ready.

- The question is about the call Park couldn't answer, which may be inside a
  dependency, as here. `park test` queues its misses too.
- Open questions are listed in `questions.txt`, at the top of `park list`, and
  at the top of `park.html`.
- A question is removed automatically once an exact example exists for its
  call, or once its function is ready (two examples). The file is deleted when
  the queue is empty. Removing a question because its function is ready does
  not execute or verify the code; `park test` does that.
- Park never reads from the terminal; answering is always a separate command
  or a source edit.

Exit codes: `0` success, `1` evaluation or test failure, `2` usage or project
validation error, `3` unresolved call.

## Browser playground

The playground (<https://shanecandoit.github.io/ProcessStarlark/>, or locally `sh serve.sh`, then
<http://localhost:8000>) runs the same
engine compiled to WebAssembly, so calls, examples, questions, `park test`,
and the catalog follow the rules on this page. The differences:

- The project lives in the browser: saved locally, and shareable as a link
  that holds every file.
- There's no `.cache/`; every call is computed fresh.
- Word-number addition is the default sample. A guided tour (**Tour**, or a
  `#tour=addition` link) introduces word meanings as examples and composition
  as code. A first visit opens the sample with collapsible guidance; the tour
  is optional. Saved projects and tour progress win over the default. The `#tour=shipping` link still opens the business tour.
- Roman numerals remain in the sample picker and `fixtures/roman/`; use
  `park --root fixtures/roman ...` to select them in the CLI.
- Function tables are editable (D21). Type a row in the blank last row and
  press Enter, click a row to change it, or paste rows from a spreadsheet.
  Each edit is written into the `.star` file as an example line. Cells take
  their column's type, so `US` in a column of text means `"US"`.
- **Import file…** under each table reads a CSV or TSV file, the playground's
  `park import`. Pasted and imported rows go in **as examples** (like
  `--add`) or **as proposals to confirm** (the CLI default). A first row that
  names the columns may list them in any order; without one, rows hold the
  inputs, then the output.
- Questions can be dismissed without answering, which is like deleting the
  line from `questions.txt`.
- Every run can show **How this answer was made**, nested steps from the same
  trace as `park run --why`. Saved answers and rules have distinct labels.
  Steps open their example rows or code; **Execution details** holds the raw
  call tree. A saved exact example has no computed children.
- The word-number sample accepts a plain expression. **Use a function call**
  reveals the underlying call. **Show code** opens the editor; the guide and
  explanation can be collapsed. These choices are remembered locally.
- Results and checks are marked stale after source edits; stale explanation
  links are disabled until rerun.
- Questions appear in a panel. Answering one adds the example, exactly like
  `park example add`, then reruns the last call.
- *Download* saves a zip that the CLI opens as a normal project.

See [plan-web-playground.md](plan-web-playground.md) for how it's built.

## Roadmap

- **v2 call samples:** keep each function's first few and most recent few
  calls as canonical JSON records, to show real usage. These samples are
  separate from authored examples and never count toward readiness.
- **Later:** per-function content hashing, overrides that answer calls
  without checking code (D17), nested processes, and
  synthesizing implementations from examples. See
  [Direction](ideas-direction.md) and the
  open questions in [Design decisions](design.md#open-questions).
