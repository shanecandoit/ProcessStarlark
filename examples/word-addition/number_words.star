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
