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
