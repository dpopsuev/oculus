# Liskov Substitution Principle (LSP)

Subtypes must be substitutable for their base types without altering program correctness. If code works with a base type, it must work identically with any subtype. Violations break polymorphism.

## Structural Signals

- Interface implementations that panic or return `ErrNotImplemented` for required methods.
- Implementations that no-op on methods the contract expects to have side effects.
- Tests that special-case specific subtypes (e.g., `if impl is FooImpl, skip assertion`).
- Overrides that weaken postconditions or strengthen preconditions.
- Implementations that ignore or discard required parameters.

## Differential Diagnosis

- **Optional interface methods**: Some interfaces intentionally have optional methods (e.g., `io.Closer`). The contract explicitly allows no-op. Not an LSP violation.
- **Partial implementation during development**: Temporarily unimplemented methods are tech debt, not necessarily design violations.
- **Adapter pattern**: An adapter may legitimately translate between incompatible interfaces, narrowing behavior.

## Context

- In Go, LSP applies to interface satisfaction. Any struct implementing an interface must honor the behavioral contract, not just the type signature.
- Remedy: contract tests. Write tests against the interface, run them for every implementation.
- LSP violations often emerge when an interface is too wide (ISP violation) and implementors cannot fulfill all methods.
- The compiler catches signature violations; only tests catch behavioral violations.

## Sources

- Liskov, B. & Wing, J. "A Behavioral Notion of Subtyping" (1994)
- Martin, R. C. *Clean Architecture* (2017) — The Liskov Substitution Principle
