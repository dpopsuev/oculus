# Lazy Class

A component too small to justify its own existence. It adds indirection and cognitive overhead without providing meaningful abstraction or encapsulation.

## Structural Signals

- LOC < 50 (insufficient logic to warrant a separate component).
- Fan-in = 0-1 (at most one consumer).
- No callers from outside the immediate parent.
- Symbol count <= 3 (only a struct and one or two methods).
- Wraps a single dependency without adding behavior.

## Differential Diagnosis

- **Interface**: A small interface with 1-2 methods is the Interface Segregation Principle in action, not laziness.
- **Value object**: Small structs that carry domain meaning (Money, Coordinate) earn their keep through type safety.
- **Intentional isolation**: A package boundary created for future extension is speculative but not necessarily wrong if the boundary is architecturally motivated.

## Context

- Lazy classes increase navigation cost: more files to open, more packages to understand.
- Remedy: merge into the nearest neighbor by edge weight (the component it interacts with most).
- Before merging, check if the class is lazy because it's new (not yet grown) vs structurally unnecessary.
- A lazy class with fan-in = 0 and no tests is a strong dead-code signal.

## Sources

- Fowler, M. *Refactoring* (2018) — Lazy Class
- Beck, K. *Smalltalk Best Practice Patterns* (1996) — cost of indirection
