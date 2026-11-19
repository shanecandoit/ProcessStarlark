"""A cache example with a literal constant and a private helper."""
PREFIX = "hello  "

example: greet("Ada") == "hello  Ada"
example: greet("Lin") == "hello  Lin"

def greet(name):
    message = _prefix() + name
    return message

def _prefix():
    return PREFIX
