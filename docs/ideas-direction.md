# Idea: where Park goes next

Status: direction notes, 2026-11-06. Nothing here is decided until it moves
into [design.md](design.md) as a numbered decision.

## Positioning

Park is closer to a **programming-language showcase** than a business tool.
What exists is a precise semantics (examples outrank code, "I don't know" is
an answer, everything is pure and cached), a design log, and a CLI for
programmers. A business product would need an editing surface for
non-programmers, decimals and ranges, multi-user history, and an embedding
API. None of that is novel, and decision tables, DMN, and rules engines
already fill that space.

So: lead with the idea, aimed at programmers and language people. The
business-shaped examples (shipping zones) stay as demos of the idea, not as a
product.

The one-sentence pitch: **a function is a table of examples a person wrote,
with code filling in the gaps, and the examples always win.**

### Related work to position against

People will ask "how is this different from X?" first.

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

**Status: next.** Because examples outrank code, a wrong example silently wins.
Provenance shows where an answer came from, recursively:

```text
$ park run --why 'Shipping.quote("DE", 800)'
2799
  Shipping.quote("DE", 800) = 2799        code   shipping.star:11
    Zones.zone("DE") = "international"    example zones.star:9
```

Each node is one dispatched call: the call, its output, and its source
(`example` with location, `code`, `cache`, or `unconfirmed code`). The engine
already dispatches every call through one function, so recording a call tree
is a small change. The same tree feeds the playground's "why" view.

Open: show cache hits as their own kind, or as `code` with a marker? A cache
hit has no subtree unless the tree is cached too.

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
see the result.

### 6. Browser playground

**Status: planned.** See [plan-web-playground.md](plan-web-playground.md). It is
the biggest reach multiplier: nothing to install, and `park.html` becomes live.

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

## Deferred

Ranges and patterns in examples, decimals, a non-programmer editing UI,
multi-user history, a REST API, and embedding Park as a library. Revisit
these once the showcase has an audience.
