# Mysterious Name

Symbols whose names do not reveal intent. The reader must examine the implementation to understand what a function, type, or variable does.

## Structural Signals

- Single-letter variables beyond `i`, `j`, `k` loop indices and `t` in tests.
- Abbreviated names that drop vowels or syllables: `Cfg`, `Mgr`, `Hlpr`, `Svc`, `Ctx` (when not `context.Context`).
- Generic names: `data`, `info`, `result`, `item`, `temp`, `val`, `obj`.
- Boolean variables or functions without a clear predicate: `flag`, `check`, `ok2`.
- Names that describe implementation rather than intent: `stringMap`, `loopHelper`.

## Differential Diagnosis

- **Go convention for small scopes**: Short names like `r` for `*http.Request`, `w` for `http.ResponseWriter`, `err` for `error`, and `ctx` for `context.Context` are idiomatic in Go and not mysterious.
- **Well-known abbreviations**: Domain-standard abbreviations (e.g., `URL`, `ID`, `HTTP`, `DB`) are clear by convention.
- **Interface names**: Single-method interface names ending in `-er` (e.g., `Reader`, `Writer`) are idiomatic Go, not mysterious.

## Context

- Naming is the most frequent refactoring in practice and the cheapest to fix.
- Mysterious names compound: one unclear name forces readers to hold extra mental context, slowing comprehension of the entire function.
- The test: can a new team member understand the symbol's purpose from its name alone, without reading its body?
- Remedy: rename to express intent. `processData` becomes `validateOrder`; `Mgr` becomes `ConnectionPool`; `x` becomes `remainingAttempts`.

## Sources

- Fowler, M. *Refactoring* 2nd ed. (2018) — Mysterious Name
