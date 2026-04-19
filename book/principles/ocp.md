# Open-Closed Principle (OCP)

Software entities should be open for extension but closed for modification. Adding new behavior should not require changing existing code. Achieved through abstraction: new variants implement existing interfaces.

## Structural Signals

- Type switches or if-else chains with > 5 cases that grow every time a new type is added.
- Functions with a `kind` or `type` string parameter that dispatches behavior.
- Repeated modifications to the same switch/conditional across multiple commits.
- Churn concentrated on dispatch logic rather than on individual implementations.

## Differential Diagnosis

- **Small enums**: A type switch with 2-3 stable cases (e.g., HTTP methods) is not an OCP violation. The key is whether the set grows.
- **Configuration dispatch**: Routing based on config keys is often simpler than full polymorphism. Pragmatism applies.
- **Performance-critical paths**: Interface dispatch has overhead. In hot loops, a switch may be intentional.

## Context

- OCP violations show up as additive churn in dispatch files: every new feature modifies the router.
- Remedy: extract an interface, make each case a concrete implementation, use a registry or factory.
- In Go, OCP is achieved through interface satisfaction, not inheritance.
- Over-applying OCP creates unnecessary abstraction. Apply when the set of variants is demonstrably growing.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — The Open-Closed Principle
- Meyer, B. *Object-Oriented Software Construction* (1988) — original formulation
