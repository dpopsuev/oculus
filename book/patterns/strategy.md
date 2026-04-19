# Strategy Pattern

Defines a family of interchangeable algorithms, encapsulates each one, and makes them substitutable. The client selects or receives an algorithm at runtime without knowing its internals.

## Structural Signals

- Interface with 1-2 methods (narrow, focused contract).
- Two or more concrete implementations of the same interface.
- Implementations stored in struct fields, function parameters, or slices/maps.
- Factory or configuration code that selects which implementation to inject.
- No type switches on the strategy — the interface handles dispatch.

## Differential Diagnosis

- **Template Method**: Similar intent but uses inheritance (embedding in Go) with hook methods. Strategy uses composition.
- **State Pattern**: Same structure but the implementations represent states that transition. Strategy implementations are stateless alternatives.
- **Simple polymorphism**: A single interface with implementations is not automatically Strategy. Strategy implies the choice is made at runtime based on context or configuration.

## Context

- Strategy is the OCP in action: adding a new algorithm means adding a new implementation, not modifying existing code.
- In Go, strategies are often function types (`type HashFunc func([]byte) []byte`) rather than interfaces when the contract is a single method.
- Strategies stored in slices enable pipeline/chain patterns (e.g., middleware).
- Over-applying Strategy for algorithms that will never vary adds unnecessary indirection.

## Sources

- Gamma, E. et al. *Design Patterns* (1994) — Strategy
- Kerievsky, J. *Refactoring to Patterns* (2004) — Replace Conditional with Strategy
