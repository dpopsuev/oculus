# Coupling Diagnosis

Martin's coupling metrics framework for quantifying how tightly components are connected. These metrics turn subjective "too coupled" judgments into measurable values with known interpretation zones.

## Metrics

- **Afferent coupling (Ca / fan-in)**: Number of incoming dependencies. High Ca means many other components depend on this one. The component carries high responsibility and is costly to change.
- **Efferent coupling (Ce / fan-out)**: Number of outgoing dependencies. High Ce means this component depends on many others. It is fragile and sensitive to changes elsewhere.
- **Instability I = Ce / (Ca + Ce)**: Ranges from 0 (maximally stable, everything depends on it, it depends on nothing) to 1 (maximally unstable, it depends on everything, nothing depends on it). Stable components should be abstract; unstable components should be concrete.
- **Abstractness A = abstract_types / total_types**: Ratio of interfaces and abstract types to total exported types. Ranges from 0 (fully concrete) to 1 (fully abstract).
- **Distance D = |A + I - 1|**: Distance from the Main Sequence. Ideally 0. Measures how well a component balances stability with abstractness.

## Interpretation Zones

- **Zone of Pain (I=0, A=0)**: Stable and concrete. Many things depend on it, but it cannot be extended without modification. Hard to change, hard to extend. Examples: utility packages without interfaces, concrete data structures used everywhere.
- **Zone of Uselessness (I=1, A=1)**: Unstable and abstract. Interfaces that nothing implements or depends on. Dead abstractions. Candidate for removal.
- **Main Sequence (A + I = 1)**: The ideal balance. Components on or near this line have appropriate abstractness for their stability level.

## Context

- These metrics apply at the package or module level, not at the function level.
- A component in the Zone of Pain is not necessarily bad if it changes rarely (e.g., standard library types). The risk materializes only when change is needed.
- Instability should increase as you move outward from the domain core to the periphery. If a leaf package has I=0, it is over-stabilized.
- Distance from Main Sequence is the single best summary metric for coupling health.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — Chapter 14: Component Coupling
- Martin, R. C. *Agile Software Development* (2002) — Stability metrics
