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

## Build and check the project

Install Go from `go.mod` and Node.js 22.13+ (22.x) or 24+, including npm, then run:

```sh
sh build.sh
```

This formats Go, runs Go tests/vet and native builds, then installs the pinned
JavaScript development tools, runs ESLint and JavaScript tests, builds WASM,
bundles the site, replays guided tours, and checks the compiled playground.
The final smoke check starts the actual WASM bridge, loads/checks all bundled
samples, and exercises answering a shipping question and rerunning its quote.
Success leaves `web/dist/` ready for a static server:

```sh
python3 -m http.server -d web/dist 8000
```

`sh web/build.sh` runs the same playground checks/build without repeating the
native Go checks. Both build scripts work when invoked from another directory.
The first build needs npm registry access; `npm ci` installs exactly the lockfile
without dependency lifecycle scripts. Lint tools stay in `web/.tools` (outside
Go's package scan) and out of the shipped site.
For faster iteration after installing dependencies, use `npm --prefix web/.tools run lint`
and `npm --prefix web/.tools test`. ESLint uses its
[recommended JavaScript rules](https://eslint.org/docs/latest/use/getting-started#configuration),
with explicit browser, worker, and Node environments. HTML and CSS are not linted;
the smoke check verifies local asset references and the engine. The separate
Chromium suite exercises browser interactions and phone-width question flows;
visual review and sessions with unfamiliar users remain necessary.

Real-browser workflow tests also run in CI before publishing. After building,
install Chromium and run them locally with:

```sh
web/.tools/node_modules/.bin/playwright install chromium --only-shell
npm --prefix web/.tools run test:browser
```

## Try an isolated draft

The local SaaS prototype lets you edit an isolated copy of the shipping
project, retain questions and history, check rules, review a diff, and export
it. Start it from the repository root:

```sh
go run ./cmd/park-drafts --root examples/shipping --db .park/shipping.sqlite
```

In another terminal, run `python3 scripts/draft_walkthrough.py`. Follow the
[draft guide](docs/saas-usage.md) for the question queue, source formatting,
restart recovery, and database backup. SQLite is CGo-free, persists drafts and
activity, and tracks SQL migrations with checksums. Local pinned reviews, release
activation, and rollback are implemented; hosted accounts remain planned.
Try the [cache and release guide](docs/cache-releases.md), including
`python3 scripts/draft_walkthrough.py --workflow release` against a fresh database.

To use the playground with explicit draft saves and
SHA-256 source versions with parent links:

```sh
sh web/build.sh
go run ./cmd/park-drafts --root examples/word-addition --db .park/words.sqlite --web-dir web/dist
```

Open `http://127.0.0.1:8081` and expand **Drafts and source versions**. Save,
inspect an earlier version, or edit it in a new draft. See the
[source version plan](docs/plan-source-versions.md) and
[durable draft plan](docs/plan-durable-drafts.md). **Show code** offers safe
formatting and basic lint. Saved-source questions can be answered or dismissed
in the optional draft panel, with dates and local user attribution.

Expand **Review, compare, and release** to inspect pinned checks/source changes,
approve or reject a review, evaluate live behavior, compare named scenarios,
and roll back. Start with the [playground review guide](docs/playground-review-guide.md).
The question flow keeps the original request visible as successive missing
facts are answered and shows when those saved facts are reused. The
[usability session protocol](docs/usability-sessions.md) is ready; participant
sessions and findings are still pending.

## Learn more

- [Park for business teams](docs/for-business.md): the idea without code,
  and how it differs from the tools business teams use today.
- [Why Park?](docs/why-park.md): how Park compares with other languages.
- [Reference](docs/reference.md): file formats, commands, and the exact rules.
- [Design decisions](docs/design.md): what was decided and why.
- [Sample projects](docs/ideas-sample-projs.md): word-number addition, a game,
  a business tool, geometry, and cache identity, each runnable from
  [`examples/`](examples).
- [Implementation plan](plan.md): the full contract.
- [Near-term product plan](docs/plan-saas.md#near-term-plan): playground reviews
  and releases, behavior comparison, question-flow usability, and measured
  retention/performance; authentication and hosted operations follow later.
- [Caching and reviewed releases](docs/cache-releases.md): cache identity, pinned
  checks, local approval/activation, release evaluation, and rollback.
- [Draft usage guide](docs/saas-usage.md): run the local API and a tested
  shipping walkthrough, review changes, and export your project.
- [Browser playground](docs/plan-web-playground.md): how the playground is
  built, and what's next for it. [Guided tours](docs/plan-guided-tours.md) are
  available for addition and shipping.
- [Treemap visualizer plan](docs/plan-treemap-visualizer.md): an open design
  exploration for explaining answers with nested rectangles.
- [Direction](docs/ideas-direction.md): where Park goes next.

**Status:** early. The command-line tool and the browser playground work end to
end, and their details may still change.
