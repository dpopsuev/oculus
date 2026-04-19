# Instability

Martin's Instability metric: I = Ce / (Ca + Ce). Measures the component's susceptibility to change. Ranges from 0.0 (maximally stable) to 1.0 (maximally unstable).

## Structural Signals

| Value | Interpretation |
|-------|---------------|
| I = 0.0 | Fully stable — everything depends on it, it depends on nothing. Hard to change. |
| I = 0.0-0.3 | Stable — should be abstract (interfaces, contracts). |
| I = 0.3-0.7 | Balanced — typical business logic. |
| I = 0.7-1.0 | Unstable — easy to change, few dependents. |
| I = 1.0 | Fully unstable — depends on others, nothing depends on it. |

- When Ca + Ce = 0, I is undefined (isolated component).
- Instability alone is neutral. It becomes a signal when combined with abstractness.

## Differential Diagnosis

- **Stable + concrete** (I near 0, A near 0): Zone of Pain. Hard to change AND hard to extend.
- **Unstable + abstract** (I near 1, A near 1): Zone of Uselessness. Nobody depends on it AND it's all interfaces.

## Context

- Dependencies should flow toward stability: unstable components depend on stable ones.
- Violating this rule means a hard-to-change component depends on an easy-to-change one — breakage flows upward.
- The Stable Dependencies Principle: I of a dependency should be <= I of the depender.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — The Stable Dependencies Principle
- Martin, R. C. "Stability" (1997) — Instability metric
