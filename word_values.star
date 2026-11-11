Process WordValues:
    """Number words and their meanings. Each row is an answer, not a parsing rule."""

    example: value("zero") == 0
    example: value("one") == 1
    example: value("two") == 2
    example: value("three") == 3
    example: value("four") == 4
    example: value("five") == 5
    example: value("six") == 6
    example: value("seven") == 7
    example: value("eight") == 8
    example: value("nine") == 9
    example: value("ten") == 10
    example: value("eleven") == 11
    # Twelve is deliberately missing: answer Park's question to add it.
    example: value("thirteen") == 13
    example: value("fourteen") == 14
    example: value("fifteen") == 15
    example: value("sixteen") == 16
    example: value("seventeen") == 17
    example: value("eighteen") == 18
    example: value("nineteen") == 19
    example: value("twenty") == 20
    example: value("thirty") == 30
    example: value("forty") == 40
    example: value("fifty") == 50
    example: value("sixty") == 60
    example: value("seventy") == 70
    example: value("eighty") == 80
    example: value("ninety") == 90

    def value(word):
        """Example-only vocabulary. An unfamiliar word becomes a question."""
        pass
