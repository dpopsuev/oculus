# God Component

A component that has accumulated too many responsibilities, becoming a central point of coupling and complexity. It knows too much, does too much, and everything depends on it or it depends on everything.

## Structural Signals

- LOC > 1000 (logic, not types)
- Fan-out > 6 (knows about too many other components)
- Fan-in > 5 AND fan-out > 5 (central coupling hub)
- Symbol count > 30 (too many exported functions/types)
- LCOM high (methods operate on disjoint field sets)
- Multiple domains touched in a single package

## Differential Diagnosis

- **Facade**: High fan-out but thin delegation. Methods are short (< 10 LOC), total LOC is low relative to fan-out. Facade is a healthy pattern.
- **Mediator**: High fan-out but coordinates, doesn't contain logic. Average LOC per symbol < 30.
- **Shared types package**: High fan-in but contains only type definitions, no logic. LOC may be high but complexity is low.
- **Entry point / composition root**: High fan-out is expected at the wiring layer (main, cmd).

## Context

- Types-only packages with high fan-in are healthy, not god components.
- A component with high fan-out but low LOC-per-method is likely a facade or mediator.
- The combination of high LOC + high fan-out + high fan-in is the strongest god signal.
- God components often emerge from convenience: one package that was easy to add to.

## Sources

- Fowler, M. *Refactoring* (2018) — Large Class, Long Method
- Riel, A. *Object-Oriented Design Heuristics* (1996) — God Class
