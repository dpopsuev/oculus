# Single Responsibility Principle (SRP)

A component should have one, and only one, reason to change. "Reason to change" means one stakeholder or business concern. When a component serves multiple masters, changes for one concern risk breaking another.

## Structural Signals

- High LOC + high fan-out = multiple responsibilities pulling in different directions.
- Fan-out targets spanning 3+ architectural domains (e.g., database, HTTP, business logic) = SRP violation.
- High LCOM: disjoint clusters of methods operating on different field subsets.
- Multiple unrelated public APIs exported from a single package.
- File names that use "and" or "manager" or "service" without qualification.

## Differential Diagnosis

- **Facade**: High fan-out but single responsibility (unified access). Delegates only, contains no logic.
- **Orchestrator**: Coordinates multiple concerns but its single responsibility IS coordination.
- **Types package**: Contains many types but its responsibility is type definitions only.

## Context

- SRP is about cohesion at the responsibility level, not the code level. A 500-line file with one responsibility is better than five 100-line files with tangled responsibilities.
- The question is not "does it do one thing?" but "does it change for one reason?"
- Over-applying SRP creates the opposite problem: lazy classes and shotgun surgery.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — The Single Responsibility Principle
- Martin, R. C. *Agile Software Development* (2002) — SRP chapter
