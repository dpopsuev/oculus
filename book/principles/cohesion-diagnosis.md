# Cohesion Diagnosis

How to assess whether a component's internals belong together. Cohesion measures the degree to which elements within a module serve a single, unified purpose.

## Metrics and Principles

- **LCOM (Lack of Cohesion of Methods)**: Count methods that share fields versus methods that do not. High LCOM means disjoint clusters of methods operating on separate field subsets. Each cluster is a candidate for extraction into its own type.
- **Common Closure Principle (CCP)**: Classes that change together belong together. If modifying feature X always requires editing files A and B but never C, then A and B should be in the same component and C should not.
- **Common Reuse Principle (CRP)**: Classes used together belong together. If a consumer imports a package but uses only 2 of its 10 types, the package may be too broad.
- **Reuse/Release Equivalence Principle (REP)**: The unit of reuse is the unit of release. If parts of a package are versioned or released independently, they should be separate packages.

## Diagnostic Signals

- Methods on a struct that do not share ANY field access with other methods = strong extraction candidate.
- A package where fewer than 50% of type pairs have a usage relationship = low cohesion.
- Git co-change analysis: files that consistently change together in commits belong together. Files in the same package that never change together may be mis-grouped.
- High fan-in to only a subset of a package's exports = the package bundles unrelated capabilities.

## Context

- Cohesion and coupling are two sides of the same coin. High cohesion within components leads to low coupling between components.
- LCOM is most useful for structs with 5+ methods. Small types with 1-2 methods are trivially cohesive.
- Git co-change validates structural cohesion with behavioral evidence. Structural analysis says what could be split; co-change analysis says what actually changes together.
- Over-splitting reduces cohesion at the package level and increases coupling between packages. The goal is balanced grouping, not maximum granularity.

## Sources

- Chidamber, S. & Kemerer, C. "A Metrics Suite for Object Oriented Design" (1994) — LCOM
- Martin, R. C. *Clean Architecture* (2017) — Component Cohesion principles (CCP, CRP, REP)
