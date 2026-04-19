# Mediator Pattern

Defines an object that encapsulates how a set of components interact. Components communicate through the mediator instead of directly, reducing pairwise coupling to a star topology.

## Structural Signals

- Fan-out > 10 (coordinates many components).
- Fan-in <= 3 (few entry points trigger the mediator).
- Average LOC per symbol < 30 (thin orchestration, not business logic).
- Methods that call multiple collaborators in sequence without complex branching.
- Components that reference the mediator but not each other.

## Differential Diagnosis

- **God Component**: High fan-out AND contains business logic. The mediator routes and coordinates; the god component computes. Check LOC per method: mediator methods are short.
- **Facade**: Provides simplified access to a subsystem. Facade is about API simplification; Mediator is about interaction management.
- **Event Bus**: Mediator knows the interaction protocol explicitly. An event bus is a generic, protocol-agnostic mediator.
- **Orchestrator**: An orchestrator in a workflow engine is a specialized mediator with sequencing and error handling.

## Context

- Mediators centralize interaction logic, making the flow easy to trace but creating a single point of complexity.
- A mediator that grows business logic is becoming a god component. Monitor LOC growth.
- In Go, a mediator is often the "app" or "server" struct that holds all dependencies and wires handlers.
- Trade-off: mediator reduces N*(N-1)/2 pairwise couplings to N, but the mediator itself becomes coupled to everything.

## Sources

- Gamma, E. et al. *Design Patterns* (1994) — Mediator
- Freeman, E. et al. *Head First Design Patterns* (2004)
