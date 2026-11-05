"""Add two lowercase Roman numerals separated by ' + '."""

def eval(text):
    """Return the sum as an integer, calling RomanParse for each term."""
    terms = text.split(" + ")
    if len(terms) != 2:
        fail("expected two Roman numerals separated by ' + '")
    return RomanParse.parse(terms[0]) + RomanParse.parse(terms[1])
