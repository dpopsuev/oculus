# Interface Segregation Principle (ISP)

No client should be forced to depend on methods it does not use. Fat interfaces create unnecessary coupling: a change to an unused method still forces recompilation and potential breakage for all implementors.

## Structural Signals

- Interface with > 5 methods.
- Implementors using < 50% of the interface's methods (rest are no-ops or panics).
- Many implementors of the same interface, each using a different subset.
- Fat interface + many implementors = high change cost (every method addition touches all implementors).
- Clients that cast the interface to a concrete type to access specific methods.

## Differential Diagnosis

- **Intentionally wide interface**: `io.ReadWriteCloser` composes narrow interfaces. The composition itself is ISP-compliant.
- **Internal interface**: An interface used by a single package with a single implementor has no segregation pressure.
- **Domain aggregate**: Some domain objects legitimately expose many methods. The question is whether all clients need all methods.

## Context

- In Go, interfaces are implicitly satisfied. ISP is achieved by defining small interfaces at the consumer site.
- Remedy: split the fat interface into role interfaces (Reader, Writer, Closer). Clients depend only on the role they need.
- ISP violations often co-occur with LSP violations: implementors that can't fulfill the whole interface start panicking or no-oping.
- The "interface in the consumer package" idiom is Go's natural ISP mechanism.

## Sources

- Martin, R. C. *Clean Architecture* (2017) — The Interface Segregation Principle
- Martin, R. C. *Agile Software Development* (2002) — ISP chapter
