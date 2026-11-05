# Why Park?

This page is for programmers who want to know why Park exists and how it
differs from what they already use.

## The problem

Much of the logic in real programs is half data and half code: alias tables,
routing rules, error-code explanations, config migrations, pricing thresholds.
Most languages make you pick one form:

- **Code.** The rules end up in `if` chains, and the cases that matter live in a
  separate test file that drifts away from the code.
- **Config or data.** It's easy to read and edit, but any rule that doesn't fit
  the table needs an escape hatch back into code.

Park starts from the known cases and keeps them central as code is added.

## What Park does differently

1. **Examples come first, and they outrank the code.** A call whose input
   matches an authored example returns that example's output, even when an
   implementation exists. Examples are the source of truth, not checks run
   after the fact.
2. **Code is optional and has to earn its place.** A function can stay an
   example table forever. Once it has a body, the body won't run until the
   function has at least two distinct examples.
3. **Unknown inputs become new examples.** When nothing answers a call, Park
   reports it instead of guessing. With `--interactive`, you supply the answer
   and Park saves it as a new example.
4. **Everything is pure and deterministic.** Functions have no filesystem,
   network, clock, environment, or randomness. Results are reproducible and safe
   to cache, and `park test` can check every example.
5. **Examples are documentation.** `park.html` is an offline catalog of every
   function, with its signature, its readiness, and real example pairs.

## Comparison

| | Where examples live | Do examples affect what runs? | Can a function have no code? |
|---|---|---|---|
| Ordinary code + unit tests | Separate test files | No | No |
| Doctests | Docstrings | No; they only verify | No |
| Lookup tables / config | Data files | Yes, but there's no code path | Yes, but it can't generalize |
| **Park** | **Beside the function, as its spec** | **Yes; they answer matching calls first** | **Yes, and code can be added later** |

Related ideas:

- **Test-driven development** writes tests before code too, but the tests never
  answer calls at runtime, and a function can't consist of tests alone.
- **Programming by example** and **program synthesis** infer code from
  examples. Park doesn't infer anything in v1: a person writes the code, and the
  examples check it. Synthesis is listed as a possible future idea.
- **Spreadsheets** let you mix typed-in values and formulas, but nothing checks
  a formula against the values someone typed in by hand.

## What Park is not

Park isn't a new general-purpose language. It's a small runtime and project
format that groups [Starlark](https://github.com/bazelbuild/starlark) functions
in stateless Process namespaces, with examples written beside the code. It suits small, pure,
rule-like functions that people need to read, check, and extend by adding cases.
It isn't meant for I/O, large programs, or performance-critical code.

## Known tension

`park test` runs each implementation against every one of its authored
examples. A function whose examples are deliberate exceptions to its rule (for
example, irregular plurals beside an "add s" rule) works correctly when called,
but it fails `park test`. Whether to support marked exceptions is an open
design question.
