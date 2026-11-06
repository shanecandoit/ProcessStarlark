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
