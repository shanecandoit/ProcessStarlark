# Park

**Build a function by writing down examples. Add code when you're ready.**

A function turns an input into an output. In most programming languages, you
have to write the code before the function does anything. In Park, you can start
with examples: a list of inputs and the answers you expect. That list already
works as a function. Later, you or someone else can write code that handles the
inputs you didn't list.

## How it works: Roman numerals

**1. Write down answers you know.** Each example pairs a Roman numeral with its
value:

```yaml
- {in: "ix", out: 9}
- {in: "vi", out: 6}
```

**2. Ask Park a question.** Ask about `ix`, and Park looks up your example and
answers `9`. Ask about `xiv`, and Park says it doesn't know yet. It never
guesses.

**3. Teach it a rule.** Write a short function in
[Starlark](https://github.com/bazelbuild/starlark), a simplified version of
Python:

```python
Process RomanParse {
    def parse(text):
        values = {"i": 1, "v": 5, "x": 10}
        total = 0
        previous = 0
        for index in range(len(text) - 1, -1, -1):   # read right to left
            value = values[text[index]]
            total += -value if value < previous else value   # "iv" means 5 - 1
            previous = value
        return total
}
```

`Process RomanParse` is a stateless namespace for organizing functions. Think
of it as a class with just a name and functions: no class features or shared
state. Variables belong inside functions, and callers use
`RomanParse.parse(text)`.

`Process` is an optional extension to Starlark. Regular `.star` files with
top-level functions work too, with examples and catalog entries supplied by
their YAML sidecars. Both forms use Park's pure-function runtime.

Now `xiv` gives `14`. Your examples are still in charge:

- When an input matches an example, Park answers with the example, even after
  the code exists.
- `park test` checks that the code gives the same answers as your examples.
- Park won't run the code until you've given it at least two examples, so a rule
  can't run until you've shown it some correct answers.

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
./park run --input '"ix"' RomanParse.parse              # 9, from an example
./park run --input '"xiv"' RomanParse.parse             # 14, from the code
./park run --input '"ix + vi"' RomanAdd.eval            # 15, from an example
./park run --input '"iv + i"' RomanAdd.eval             # 5, from the code
```

Then open `park.html` in this folder to see every function and its examples.
The root Roman demo includes two examples for each implementation. A separate
copy in `fixtures/roman/` is available for tests and experimentation; select it
with `--root fixtures/roman` before the command.

## Learn more

- [Why Park?](docs/why-park.md): how Park compares with other languages.
- [Reference](docs/reference.md): file formats, commands, and the exact rules.
- [Sample project ideas](docs/ideas-sample-projs.md) and the
  [implementation plan](plan.md).

**Status:** early. The command-line tool works end to end, and its details may
still change.
