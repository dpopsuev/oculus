# Distance from the Main Sequence

D = |A + I - 1|, where A is abstractness and I is instability. Measures how well a component balances stability with abstraction. D near 0 is ideal — the component sits on the Main Sequence.

## Structural Signals

| D Value | Interpretation |
|---------|---------------|
| D near 0 | Balanced: stability matches abstraction level |
| D = 0.0-0.2 | Healthy range |
| D = 0.2-0.5 | Worth reviewing |
| D > 0.5 | Strong signal of imbalance |

- **Zone of Pain** (I near 0, A near 0): Stable AND concrete. Many things depend on it, but it exposes no abstractions. Painful to change, painful to extend. Examples: concrete utility libraries baked into everything.
- **Zone of Uselessness** (I near 1, A near 1): Unstable AND abstract. All interfaces, no dependents. Nobody uses it. Dead abstraction.

## Differential Diagnosis

- **Healthy stable concrete**: Small, simple, rarely changing components (constants, enums) can sit in the Zone of Pain without actual pain. Size and churn matter.
- **Intentionally abstract leaf**: An interface package with no dependents yet may be in the Zone of Uselessness temporarily during development.

## Context

- D is a portfolio metric — plot all components to see systemic patterns.
- Individual outliers matter less than clusters in the zones.
- The Main Sequence is a guideline. Some components legitimately live off it.
- Abstractness (A) = ratio of abstract types (interfaces) to total types in the component.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — The Main Sequence
- Martin, R. C. "OO Design Quality Metrics" (1994) — Distance metric
