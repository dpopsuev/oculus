# Shotgun Surgery

A single logical change requires modifications across many files or components. The responsibility is scattered, so every change sprays edits across the codebase.

## Structural Signals

- High churn correlation across multiple files (files that always change together).
- High fan-in on a concrete component (many dependents that must adapt to changes).
- A single feature addition touching 5+ files in different packages.
- Corrective churn appearing in the same file set repeatedly.

## Differential Diagnosis

- **Divergent Change**: The opposite smell. One component changes for many different reasons (too many responsibilities in one place). Shotgun surgery is one reason causing changes in many places.
- **Cross-cutting concern**: Changes to logging format or error handling naturally touch many files. This is expected, not shotgun surgery.
- **Refactoring wave**: A planned rename or API change touches many files once. Not a recurring smell.

## Context

- Shotgun surgery is the coupling smell. The responsibility exists but is spread thin.
- Remedy: consolidate the scattered logic into a single component or behind a single interface.
- Often appears after premature decomposition — splitting too early before boundaries stabilize.
- High churn + high fan-in is the quantitative proxy for this smell.

## Sources

- Fowler, M. *Refactoring* (2018) — Shotgun Surgery
- Tornhill, A. *Your Code as a Crime Scene* (2015) — change coupling analysis
