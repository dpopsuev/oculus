# LOC Thresholds

Lines of code as a complexity proxy. Not a quality metric on its own, but a reliable triage signal when combined with coupling metrics. Raw LOC without context produces false positives.

## Structural Signals

| Scope | Suspect | Split |
|-------|---------|-------|
| Function | >40 lines | >100 lines |
| File | >500 lines | >1000 lines |
| Package | >2000 lines | Context-dependent |

- Count logical lines (exclude blanks and comments) for comparison.
- Functions over 40 lines correlate with cyclomatic complexity >10.
- Files over 500 lines often contain multiple responsibilities.

## Differential Diagnosis

- **Types-only package**: Can be 1000+ LOC of struct definitions with no smell. No logic = no complexity.
- **Generated code**: Codegen output (protobuf, ORMs) is large by nature. Exclude from analysis.
- **Table-driven tests**: Test files with large test tables are verbose but not complex.

## Context

- LOC is a screening metric, not a diagnosis. Always pair with fan-out and cohesion.
- Small LOC + high fan-out = thin facade or mediator (healthy pattern).
- Large LOC + high fan-out + high fan-in = god component (unhealthy).
- Language matters: 40 lines of Go is different from 40 lines of Java.

## Sources

- Fowler, M. *Refactoring* (2018) — Long Method, Large Class
- Herzig, K. & Nagappan, N. "The Impact of Code Complexity" (2013) — empirical LOC/defect correlation
