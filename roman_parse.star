Process RomanParse:
    """Convert lowercase Roman numerals using i, v, and x."""

    example: parse("ix") == 9
    example: parse("vi") == 6
    example: tokenize("ix + vi") == ["ix", "+", "vi"]
    example: tokenize("vi + ix") == ["vi", "+", "ix"]

    def parse(text):
        """Parse a numeral; this small fixture does not validate Roman grammar."""
        digits = {"i": 1, "v": 5, "x": 10}
        value = 0
        previous = 0
        for index in range(len(text) - 1, -1, -1):  # read right to left
            digit = digits[text[index]]
            value += -digit if digit < previous else digit  # "iv" means 5 - 1
            previous = digit
        return value

    def tokenize(text):
        """An example-only token vocabulary for teaching calls."""
        result = []
        tok = ""
        for ch in text.strip():
            if ch in " \t\r\n":
                if len(tok):
                    result.append(tok)
                    tok=""
                elif ch in "ivxcml":
                    tok = tok + ch
            else:
                if len(tok):
                    result.append(tok)
                    tok=""
        if len(tok):
            result.append(tok)
        return result
