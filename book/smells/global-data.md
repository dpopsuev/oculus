# Global Data

Package-level variables accessed from multiple packages. Global data creates invisible coupling: any package can read or write it, making dependencies implicit rather than explicit.

## Structural Signals

- Exported `var` (not `const`) at package level.
- Multiple packages referencing the same package-level variable.
- Package-level variable modified outside its declaring package.
- Absence of constructor injection for values consumed by multiple packages.
- Test files that manipulate package-level vars to set up test state.

## Differential Diagnosis

- **Constants**: Package-level `const` values are immutable and safe. The smell applies to `var` only.
- **sync.Once initialization**: A variable set once via `sync.Once` and only read afterward is a controlled singleton, not the same risk as freely mutable global data.
- **Registry pattern**: A package that maintains a registry (e.g., `database/sql` driver registration) is a deliberate design choice with known trade-offs, not an accidental smell.

## Context

- Global data violates DIP: consumers depend on a concrete package-level variable rather than an abstraction injected at construction time.
- In Go, package-level variables initialized in `init()` and never mutated are common and generally acceptable.
- The risk scales with the number of writers. Read-only globals are low risk; read-write globals accessed from multiple goroutines are high risk.
- Remedy: move the data into a struct, pass it via constructors, or use functional options to inject configuration.

## Sources

- Fowler, M. *Refactoring* 2nd ed. (2018) — Global Data
