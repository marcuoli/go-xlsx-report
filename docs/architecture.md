# Architecture

- `zipio` streams archive members using the standard library and repairs genuine ZIP64 local headers for the tracked `Create` path. The destination file remains caller-owned; inherited archive/zip methods retain their ordinary contracts.
- `cell` preserves full literal values and checks the 32,767 UTF-16-unit capacity before a backend writes them.
- `layout` measures Unicode display widths and wraps using locale-independent glyph rules.
- `table` projects caller-owned headers/rows through a small backend. Undeclared values remain literal text. Explicit NumericCells payloads opt body cells into finite, precision-bounded numbers through an optional NumberWriter; dates, application schemas and styles remain outside this contract. Every backend mutation observes cancellation before and after the call.
- `validation` checks generated literal XLSX packages before publication: exact sheet order, identity, rows, declared numeric versus literal text cell types/values, XML and ZIP integrity, resource bounds, and prohibited formulas/external relationships. It is not a general workbook importer, macro reader, or a complete OOXML schema implementation. Default compressed/expanded limits are 32 MiB/128 MiB with 2,048 parts.
- `publication` validates a restricted sibling stage, synchronizes/closes it, and uses the released atomic-file package. The installed result remains distinct from cleanup failure.

Applications provide concrete writer adapters, styles, locale text, field meanings, evidence ownership and missing-data policy. Caller-owned snapshots must remain immutable through rendering and saved validation. No backend dependency is forced into consumers; independently qualify the official backend version and the actual supported APIs. Unsupported values fail publication instead of being truncated or silently changed.
