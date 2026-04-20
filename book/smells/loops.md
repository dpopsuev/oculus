# Loops

Imperative loops that could be replaced with declarative pipelines or iterators. The loop body mixes iteration mechanics with business logic, making intent harder to see.

## Structural Signals

- Nested `for` loops with manual index manipulation (`i++`, `j--`).
- Manual accumulation into a pre-declared slice or map inside a loop.
- Filter-then-collect patterns implemented with `if` inside `for` and `append`.
- Break/continue logic that obscures the loop's purpose.
- Loop body exceeding 15 lines of logic.

## Differential Diagnosis

- **Performance-critical path**: Explicit loops with pre-allocated buffers are appropriate in hot paths where allocation matters. Profile first.
- **Simple range loop**: A single-level `for range` with a clear body (< 5 lines) is idiomatic Go and not a smell.
- **Iterator protocol**: Using `iter.Seq` or `iter.Seq2` (Go 1.23+) to yield values is the pipeline approach, not the smell.

## Context

- Go does not have LINQ or Java Streams, but the `slices` and `maps` packages (Go 1.21+) cover filter, sort, compact, and contains.
- This is a low-priority smell. Readability is the primary concern, not a structural defect.
- Refactoring: extract the loop body into a named function, or use `slices.ContainsFunc`, `slices.SortFunc`, `slices.Collect` where applicable.
- Deeply nested loops (3+ levels) are a stronger signal than simple two-level iterations.

## Sources

- Fowler, M. *Refactoring* 2nd ed. (2018) — Loops
