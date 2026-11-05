# Sample projects

Three small Park projects for three kinds of users. Each one is a folder of
`.star` files that passes `park test` as written. Each also has a place where
an unknown input stops Park, and someone answers it once to teach it.

| Project | For | Shows |
|---|---|---|
| [Robot on a grid](#game-robot-on-a-grid) | people learning to code | state in, state out; a vocabulary that grows |
| [Shipping quotes](#business-shipping-quotes) | operations and the developers they work with | a rule table owned by non-programmers, code owned by developers |
| [Shapes on a grid](#geometry-shapes-on-a-grid) | teachers and students | examples as worked answers, checked against code |

To try one, put its files in an empty folder and run `park init`, then the
commands shown. See the [reference](reference.md) for the format.

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

Negotiated one-off prices are deliberate exceptions: the example answers the
call correctly, but `park test` flags it because the rule disagrees. See the
open questions in [Design decisions](design.md#open-questions).

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
