# Park

**Write down the answers you know. Code fills in the rest. Anything else, Park
asks about.**

Usually, the people who know a business's rules write requirements, and
developers turn them into code. Cases the requirements missed get guessed, and
the document and the system drift apart.

In Park, the people who know the rules write **examples** instead: "a 500 g
parcel to the US costs $5.99". The examples aren't a description of the
system; they are its answers, from the day they're written. Developers then
write code for the general rule, filling in the cases nobody listed. Code
never overrides an example, and Park checks that the two agree. When a case
comes up that neither covers, Park doesn't guess: it asks the people who own
the rule, and their answer becomes a new example.

**Try it in your browser: <https://shanecandoit.github.io/ProcessStarlark/>**, with nothing to install. The default
sample adds number words: "twenty-two plus eleven" becomes `33`. A
[short guided tour](https://shanecandoit.github.io/ProcessStarlark/#tour=addition)
shows how one new word meaning unlocks more expressions. The
[shipping tour](https://shanecandoit.github.io/ProcessStarlark/#tour=shipping)
shows the handoff between a business user and a developer.

- **For business teams and buyers:** [Park for business teams](docs/for-business.md)
  walks through a shipping-quote example and compares Park with specs,
  spreadsheets, rules engines, and low-code tools.
- **For programmers:** read on, then see [Why Park?](docs/why-park.md).

## How it works: number words and addition

A function turns an input into an output. In most programming languages, you
have to write the code before the function does anything. In Park, you can
start with examples: a list of inputs and the answers you expect. That list
already works as a function.

**1. Write down answers you know.** Each example is a call and its answer:

```python
Process WordValues:
    example: value("twenty") == 20
    example: value("two") == 2
    example: value("eleven") == 11

    def value(word):
        pass
```

`def value(word): pass` declares an example-only function. These rows already
answer calls. Eleven's meaning is vocabulary, so it can stay an example;
there is no need to invent a general rule for deriving `11` from its spelling.

**2. Use code for structure.** The runnable project has three functions:

| Function | Its job |
|---|---|
| `WordValues.value(word)` | Look up a saved word meaning, such as `"eleven" → 11`. |
| `NumberWords.parse(text)` | Read a single word or combine a valid tens-and-units form: `"twenty-two" → 20 + 2`. |
| `WordAdd.eval(text)` | Split around "plus," parse the two numbers, and add them. |

The addition rule is written in
[Starlark](https://github.com/bazelbuild/starlark), a simplified version of Python:

```python
Process WordAdd:
    example: eval("one plus two") == 3
    example: eval("twenty plus zero") == 20

    def eval(text):
        words = text.lower().split()
        if len(words) != 3 or words[1] != "plus":
            fail("expected two number words separated by plus")
        left = NumberWords.parse(words[0])
        right = NumberWords.parse(words[2])
        total = left + right
        return total
```

The names in the code are the names in the examples: `eval` takes `text` and
returns `total`, so its signature is `eval(text) total`. Every `return` hands
back a named variable for that reason.

`Process WordAdd:` is a stateless namespace for organizing functions. Think
of it as a class with just a name, functions, and examples: no class features
or shared state. Callers use `WordAdd.eval("twenty-two plus eleven")`. Plain
`.star` files with top-level functions work too; the file name supplies the
process name.

**3. Fill a gap once.** The sample deliberately leaves out twelve. Run
`WordAdd.eval("twenty-two plus twelve")` and Park asks for
`WordValues.value("twelve")`. Answer `12`, and rerun: the result is `34`.
`WordAdd.eval("twelve plus two")` now gives `14` too. One new row works
wherever the code needs that word meaning.

The small language accepts English operands from 0 to 99, with compounds
hyphenated, joined by "plus." Capitals and extra whitespace are ignored. The
sum can exceed 99. This is a deterministic parser using explicit vocabulary,
not a model inferring rules from examples. See the
[full example and commands](docs/ideas-sample-projs.md#number-words-addition).

Your examples are still in charge:

- When a call matches an example, Park answers with the example, even after
  the code exists.
- Examples can be written in any file and name any function. They all go into
  one project-wide set.
- `park test` checks that the code gives the same answers as your examples.
- Until a function has at least two examples, its code still runs, but each
  answer is *unconfirmed*: Park prints it, queues it in `questions.txt` as a
  proposed example, and you confirm the ones that are right. Write code first
  and the examples follow. `park run` doesn't run the tests for you; use
  `park test`.

If the entire expression has a saved example, a normal run returns that answer
directly; it doesn't execute the nested calculation. `park test` bypasses that
function's own examples to check its implementation. The default expression
has no whole-expression example, so `--why` shows the composition.

## Why use it?

- **The people who know the answers write them.** Examples are enough to
  start, with no code and no handoff.
- **It says "I don't know" instead of guessing,** and the answer becomes a new
  example.
- **Every answer explains itself.** `park run --why` shows which example or
  code produced each part of an answer.
- **Every function shows its examples.** Park builds a web page, `park.html`,
  listing each function with real inputs and outputs.

## Try it

You need [Go](https://go.dev/dl/) 1.26 or newer, on macOS or Linux. To install
just the CLI:

```sh
go install github.com/shanecandoit/ProcessStarlark/cmd/park@latest
```

Or, in a clone of this repository, run:

```sh
go build -o park ./cmd/park
./park build                                          # generate park.html
./park test                                           # code vs. examples
./park run 'WordValues.value("eleven")'               # 11, from an example
./park run 'NumberWords.parse("twenty-two")'          # 22, from code using examples
./park run 'WordAdd.eval("twenty-two plus eleven")'   # 33
./park run --why 'WordAdd.eval("twenty-two plus eleven")'
./park run 'WordAdd.eval("twenty-two plus twelve")'   # asks what twelve means
./park example add 'WordValues.value("twelve") == 12'
./park run 'WordAdd.eval("twenty-two plus twelve")'   # 34
./park version list                                   # show function hashes & readiness
```

Then open `park.html` in this folder to see every function and its examples.

To try Park in a browser instead, run `sh serve.sh` and open
<http://localhost:8000>. The hosted copy at <https://shanecandoit.github.io/ProcessStarlark/> is rebuilt on
every push to `main` by [.github/workflows/pages.yml](.github/workflows/pages.yml). The playground runs the same engine, compiled to
WebAssembly: try a plain expression, edit examples in tables, and answer
Park's questions beside the result. Expand the visual explanation to inspect
how an answer was made; **Show code** opens the editor. Guidance and
explanations can be collapsed, and the playground remembers those choices.
The root demo is three files: `word_values.star` for vocabulary,
`number_words.star` for number structure, and `word_add.star` for addition.
A clean copy lives in `examples/word-addition/` and is bundled as the first
playground sample and the default optional tour. Roman numerals remain available
in the sample picker and in `fixtures/roman/`; use `--root fixtures/roman`
before the CLI command to try them. Saved browser projects and tour progress
still take precedence over the first-visit default.

## Learn more

- [Park for business teams](docs/for-business.md): the idea without code,
  and how it differs from the tools business teams use today.
- [Why Park?](docs/why-park.md): how Park compares with other languages.
- [Reference](docs/reference.md): file formats, commands, and the exact rules.
- [Design decisions](docs/design.md): what was decided and why.
- [Sample projects](docs/ideas-sample-projs.md): word-number addition, a game,
  a business tool, and geometry, each runnable from [`examples/`](examples).
- [Implementation plan](plan.md): the full contract.
- [SaaS product plan](docs/plan-saas.md): draft→prod promotion with per-function versioning.
- [SaaS usage guide](docs/saas-usage.md): CLI commands for draft→production workflow.
- [Browser playground](docs/plan-web-playground.md): how the playground is
  built, and what's next for it. [Guided tours](docs/plan-guided-tours.md) are
  available for addition and shipping.
- [Treemap visualizer plan](docs/plan-treemap-visualizer.md): an open design
  exploration for explaining answers with nested rectangles.
- [Direction](docs/ideas-direction.md): where Park goes next.

**Status:** early. The command-line tool and the browser playground work end to
end, and their details may still change.
