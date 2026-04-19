# Factory Pattern

Creates objects without exposing instantiation logic to the client. The client requests an object by specifying criteria (type, config, name) and receives a concrete implementation behind an interface.

## Structural Signals

- Functions named `New*` that return interface types rather than concrete structs.
- Constructor functions that dispatch on a type string, enum, or configuration value.
- A dedicated `factory.go` or `registry.go` file with creation functions.
- Switch/map-based dispatch that returns different implementations of the same interface.
- Registration functions (`Register`, `Add`) that populate a type registry at init time.

## Differential Diagnosis

- **Simple constructor**: A `NewFoo()` returning `*Foo` (concrete) is just a constructor, not a factory. Factories return abstractions.
- **Builder pattern**: Builders configure a single complex object step by step. Factories select among different types.
- **Abstract Factory**: Creates families of related objects. Simple Factory creates one object at a time.
- **Dependency Injection**: DI wires dependencies at the composition root. Factory creates objects at runtime based on dynamic input.

## Context

- Factories are the DIP in action: clients depend on the interface, the factory hides which concrete type is used.
- In Go, factories are typically package-level functions (`New*`) or method factories on a registry struct.
- Factory + Strategy = runtime-selectable algorithms without client coupling to implementations.
- Over-applying Factory when there is only one implementation adds indirection without benefit. Apply when the set of implementations is > 1 or expected to grow.

## Sources

- Gamma, E. et al. *Design Patterns* (1994) — Factory Method, Abstract Factory
- Kerievsky, J. *Refactoring to Patterns* (2004) — Replace Constructor with Factory Method
