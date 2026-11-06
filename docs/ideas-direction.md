# Idea: where Park goes next

Status: direction notes, 2026-11-06. Nothing here is decided until it moves
into [design.md](design.md) as a numbered decision.

## Positioning

Decided 2026-11-06: **the idea is for business readers; the technology is
for programmers; the goal is a B2B product.**

- **The idea** is a different handoff between the people who know the rules
  and the developers who build them. Business users write examples instead of
  requirements; the examples are the system's answers from day one;
  developers backfill code for the general rule; gaps become questions back to
  the rule's owner. That has to make sense to non-programmers, both the users
  who'd write examples and the buyers who'd pay. See
  [Park for business teams](for-business.md).
- **The technology** (the semantics, the CLI, the design log, the playground)
  is for programmers, and is what makes the idea credible. Language people
  remain an audience for the write-up (#7) and the related work below.
- **The product** would be a B2B SaaS or similar. Today's code is the engine
  and a proof of the idea, not the product. See [Product path](#product-path).

Two pitches, one idea:

- Business: **write down the answers you know; code fills in the rest; anything
  else, Park asks.**
- Programmers: **a function is a table of examples a person wrote, with code
  filling in the gaps, and the examples always win.**

Buyers will compare Park with specs and tickets, spreadsheets, rules engines
and decision tables (DMN, BRMS), low-code builders, and BDD scenarios. The
business doc answers each. The short version: those tools either describe the
system without running, or run without asking, and Park's examples run *and*
its gaps ask.

### Related work to position against (for programmers)

Programmers and language people will ask "how is this different from X?"
first.

| Work | Shared idea | Park's difference |
|---|---|---|
| Babylonian programming (Rauch, Hirschfeld) | Examples live in the source | Park's examples answer calls at runtime |
| Hazel (Omar et al.) | Holes, and evaluation around holes | Park's holes are filled with data, then optionally code |
| FlashFill / programming by example | Functions from examples | Park doesn't infer code; people write it, examples check it |
| Glamorous Toolkit example methods | Examples as first-class, browsable objects | Examples are the function's authoritative partial definition |
| Unison | Content-addressed definitions, cached results | Park caches by project revision today; see below |
| Default logic, rules with exceptions | A general rule plus overriding facts | Park's open question D17 is exactly this |
| Spreadsheets | Typed-in values beside formulas | Park checks formulas against the typed-in values |

## Ideas, in rough priority order

### 1. `park run --why`: provenance for every answer

**Status: done** ([D20](design.md#d20-park-run---why-shows-where-an-answer-came-from)),
in the CLI and the playground. Because examples outrank code, a wrong example
silently wins.
Provenance shows where an answer came from, recursively:

```text
$ park run --why 'Shipping.quote("DE", 800)'
2799
  Shipping.quote("DE", 800) = 2799        code   shipping.star:11
    Zones.zone("DE") = "international"    example zones.star:9
```

Each node is one dispatched call: the call, its output, and its source
(`example`, `code`, `unconfirmed code`, or `unknown`) with a location. The
same tree feeds the playground's "Why this answer" panel.

Settled: `--why` recomputes instead of reading the cache, so there is no
`cache` kind and every tree is complete. If per-function hashing (#3) lands,
caching the tree beside the result would bring cache hits back.

### 2. Spec versus override (resolving D17)

"Examples outrank code" plus "every exception fails `park test`" is the most
interesting semantic tension in Park. Two constructs would resolve it:

- `example:` is a **spec**: the code must agree, and `park test` checks it.
- `override:` (name open) is a **defeasible fact**: it answers calls and
  outranks code, but the code is not expected to agree.

Irregular plurals and negotiated prices become overrides; the rule stays
clean. Questions to settle: does an override count toward readiness (probably
not: it checks nothing)? Should `park test` report how many overrides the code
happens to agree with (a hint that the override is redundant)?

### 3. Per-function content hashing (exploring Unison ideas)

**Status: wanted.** Today any edit invalidates the whole cache (D14). Instead,
hash each function by its normalized body, its examples, and the hashes of the
functions it calls. Then:

- editing one function invalidates only it and its callers;
- a cached result is keyed by the hash of what produced it, so reverting an
  edit makes old results valid again;
- renaming a parameter or reformatting could be hash-neutral if hashing works
  on the resolved syntax tree rather than bytes.

Further Unison-style directions to look at, roughly in order of ambition:

- **Names as metadata.** Store definitions by hash and treat `Process.fn` as a
  pointer. Renames stop being edits.
- **Example sets as values.** An example set is itself hashable. Two projects
  sharing a vocabulary (like `Zones.zone`) could share its hash.
- **Cache as a shared, append-only store.** Pure, deterministic, hashed
  results can be shared between machines safely.

Blocker to settle first: calls go through `Process.fn` names resolved at load
time, and dependencies are whole processes (`Uses`). Hashing per function
needs per-function dependency edges.

### 4. One interface for unknowns (the "Ask" handler)

**Status: internal architecture, not user vocabulary.** An unresolved call is,
in language terms, an `Ask` effect, and several things can handle it: the CLI
queue (`questions.txt`), `park test` (fail and queue), the browser playground
(answer inline), a default value, or a synthesizer. Today the engine returns
`*Unresolved` and each caller handles it ad hoc.

What the framing buys in practice is a single engine interface:

```go
type Resolver interface {
    // Resolve answers a call nothing else could, or returns ErrUnknown.
    Resolve(ctx context.Context, call Call) (any, error)
}
```

That interface is also what fill-and-resume (below) and LLM proposals plug
into. Keep the word "effect" out of user-facing docs.

### 5. Fill-and-resume

Today answering a question means re-running the whole call. Instead, the
playground could suspend the computation at the innermost miss, take the
answer inline, and continue. Because everything is pure and deterministic,
"continue" can simply be a replay with the new example added. True
continuation capture isn't necessary. The demo effect is the same: ask, answer,
see the result. The playground already does the simple version: answering a
question reruns the last call.

### 6. Browser playground

**Status: phases 1 and 2 done** (runs, shares, saves, downloads). See
[plan-web-playground.md](plan-web-playground.md). It's the biggest reach
multiplier: nothing to install, and `park.html` becomes live. The `--why`
view is in, and a GitHub Pages workflow publishes it on every push to `main`.

### 7. Write-up

A short essay, "Examples that outrank code", covering the semantics, the
related work above, and the D17 tension. Venues: a blog post, then
r/ProgrammingLanguages, the LIVE workshop, PX or Onward!.

## Left open on purpose: synthesis

LLM or enumerative synthesis is the obvious move (people own the examples,
a model writes the code, and the examples always win). Park should not lead
with it, but nothing should close it off. Constraints that keep the door open:

- Synthesized code arrives as a **proposal** through the same question queue
  as unconfirmed answers (D18). Nothing is accepted without a person.
- `park test` is the acceptance check, unchanged.
- No model, API key, or network in the core runtime. A synthesizer is an
  external `Resolver` or a separate command.

## Product path

What a business team would need before using Park day to day, roughly in
order. None of it is built; the engine is.

1. **Hosted playground** with the shipping sample first, as the thing to send
   people. (The Pages workflow publishes it; the sample is already first.)
2. **A table editor for examples and questions,** so rule owners never see
   source files. Started: playground tables are editable, and spreadsheet
   rows can be pasted in (D21). Next: a view with only tables and questions,
   and values shown without Starlark quotes.
3. **Business-shaped values:** decimals and money, dates, and ranges or
   patterns in examples ("any parcel over 30 kg").
4. **Overrides** (#2): negotiated prices and one-off exceptions are everyday
   business cases, so D17 needs settling before this ships.
5. **Accounts, change history, and approvals:** who changed which answer,
   and review before it goes live. `--why` (#1) is the read side of the same
   audit story.
6. **An API** so other systems can ask Park for answers, and embedding Park as
   a library.

Until there's interest from real teams, keep this list as direction and keep
building the engine and the demo.
