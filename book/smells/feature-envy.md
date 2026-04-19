# Feature Envy

A function or method that uses another component's data and behavior more than its own. It reaches across boundaries to access foreign state, suggesting the logic is misplaced.

## Structural Signals

- More than 50% of call sites target a single foreign component.
- Method accesses fields of a foreign struct more than its own receiver's fields.
- Heavy use of getters/accessors from another package.
- Import list dominated by a single external package.

## Differential Diagnosis

- **Delegation**: A thin wrapper that forwards calls is not envious — it's a facade.
- **Cross-cutting concern**: Logging, metrics, and error handling naturally touch foreign state.
- **Data transfer**: Functions that transform between representations (mappers, converters) legitimately access two domains.

## Context

- Feature envy is a placement signal: the logic exists, but it lives in the wrong component.
- Remedy options:
  - **Move function**: Relocate the method to the component it envies.
  - **Extract interface**: Define an interface in the caller's package for the accessed behavior.
  - **Introduce parameter**: Pass the needed data instead of reaching for it.
- Feature envy between packages in the same layer is a weaker signal than cross-layer envy.

## Sources

- Fowler, M. *Refactoring* (2018) — Feature Envy
- Kerievsky, J. *Refactoring to Patterns* (2004)
