# Park

**Build a function by writing down examples. Add code when you're ready.**

A function turns an input into an output. In most programming languages, you
have to write the code before the function does anything. In Park, you can start
with examples: a list of inputs and the answers you expect. That list already
works as a function. Later, you or someone else can write code that handles the
inputs you didn't list.

## How it works: Roman numerals

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

- **You can start before you know how to code.** Examples are enough to begin.
- **It says "I don't know" instead of guessing,** and you can answer by adding
  a new example.
- **Every function shows its examples.** Park builds a web page, `park.html`,
  listing each function with real inputs and outputs.

Read more in [Why Park?](docs/why-park.md).

## Try it

You need [Go](https://go.dev/dl/) 1.26 or newer, on macOS or Linux. In this
folder, run:

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
<http://localhost:8000>. The playground runs the same engine, compiled to
WebAssembly: edit the samples, run calls, and answer Park's questions inline.
The root Roman demo is two files, `roman_parse.star` and `roman_add.star`, each
holding its code and examples. A separate copy in `fixtures/roman/` is
available for tests and experimentation; select it with `--root fixtures/roman`
before the command.

## Learn more

- [Why Park?](docs/why-park.md): how Park compares with other languages.
- [Reference](docs/reference.md): file formats, commands, and the exact rules.
- [Design decisions](docs/design.md): what was decided and why.
- [Sample projects](docs/ideas-sample-projs.md): a game, a business tool, and
  geometry, each runnable from [`examples/`](examples).
- [Implementation plan](plan.md): the full contract.
- [Browser playground](docs/plan-web-playground.md): how the playground is
  built, and what's next for it.
- [Direction](docs/ideas-direction.md): where Park goes next.

**Status:** early. The command-line tool and the browser playground work end to
end, and their details may still change.
