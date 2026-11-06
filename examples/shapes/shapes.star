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
