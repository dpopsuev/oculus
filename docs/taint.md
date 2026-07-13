# Optional CPG / taint federation

Locus is a **structural walker**: architecture, coupling, symbol-graph call edges.
It does **not** claim sound program-dependence / CPG taint analysis.

## Built-in heuristic (`engine: "heuristic"`)

`analysis action=taint` with `from`/`to` (source/sink) runs a BFS over symbol-graph
**call** edges (and `data_flow` when present). Response includes a disclaimer.
Treat hits as navigation hints, not proof.

## Federation (`engine: "federated"`)

Set `LOCUS_TAINT_CMD` to a shell template. Placeholders:

| Token | Meaning |
|-------|---------|
| `{source}` | source symbol |
| `{sink}` | sink symbol |
| `{path}` | project root |

Example:

```bash
export LOCUS_TAINT_CMD='joern-taint --src {source} --sink {sink} --project {path}'
```

Stdout is attached as `federated`. Use Joern, CodeQL, or any wrapper — Locus does
not vendor those tools. CI stays green without them.
