# Hexagonal Architecture Diagnostic

How to verify hexagonal (ports-and-adapters) architecture from structural signals alone. The diagnostic checks whether domain logic is isolated from infrastructure and whether boundaries are enforced through import direction.

## Structural Signals

- **Domain layer purity**: Domain packages should have zero imports of adapter or infrastructure packages. Any domain-to-adapter edge is a boundary violation.
- **Port detection**: A package where > 50% of exported types are interfaces is likely a port layer. Ports define capabilities the domain needs without specifying implementations.
- **Adapter detection**: A package with direct edges to external dependencies (database drivers, HTTP clients, message brokers) is an adapter. Adapters implement port interfaces.
- **Entry point identification**: A package with high fan-out, low fan-in, and a `main` symbol is the composition root. It wires ports to adapters.
- **Boundary violation**: A domain package importing an adapter package is a DIP violation. Import arrows must point inward: adapter imports domain, never the reverse.

## Diagnostic Checklist

1. Identify candidate domain packages: high business logic density, no external dependency edges.
2. Identify candidate adapter packages: external dependency edges, implements interfaces from domain/port packages.
3. Verify import direction: every edge from adapter to domain, no edge from domain to adapter.
4. Check port coverage: each external capability used by the domain has a corresponding interface in the domain or port layer.
5. Verify composition root: one package (usually `main` or `cmd`) wires everything together with high fan-out.

## Context

- Hexagonal architecture is DIP applied systematically across the entire application boundary.
- In Go, import direction is the primary enforcement mechanism. The compiler prevents circular imports, which naturally supports inward-pointing dependencies.
- A partial hexagonal architecture (some adapters behind ports, some not) is common and still valuable. The diagnostic identifies which boundaries are enforced and which are not.
- Fan-in and fan-out metrics at the package level are the primary quantitative inputs to this diagnostic.

## Sources

- Cockburn, A. "Hexagonal Architecture" (2005) — Ports and Adapters
- Martin, R. C. *Clean Architecture* (2017) — The Dependency Rule
