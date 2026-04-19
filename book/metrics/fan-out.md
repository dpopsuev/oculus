# Fan-Out (Efferent Coupling)

The number of components this component depends on. High fan-out means the component knows about many other parts of the system, making it fragile to upstream changes. Martin's Ce metric.

## Structural Signals

| Range | Interpretation |
|-------|---------------|
| 0-1   | Leaf component, self-contained |
| 2-5   | Normal, bounded knowledge |
| 6-10  | Orchestrator or coordinator |
| 11+   | God component or facade candidate |

- Count distinct imported packages, not individual symbols.
- Distinguish between domain imports and infrastructure imports.
- Fan-out to interfaces is cheaper than fan-out to concrete types.

## Differential Diagnosis

- **Facade**: High fan-out but thin delegation, low LOC per method. Healthy pattern.
- **Entry point / main**: Wiring packages naturally have high fan-out. Expected at composition root.
- **God component**: High fan-out AND high LOC AND contains logic. Unhealthy.

## Context

- High fan-out + low fan-in = unstable component (I near 1). Fine if it's a leaf.
- High fan-out + high fan-in = change amplifier. Highest risk combination.
- Fan-out targets spanning 3+ architectural layers = SRP violation signal.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — Efferent Coupling (Ce)
- Martin, R. C. "Stability" (1997) — OO Design Quality Metrics
