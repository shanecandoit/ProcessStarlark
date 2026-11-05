# Idea: what goes into and out of a function

Status: open question. This page compares options; nothing here changes v1 yet.

## Where we are today

Every Park function takes exactly **one value** and returns **one value**. A
value can be null, a boolean, a string, an integer, a list, or a string-keyed
map. The engine enforces this: a dispatched call must have exactly one
positional argument and no keywords, and an implementation must have exactly
one parameter.

A function that needs several inputs takes a map:

```yaml
- function: add(request) sum
  examples:
    - {request: {left: 9, right: 6}, sum: 15}
```

```python
def add(request):
    return request["left"] + request["right"]

# a caller writes:
Math.add({"left": 9, "right": 6})
```

The input value, encoded as canonical JSON, is the lookup key for authored
examples and for the cache.

## Does canonical JSON make the choice matter?

**Not much for the machine, a lot for people.** Canonical JSON can turn any
input shape into one stable key: a single value, a list of positional
arguments, or a map of named arguments. Sorted keys mean argument order
doesn't change a named key. Caching, example lookup, `park test`, determinism,
and purity work the same way for every option below.

So the decision is mostly about what people read and type:

- the example rows in YAML,
- the call sites in Starlark,
- the `park run` command line,
- the rows in `park.html`.

## Options

### A. One input value (current)

`parse(text) value`, `add(request) sum`

Easier for the tech:
- One key, one parameter, one label. This is the smallest rule set to
  implement and explain to a programmer.
- No arity checks, no keyword handling, no argument-order questions.

Harder for people:
- Multi-input functions add a wrapper layer: `{request: {left: 9, right: 6}}`.
  The made-up name `request` is noise the author has to invent.
- Call sites build maps by hand: `Math.add({"left": 9, "right": 6})`.
- On the command line the whole map is one quoted JSON blob.
- Two styles appear in the same project: flat rows for one-input functions and
  nested rows for the rest.

### B. Several positional inputs

`add(left, right) sum`, called as `Math.add(9, 6)`

Easier for people:
- Example rows are flat: `{left: 9, right: 6, sum: 15}`.
- Call sites look like ordinary Python.

Harder for the tech, and some for people:
- The key is an ordered list, so reordering parameters silently changes every
  key. Examples written as named fields have to be mapped to positions.
- Defaults, `*args`, and keyword calls need rules or bans.
- Callers must remember argument order.

### C. Named inputs: each example row is a table row

`add(left, right) sum`. The parameter names are the input column names.

```yaml
- function: add(left, right) sum
  examples:
    - {left: 9, right: 6, sum: 15}
    - {left: 1, right: 1, sum: 2}

- function: parse(text) value
  examples:
    - {text: "ix", value: 9}
```

```python
def add(left, right):
    return left + right

Math.add(left=9, right=6)   # by name
Math.add(9, 6)              # by position, mapped through the def's order
```

The key is the canonical JSON map of input fields, for example
`{"left":9,"right":6}`. A one-input function is the one-column case. Its rows
already look exactly like today's YAML.

Easier for people:
- **A function is a table.** Input columns, one output column. Anyone who has
  used a spreadsheet already knows this model, and `park.html` can render it
  as an actual table.
- Rows are flat and use the same names as the code, so there is one name per
  value (matching the "labels must match parameters" rule).
- No wrapper names to invent.
- The command line can take `name=value` pairs:
  `park run Math.add left=9 right=6`.

Easier for the tech:
- Sorted map keys make the key independent of argument order, so reordering
  parameters in the `def` changes nothing.
- Positional calls are just a translation step through the `def`'s parameter
  list, done before lookup.
- Validation is simple: each row must contain exactly the input names plus the
  output name.

Costs:
- Changing an implemented function's arity or renaming a parameter changes
  every key. Its examples need the same edit. (This is true in every option,
  but here it's visible as a column rename.)
- The output name must differ from every input name.
- Defaults and optional inputs: simplest to ban in v1. Every row lists every
  input. Explicit `null` stays distinct from a missing field.
- A zero-input function would have only one possible key (`{}`), so it can never
  have two distinct examples. Either ban it, or treat it as a constant that
  needs no gate.
- `name=value` on the command line needs a rule for types. One option: parse
  the value as JSON if it's valid JSON, otherwise treat it as a string. Then
  `left=9` is an integer and `text=ix` is a string. The catch is `text=true` or
  `text=9`, which would need quotes (`text='"9"'`).

### Several outputs?

Keep **one output** in every option. Starlark returns one value anyway, and a
function that needs several results can return a map: `{quotient: 3,
remainder: 1}`. Several output columns would make "which column is the answer"
ambiguous in the table model and complicate `park test` without much gain.

## The quadrant

| | Easy for people | Hard for people |
|---|---|---|
| **Easy for the tech** | **C: named inputs, one output** | A: one wrapped value |
| **Hard for the tech** | B with full Python calls (defaults, `*args`, kwargs) | Several outputs, optional inputs |

Option C sits in the top-left because the hard part of named arguments,
ordering, is exactly what canonical JSON's sorted keys already solve. The
engine change is small: build a map from the call's arguments, then look it up
as today.

## Determinism and purity

These don't depend on the choice. Every option keeps pure functions, the
restricted builtins, the step limits, and byte-stable keys. Canonical JSON is
what makes the keys stable, and it works the same for one value or a map of
named values.

## If we choose C: what changes

- Signature: `name(a, b, ...) output`. Input names must equal the `def`'s
  parameters, in any order. An example-only function declares its own names.
- Engine: accept positional or keyword calls, check names and count, build the
  input map, then key and dispatch as today. Call the body with keywords.
- Registration: copy parameter names from the `def`. Ask for (or require) an
  output name rather than writing `out`.
- YAML rows: unchanged for one-input functions. Multi-input rows lose their
  wrapper.
- Cache: keys for one-input functions change from `"ix"` to `{"text":"ix"}`.
  The cache is disposable, so a revision bump is enough.
- CLI: add `name=value` operands. Keep `--input JSON` as the exact form.
- Catalog: render each function's examples as a table with one column per name.

## Open questions

- Should one-input functions keep the bare value as their key, for shorter
  cache records? This saves little and adds a special case. Recommendation: no.
- Allow defaults later? If so, the key should include the default value so
  `add(left=1)` and `add(left=1, right=0)` share one example.
- Should the "two distinct examples" gate scale with the number of inputs?
