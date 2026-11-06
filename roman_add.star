Process RomanAdd:
    """Add two lowercase Roman numerals separated by ' + '."""

    example: eval("ix + vi") == 15
    example: eval("vi + ix") == 15
    example: eval("ii  +  x") == 12

    # Examples can name any function in the project. This one counts toward
    # RomanParse.parse exactly as if it were written in roman_parse.star.
    example: RomanParse.parse("xii") == 12

    def eval(text):
        """Return the sum as an integer, calling RomanParse for each term."""
        terms = text.split(" + ")
        if len(terms) != 2:
            fail("expected two Roman numerals separated by ' + '")
        total = RomanParse.parse(terms[0]) + RomanParse.parse(terms[1])
        return total
