# Mutable Data

Global mutable state shared across packages without protection. Data that can be changed by any caller at any time creates invisible coupling and race conditions.

## Structural Signals

- Package-level `var` (not `const`) mutated from multiple packages.
- Exported mutable slices or maps at package scope.
- No synchronization primitive (mutex, atomic, sync.Once) guarding the mutation.
- Multiple goroutines writing to the same package-level variable.
- Setter functions that modify package-level state without locking.

## Differential Diagnosis

- **Config loaded once at startup**: A package-level var initialized in `init()` or `main()` and never modified afterward is safe. Immutable-after-init is not mutable data.
- **Global Data**: Mutable Data is a subset of Global Data. Global Data becomes Mutable Data when writes happen after initialization.
- **Singleton**: A `sync.Once`-guarded singleton that exposes only read methods is safe. The smell applies when the singleton's state is mutated during normal operation.

## Context

- Mutable global state is the most common source of data races in Go.
- The `-race` detector catches concurrent writes at runtime, but only on exercised code paths.
- Remedy: pass dependencies explicitly, use constructor injection, or encapsulate state behind a struct with synchronized access.
- Config values should be loaded once and passed as immutable structs rather than kept in writable package-level variables.

## Sources

- Fowler, M. *Refactoring* 2nd ed. (2018) — Mutable Data
