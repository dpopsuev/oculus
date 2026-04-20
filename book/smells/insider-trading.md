# Insider Trading

Modules exchanging data through back channels instead of defined interfaces. Rather than declaring dependencies explicitly, components reach behind each other's boundaries to share information covertly.

## Structural Signals

- Accessing unexported fields or methods via `reflect` package.
- Shared mutable state in a third package used as a communication channel between two otherwise unrelated packages.
- Data passed through `context.Value` to avoid threading parameters through function signatures.
- Type assertions to concrete types across package boundaries instead of using interfaces.
- Build tags or link-time variable injection (`-ldflags -X`) used to swap behavior that should be injected.

## Differential Diagnosis

- **Middleware context values**: HTTP middleware storing request-scoped values (request ID, auth token) in context is an accepted Go pattern, not insider trading.
- **Internal packages**: Go's `internal/` directory restricts access by design. Using `internal` types within their allowed scope is not a back channel.
- **Shared domain types**: Two packages importing a common types package through normal imports is explicit dependency, not insider trading.

## Context

- Insider trading makes dependency graphs lie: the import graph shows no connection, but the runtime behavior is coupled.
- The smell often emerges when developers want to avoid circular imports and choose a workaround that hides the dependency rather than restructuring.
- Remedy: introduce an explicit interface (port) in the consumer package and have the provider implement it, or restructure packages to allow direct import.
- `context.Value` is particularly insidious because the compiler cannot check that the expected key exists.

## Sources

- Fowler, M. *Refactoring* 2nd ed. (2018) — Insider Trading
