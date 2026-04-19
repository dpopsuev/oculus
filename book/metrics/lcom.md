# LCOM (Lack of Cohesion of Methods)

Chidamber-Kemerer cohesion metric. Measures how related a class's methods are by counting shared field access. Low LCOM means methods work on the same data (cohesive). High LCOM means the class contains unrelated responsibilities.

## Structural Signals

| Value | Interpretation |
|-------|---------------|
| LCOM = 0 | Fully cohesive — every method pair shares at least one field |
| LCOM > 0 | Some method pairs share no fields — potential split candidate |
| LCOM high | Multiple clusters of methods using disjoint field sets — likely multiple responsibilities |

- LCOM1 (original CK): count of method pairs sharing no fields minus pairs sharing fields (floor 0).
- LCOM4 (graph-based): number of connected components in the method-field graph. LCOM4 > 1 means the class can be split.
- For Go: treat receiver methods as class methods, struct fields as instance variables.

## Differential Diagnosis

- **Utility class**: Methods intentionally unrelated (string helpers, math). High LCOM is structural, not a smell.
- **Builder pattern**: Methods chain but touch different fields during construction. Can inflate LCOM.
- **Interface adapter**: Implements a wide interface but only uses a few fields per method.

## Context

- LCOM is most useful for structs with 5+ methods. Below that, sample size is too small.
- LCOM does not account for method-to-method calls, only method-to-field access.
- Combine with fan-out: high LCOM + high fan-out = strong split signal.
- LCOM = 0 does not guarantee good design; a god class can be internally cohesive.

## Sources

- Chidamber, S. R. & Kemerer, C. F. "A Metrics Suite for Object-Oriented Design" (1994)
- Henderson-Sellers, B. *Object-Oriented Metrics: Measures of Complexity* (1996) — LCOM variants
