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
