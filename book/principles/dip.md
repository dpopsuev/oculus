# Dependency Inversion Principle (DIP)

High-level modules should not depend on low-level modules. Both should depend on abstractions. Import direction should point inward: domain code never imports infrastructure code directly.

## Structural Signals

- Domain package imports adapter or infrastructure package (e.g., `domain` imports `postgres`, `http`).
- Business logic directly instantiates external clients (database connections, HTTP clients).
- Import arrows in the dependency graph pointing outward from core to periphery.
- No interfaces defined in the domain layer for infrastructure capabilities.
- Test files that require running infrastructure to test business logic.

## Differential Diagnosis

- **Composition root**: The main/cmd package legitimately imports both domain and infrastructure to wire them together. This is not a DIP violation.
- **Convenience import**: Importing a shared types package that happens to live in an infrastructure directory is a packaging issue, not a DIP violation.
- **Standard library**: Importing `net/http` in a handler package is expected. DIP applies to your own architectural boundaries.

## Context

- DIP is the structural foundation of Hexagonal Architecture (Ports and Adapters).
- Remedy: define an interface (port) in the domain package, implement it in the infrastructure package, inject at the composition root.
- In Go, DIP is enforced through import direction: inner packages never import outer packages.
- DIP violations make domain logic untestable without infrastructure, slow test suites, and create deployment coupling.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — The Dependency Inversion Principle
- Cockburn, A. "Hexagonal Architecture" (2005) — Ports and Adapters
