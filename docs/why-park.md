# Why Park?

Park is a small runtime and project format for pure, rule-like functions.
It combines explicit lookup tables with code for the cases outside the table.
For a walkthrough without code, see [Park for business teams](for-business.md).

## Examples answer calls

An authored example is part of the function's runtime behavior. A call whose
inputs match an example returns its output, even when an implementation exists.
A function can also remain an example-only table.

Code handles inputs without a matching example. If neither an example nor an
implementation answers a call, Park records a question. Errors while executing
code are reported as errors.

Code written before a function has two distinct examples still runs, but its
answers are unconfirmed and queued as proposals. Two examples meet the current
readiness policy; they do not establish correctness.

## Checks are separate

`park test` runs each implementation against its authored examples, bypassing
the target function's own example and cache lookups. Dependencies retain normal
dispatch, with computed caching disabled throughout the test run.

Every example is a strict implementation check. A deliberate exception saved
as an example answers calls at runtime, but fails the check if the code disagrees.
The implementation must handle that case too. Example-only tables are validated
as data and cannot prove an implementation.

## Runtime and tooling

Functions use [Starlark](https://github.com/bazelbuild/starlark) in stateless
Process namespaces. They have no filesystem, network, clock, environment, or
randomness access. The runtime enforces input, project, execution-step, and
time limits. The same engine runs in the CLI and browser playground.

`park run --why` records the calls and examples used to produce an answer.
`park.html` lists functions with their signatures and examples. The local
backend adds isolated drafts, source history, reviews, and releases.

Park suits small deterministic rules and mappings. An application handles I/O
and external actions around them. See the [reference](reference.md) for exact
semantics, [design decisions](design.md) for rationale, and
[roadmap](roadmap.md) for proposed work.
