# Sample projects

Four small Park projects for different kinds of users. Each one is a folder in
[`examples/`](../examples) that passes `park test`; a test keeps the code on
this page identical to those files. Each also has a place where
an unknown input stops Park, and someone answers it once to teach it.

| Project | For | Shows |
|---|---|---|
| [Number-word addition](#number-words-addition) (`examples/word-addition`) | anyone trying Park | vocabulary as examples, composition as code, one new row reused |
| [Robot on a grid](#game-robot-on-a-grid) (`examples/robot`) | people learning to code | state in, state out; a vocabulary that grows |
| [Shipping quotes](#business-shipping-quotes) (`examples/shipping`) | operations and the developers they work with | a rule table owned by non-programmers, code owned by developers |
| [Shapes on a grid](#geometry-shapes-on-a-grid) (`examples/shapes`) | teachers and students | examples as worked answers, checked against code |

To try one, `cd` into its folder, run `park build` to generate `park.html`,
then try the commands shown. Answering questions edits the example's files;
`git checkout examples` resets them. See the [reference](reference.md) for the
format.

## Number words: addition

The default project evaluates `"twenty-two plus eleven"` as `33`. Word meanings
are authored answers: eleven means 11 because a row says so. Structural rules
combine those meanings. There is no code synthesis or statistical inference.

The grammar is intentionally small: one English number from 0 to 99, the word
"plus," then another number from 0 to 99. Compounds use one hyphen, as in
`twenty-two`. Capitals and extra whitespace around "plus" are ignored. The
sum may exceed 99. Digits, negatives, hundreds, "and," other operators, and
unhyphenated compounds are outside this sample's grammar.

`word_values.star` is the vocabulary table. It stays example-only. Twelve is
deliberately missing to make the question-and-answer flow easy to try:

```python
Process WordValues:
    """Number words and their meanings. Each row is an answer, not a parsing rule."""

    example: value("zero") == 0
    example: value("one") == 1
    example: value("two") == 2
    example: value("three") == 3
    example: value("four") == 4
    example: value("five") == 5
    example: value("six") == 6
    example: value("seven") == 7
    example: value("eight") == 8
    example: value("nine") == 9
    example: value("ten") == 10
    example: value("eleven") == 11
    # Twelve is deliberately missing: answer Park's question to add it.
    example: value("thirteen") == 13
    example: value("fourteen") == 14
    example: value("fifteen") == 15
    example: value("sixteen") == 16
    example: value("seventeen") == 17
    example: value("eighteen") == 18
    example: value("nineteen") == 19
    example: value("twenty") == 20
    example: value("thirty") == 30
    example: value("forty") == 40
    example: value("fifty") == 50
    example: value("sixty") == 60
    example: value("seventy") == 70
    example: value("eighty") == 80
    example: value("ninety") == 90

    def value(word):
        """Example-only vocabulary. An unfamiliar word becomes a question."""
        pass
```

`number_words.star` supplies the structural rule. Only a tens value from
20 through 90 followed by a unit from 1 through 9 can form a compound;
`eleven-two` and `twenty-zero` are errors. An unfamiliar word in an otherwise
valid position becomes a question for the vocabulary table.

```python
Process NumberWords:
    """Read an English number from 0 to 99, using a table of word meanings."""

    example: parse("one") == 1
    example: parse("thirty-three") == 33

    def parse(text):
        """A single word, or a hyphenated tens word followed by a unit."""
        parts = text.strip().lower().split("-")
        if len(parts) > 2:
            fail("expected a word or a tens-units number such as twenty-two")
        for part in parts:
            if not part or not part.isalpha():
                fail("expected number words joined by a single hyphen")
        if len(parts) == 1:
            value = WordValues.value(parts[0])
        else:
            tens = WordValues.value(parts[0])
            if type(tens) != "int" or tens < 20 or tens > 90 or tens % 10 != 0:
                fail("the first word must mean twenty, thirty, ..., or ninety")
            units = WordValues.value(parts[1])
            if type(units) != "int" or units < 1 or units > 9:
                fail("the second word must mean one through nine")
            value = tens + units
        if type(value) != "int" or value < 0 or value > 99:
            fail("expected a whole number from 0 to 99")
        return value
```

`word_add.star` supplies the addition rule. Its two examples check known
expressions; the default expression is deliberately different so a normal
run executes the code and exposes its nested calls.

```python
Process WordAdd:
    """Add two English numbers, as in twenty-two plus eleven."""

    example: eval("one plus two") == 3
    example: eval("twenty plus zero") == 20

    def eval(text):
        """Two numbers separated by plus; ignore capitals and extra whitespace."""
        words = text.lower().split()
        if len(words) != 3 or words[1] != "plus":
            fail("expected two number words separated by plus, such as twenty-two plus eleven")
        left = NumberWords.parse(words[0])
        right = NumberWords.parse(words[2])
        total = left + right
        return total
```

Try it from `examples/word-addition/`, or from the repository root, which
starts with the same three files:

```sh
park test
park run 'WordValues.value("eleven")'                    # 11, from an example
park run --why 'WordAdd.eval("twenty-two plus eleven")'  # 33, composed by code
park run 'WordAdd.eval("twenty-two plus twelve")'        # unresolved: WordValues.value(word="twelve")
park example add 'WordValues.value("twelve") == 12'
park run 'WordAdd.eval("twenty-two plus twelve")'        # 34
park run 'WordAdd.eval("twelve plus two")'               # 14: the same new row is reused
park run 'WordAdd.eval("ninety-nine plus one")'          # 100
park test
```

In the browser, use the default Word-number addition sample or the
[addition tour](https://shanecandoit.github.io/ProcessStarlark/#tour=addition).
Answering a question reruns the last expression automatically; the CLI
requires the explicit rerun shown above.

A whole-expression example would answer that exact call directly, skipping
its calculation at runtime. `park test` checks the implementation separately,
bypassing the checked function's own examples. Example-only vocabulary rows
are reported as data, not as implementation checks. Two examples make code
ready under Park's current policy; they do not prove correctness for every
possible input.

## Game: robot on a grid

A robot moves on a 5 × 5 grid. Nothing is stored between moves: each call takes
a position and returns a new one, so the same moves always end in the same
place.

`directions.star` is example-only. It's the game's vocabulary:

```python
Process Directions:
    """Words people use for directions, mapped to compass points."""

    example: canonical("north") == "north"
    example: canonical("south") == "south"
    example: canonical("east") == "east"
    example: canonical("west") == "west"
    example: canonical("up") == "north"
    example: canonical("right") == "east"

    def canonical(word):
        """Example-only: a vocabulary that grows as players use new words."""
        pass
```

`robot.star`:

```python
Process Robot:
    """Move a robot on a 5 x 5 grid. A position goes in; a new one comes out."""

    example: step([0, 0], "east") == [1, 0]
    example: step([4, 0], "east") == [4, 0]  # walls stop the robot
    example: walk([0, 2], ["up", "right", "right"]) == [2, 1]
    example: walk([3, 3], []) == [3, 3]

    def step(position, direction):
        """Move one square. North is up, so it decreases y."""
        offsets = {"north": [0, -1], "south": [0, 1], "east": [1, 0], "west": [-1, 0]}
        offset = offsets[Directions.canonical(direction)]
        x = position[0] + offset[0]
        y = position[1] + offset[1]
        moved = [x, y] if 0 <= x and x < 5 and 0 <= y and y < 5 else position
        return moved

    def walk(position, moves):
        """Apply each move in order."""
        final = position
        for move in moves:
            final = step(final, move)
        return final
```

Try it:

```sh
park run 'Robot.walk([0, 0], ["east", "south", "south"])'      # [1, 2]
park run 'Robot.walk([0, 0], ["right", "down"])'
# Park doesn't know "down", so it queues the question Directions.canonical("down").
park example add 'Directions.canonical("down") == "south"'
park run 'Robot.walk([0, 0], ["right", "down"])'               # [1, 1]
```

A player's new word becomes a new line of vocabulary, not a code change.

## Business: shipping quotes

A developer writes the pricing rule once. The operations team keeps the table
of countries and zones, which is just examples, and never touches the code.

`zones.star` is the team's table:

```python
Process Zones:
    """Which shipping zone each destination country belongs to."""

    example: zone("US") == "domestic"
    example: zone("CA") == "north-america"
    example: zone("MX") == "north-america"
    example: zone("FR") == "international"

    def zone(country):
        """Example-only: the operations team keeps this table."""
        pass
```

`shipping.star` is the developer's rule. Prices are in cents because Park has
no floats:

```python
Process Shipping:
    """Quote shipping in cents from a destination country and a weight in grams."""

    example: quote("US", 500) == 599
    example: quote("CA", 1200) == 2199
    example: quote("FR", 2500) == 3999

    def quote(country, grams):
        """A base fee per zone, plus a fee for each started 500 g."""
        base = {"domestic": 499, "north-america": 1299, "international": 1999}
        per_step = {"domestic": 100, "north-america": 300, "international": 400}
        zone = Zones.zone(country)
        steps = (grams + 499) // 500
        cents = base[zone] + per_step[zone] * steps
        return cents
```

Try it:

```sh
park run 'Shipping.quote("MX", 3000)'                 # 3099
park run 'Shipping.quote("DE", 800)'                  # unresolved: Zones.zone(country="DE")
park example add 'Zones.zone("DE") == "international"'
park run 'Shipping.quote("DE", 800)'                  # 2799
```

A new country is a new example, not a code change, and Park never guesses a
zone. `park.html` shows the zone table to anyone.

Negotiated one-off prices can override a runtime call through an example, but
`park test` still flags the disagreement. Strict checks are intentional: the
implementation must handle the negotiated price too for tests to pass. See
[D17](design.md#d17-every-example-is-a-strict-implementation-check).

## Geometry: shapes on a grid

Examples double as worked answers. A student or teacher writes down answers
they know, then writes the code, and `park test` checks every answer against
it. Coordinates and lengths are whole numbers.

`shapes.star`:

```python
Process Shapes:
    """Shapes with whole-number coordinates and side lengths."""

    example: area([0, 0], [4, 3]) == 12
    example: area([3, 5], [1, 1]) == 8
    example: contains([0, 0], [4, 3], [2, 2]) == True
    example: contains([0, 0], [4, 3], [5, 1]) == False
    example: triangle(3, 4, 5) == "scalene"
    example: triangle(2, 2, 3) == "isosceles"
    example: triangle(1, 1, 5) == "impossible"
    example: polygon(3) == "triangle"
    example: polygon(4) == "quadrilateral"
    example: polygon(5) == "pentagon"

    def area(corner, opposite):
        """Area of a rectangle with sides along the axes, from two opposite corners."""
        width = opposite[0] - corner[0]
        height = opposite[1] - corner[1]
        size = width * height
        if size < 0:
            size = -size
        return size

    def contains(corner, opposite, point):
        """Whether a point lies inside the rectangle or on its edge."""
        inside = True
        for axis in [0, 1]:
            low = min(corner[axis], opposite[axis])
            high = max(corner[axis], opposite[axis])
            if point[axis] < low or point[axis] > high:
                inside = False
        return inside

    def triangle(a, b, c):
        """Classify a triangle by its side lengths."""
        sides = sorted([a, b, c])
        if sides[0] + sides[1] <= sides[2]:
            kind = "impossible"
        elif sides[0] == sides[2]:
            kind = "equilateral"
        elif sides[0] == sides[1] or sides[1] == sides[2]:
            kind = "isosceles"
        else:
            kind = "scalene"
        return kind

    def polygon(sides):
        """Example-only: names of polygons by number of sides."""
        pass
```

Try it:

```sh
park run 'Shapes.triangle(5, 5, 5)'                   # "equilateral", from the code
park run 'Shapes.area([2, 2], [7, 4])'                # 10
park run 'Shapes.polygon(6)'                          # queued as a question
park example add 'Shapes.polygon(6) == "hexagon"'     # answers it
park test
```

For a classroom exercise, hand out only the examples and `pass` bodies.
Students replace each `pass` with code until `park test` passes.
