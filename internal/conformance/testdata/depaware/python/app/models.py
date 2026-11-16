class User:
    """A registered user."""

    def __init__(self, name: str) -> None:
        self.name = name

    def greeting(self) -> str:
        return "hello " + self.name
