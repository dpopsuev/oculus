# Churn

The frequency and magnitude of changes to a component over time. Raw churn is not inherently bad — it must be normalized and classified to produce actionable signals.

## Structural Signals

- **Raw churn**: Total commits or lines changed. Meaningless without normalization.
- **Normalized churn**: churn/LOC (change density), churn/months (velocity).
- **Additive churn**: New features, growing functionality. Normal during active development.
- **Corrective churn**: Rework, bug fixes to existing logic. Signals instability or unclear requirements.
- High churn + high fan-in = shotgun surgery. Every change ripples to many dependents.
- High churn + low test coverage = reliability risk.

## Differential Diagnosis

- **Active development**: New packages churn heavily during initial build-out. Not a smell.
- **Configuration files**: Frequently updated but structurally simple. Filter from analysis.
- **Refactoring wave**: Temporary spike in churn across many files. Distinguish from chronic churn.

## Context

- Churn is a trailing indicator. It tells you what already happened, not what will happen.
- Pair churn with coupling: a high-churn leaf component is low risk; a high-churn hub is high risk.
- Corrective churn ratio (fixes / total changes) above 40% suggests design problems.
- Churn hotspots that overlap with coupling hotspots are the highest-priority targets.

## Sources

- Tornhill, A. *Your Code as a Crime Scene* (2015) — change frequency analysis
- Nagappan, N. et al. "Using Software Dependencies and Churn Metrics to Predict Field Failures" (2007)
