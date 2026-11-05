"""Convert lowercase Roman numerals using i, v, and x."""

def parse(text):
    """Parse a numeral; this small fixture does not validate Roman grammar."""
    values = {"i": 1, "v": 5, "x": 10}
    total = 0
    previous = 0
    for index in range(len(text) - 1, -1, -1):
        value = values[text[index]]
        total += -value if value < previous else value
        previous = value
    return total
