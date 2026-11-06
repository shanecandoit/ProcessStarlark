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

**Try it in your browser: <https://shanecandoit.github.io/ProcessStarlark/>**, with nothing to install. The shipping
sample walks through a question and its answer, and a [two-minute guided
tour](https://shanecandoit.github.io/ProcessStarlark/#tour=shipping) plays it
as the two people involved.

- **For business teams and buyers:** [Park for business teams](docs/for-business.md)
  walks through a shipping-quote example and compares Park with specs,
  spreadsheets, rules engines, and low-code tools.
- **For programmers:** read on, then see [Why Park?](docs/why-park.md).

## How it works: Roman numerals

A function turns an input into an output. In most programming languages, you
have to write the code before the function does anything. In Park, you can
start with examples: a list of inputs and the answers you expect. That list
already works as a function.

**1. Write down answers you know.** Each example is a call and its answer:

```python
Process RomanParse:
    example: parse("ix") == 9
    example: parse("vi") == 6

    def parse(text):
        pass
```

`def parse(text): pass` declares a function with no code yet. Its examples are
all it knows.

**2. Ask Park a question.** Ask about `ix`, and Park looks up your example and
answers `9`. Ask about `xiv`, and Park says it doesn't know yet. It never
guesses.

**3. Teach it a rule.** Replace `pass` with code in
[Starlark](https://github.com/bazelbuild/starlark), a simplified version of
Python:

```python
Process RomanParse:
    example: parse("ix") == 9
    example: parse("vi") == 6

    def parse(text):
        digits = {"i": 1, "v": 5, "x": 10}
        value = 0
        previous = 0
        for index in range(len(text) - 1, -1, -1):  # read right to left
            digit = digits[text[index]]
            value += -digit if digit < previous else digit  # "iv" means 5 - 1
            previous = digit
        return value
```

The names in the code are the names in the examples: `parse` takes `text` and
returns `value`, so its signature is `parse(text) value`. Every `return` hands
back a named variable for that reason.

`Process RomanParse:` is a stateless namespace for organizing functions. Think
of it as a class with just a name, functions, and examples: no class features
or shared state. Callers use `RomanParse.parse("ix")`. Plain `.star` files with
top-level functions work too; the file name supplies the process name.

Now `xiv` gives `14`. Your examples are still in charge:

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
./park run 'RomanParse.parse("ix")'                   # 9, from an example
./park run 'RomanParse.parse("xiv")'                  # 14, from the code
./park run 'RomanAdd.eval("ix + vi")'                 # 15, from an example
./park run 'RomanAdd.eval("iv + i")'                  # 5, from the code
./park run --why 'RomanAdd.eval("ix + i")'            # 10, and where each part came from
```

Then open `park.html` in this folder to see every function and its examples.

To try Park in a browser instead, run `sh serve.sh` and open
<http://localhost:8000>. The hosted copy at <https://shanecandoit.github.io/ProcessStarlark/> is rebuilt on
every push to `main` by [.github/workflows/pages.yml](.github/workflows/pages.yml). The playground runs the same engine, compiled to
WebAssembly: edit the samples, run calls, and answer Park's questions inline.
The root Roman demo is two files, `roman_parse.star` and `roman_add.star`, each
holding its code and examples. A separate copy in `fixtures/roman/` is
available for tests and experimentation; select it with `--root fixtures/roman`
before the command.

## Learn more

- [Park for business teams](docs/for-business.md): the idea without code,
  and how it differs from the tools business teams use today.
- [Why Park?](docs/why-park.md): how Park compares with other languages.
- [Reference](docs/reference.md): file formats, commands, and the exact rules.
- [Design decisions](docs/design.md): what was decided and why.
- [Sample projects](docs/ideas-sample-projs.md): a game, a business tool, and
  geometry, each runnable from [`examples/`](examples).
- [Implementation plan](plan.md): the full contract.
- [Browser playground](docs/plan-web-playground.md): how the playground is
  built, and what's next for it. [Guided tours](docs/plan-guided-tours.md) are
  planned next.
- [Direction](docs/ideas-direction.md): where Park goes next.

**Status:** early. The command-line tool and the browser playground work end to
end, and their details may still change.
