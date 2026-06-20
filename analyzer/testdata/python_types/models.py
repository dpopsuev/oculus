class Animal:
    def __init__(self, name: str):
        self.name = name

    def speak(self) -> str:
        return ""


class Dog(Animal):
    def speak(self) -> str:
        return "Woof"

    def fetch(self, item: str):
        pass


class GuideDog(Dog):
    def guide(self, destination: str):
        pass


class Cat(Animal):
    def speak(self) -> str:
        return "Meow"

    def purr(self):
        pass


class _Internal:
    def helper(self):
        pass


class MultiParent(Cat, Dog):
    pass
