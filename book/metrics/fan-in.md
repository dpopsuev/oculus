# Fan-In (Afferent Coupling)

The number of components that depend on this component. High fan-in means many consumers rely on this code, making changes expensive and risky. Martin's Ca metric.

## Structural Signals

| Range | Interpretation |
|-------|---------------|
| 0-2   | Leaf component, no dependents |
| 3-7   | Normal, healthy reuse |
| 8-15  | Hub or shared-types package |
| 16+   | God candidate — change ripple radius is extreme |

- Look for fan-in concentrated on a single struct or function vs spread across a package.
- High fan-in on an interface is healthy (abstraction point).
- High fan-in on a concrete struct with logic is a risk multiplier.

## Differential Diagnosis

- **Shared types package**: Fan-in 20+ is expected and healthy when the package contains only type definitions and no logic.
- **Stdlib or utility**: Logging, errors, and config packages naturally accumulate high fan-in.

## Context

- High fan-in + high stability (I near 0) + high abstraction = ideal. This is the Stable Abstractions Principle.
- High fan-in + concrete logic + high churn = shotgun surgery risk.
- Fan-in = 0 on a non-main, non-test package = dead code candidate.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — Afferent Coupling (Ca)
- Martin, R. C. "Stability" (1997) — OO Design Quality Metrics
