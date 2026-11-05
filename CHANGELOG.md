# Changelog

## v0.2.0

- Add explicit precision-bounded numeric cell payloads and an optional backend contract, retaining literal text defaults.
- Validate saved numeric types and decimal equivalence, reject undeclared coercion and retain cancellation/error/publication checks.

## v0.1.0

- PR #2: add functional packages for literal cells/tables, Unicode layout, exact saved-package validation, streamed ZIP64 output and validated atomic publication.
- Preserve precise strings, formula-looking values, cancellation and failed-publication evidence. Include corruption, archive extent, escape-envelope and interoperability regression coverage.
