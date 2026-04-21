# Oculus

Analysis engine for AI agents. Language-agnostic symbol resolution, call graph analysis, architecture scanning, and the Architect's Book knowledge graph.

## Axiom

**Oculus emits facts, never taste.** Raw structural signals (fan-in, fan-out, LOC, churn, cycles, edges). No scores, no severity, no heuristic pattern labels. The consuming agent applies judgment.

## 5 Symbol Primitives

| Primitive | What it does |
|-----------|-------------|
| **Probe** | All vitals for one symbol — fan-in, fan-out, instability, cross-pkg, circuits. Zero traversal. |
| **Scenario** | Trace upstream to entry points, downstream to leaves. N depth. Stress metrics. |
| **Convergence** | Where N symbols' downstream trees overlap. Gradient counts. |
| **Isolate** | Remove a symbol — what disconnects? |
| **Islands** | Find symbols unreachable from entry points (dead code). |

All primitives accept `*SymbolGraph` directly — works on real scans or synthetic graphs.

## Architect's Book

28 knowledge entries in a graph with 43 typed edges. Embedded in the binary via `embed.FS`. Query with keywords + hops → knowledge subgraph with typed relationships (violates, measured_by, confused_with, remediation, feeds, part_of, distinguished_by).

Categories: metrics (fan-in, fan-out, instability, LOC, churn, LCOM, distance-from-main-sequence), smells (9 Fowler), principles (SOLID + hexagonal + coupling + cohesion), patterns (facade, strategy, mediator, factory).

## Diagnose

One-call composite: probe a symbol → derive keywords from signals → query Book → return `DiagnoseResult{Probe, Book}`.

## LSP-Only Analysis

Call graph analysis uses LSP servers exclusively (Strangler Fig — non-LSP backends rejected). Accurate type-resolved edges, no name-collision false positives.

Required servers: gopls (Go), rust-analyzer (Rust), pyright (Python), typescript-language-server (TypeScript/JS), clangd (C/C++), jdtls (Java), kotlin-language-server (Kotlin), omnisharp (C#), sourcekit-lsp (Swift), zls (Zig).

Fail-fast: if LSP unavailable, returns error naming the required server — never silent empty data.

## Other Features

- **go-git**: zero exec.Command shell-outs, pure Go git operations
- **SymbolGraph cache**: in-memory by path@sha, instant repeat queries
- **Project Context**: XDG-based writable knowledge layer, git-aware staleness
- **Symbol-level diff**: compare two SymbolGraphs by FQN
- **Edge confidence**: Layer field on SymbolEdge (goast/treesitter/lsp/regex)

## Usage

```go
import oculus "github.com/dpopsuev/oculus/v3"

// Probe a symbol
sg := buildSymbolGraph(...)
result := oculus.Probe(sg, "arch.ScanAndBuild")

// Trace a scenario
scenario := oculus.TraceScenario(sg, "ScanAndBuild", 10, true, 0)

// Query the Book
bg, _ := book.LoadEmbedded()
knowledge := bg.Query([]string{"high fan-out", "facade"}, 2)

// One-call diagnose
diagnosis := oculus.Diagnose(sg, bg, "ScanAndBuild")
```

## License

MIT
