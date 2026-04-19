# Facade Pattern

Provides a unified, simplified interface to a complex subsystem. The facade delegates to subsystem components without adding business logic of its own. It reduces coupling between clients and the subsystem.

## Structural Signals

- High fan-out (delegates to many subsystem components, typically 5+).
- Low fan-in (few clients use the facade — often just one entry point layer).
- Thin methods: low LOC per symbol, typically < 10 lines per method.
- Low total LOC relative to fan-out. A facade with fan-out 8 but only 120 LOC is delegating, not computing.
- Methods that call one subsystem method and return the result with minimal transformation.

## Differential Diagnosis

- **God Component**: High fan-out AND high LOC AND contains business logic. The key distinction is whether the component delegates or computes.
- **Mediator**: Also has high fan-out but coordinates interactions between components. Facade provides access; Mediator manages flow.
- **Service Layer**: May look like a facade but adds transaction management, authorization, and orchestration logic on top of delegation.

## Context

- Facades are healthy when they simplify a genuinely complex subsystem for external consumers.
- A facade that grows logic over time is transitioning into a god component — monitor LOC growth.
- In Go, a facade is often a struct with injected dependencies and short methods that call them.
- Facades at package boundaries reduce the public API surface of the subsystem.

## Sources

- Gamma, E. et al. *Design Patterns* (1994) — Facade
- Freeman, E. et al. *Head First Design Patterns* (2004)
