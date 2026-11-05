# go-xlsx-report

Reusable literal-table rendering, Unicode layout, saved XLSX validation, ZIP/ZIP64 output and atomic publication. Applications supply schemas, messages and concrete workbook adapters.

Use functional imports under `cell/`, `layout/`, `table/`, `validation/`, `publication/` and `zipio/`. The module has no dependency on a concrete workbook library and preserves exact numeric strings and formula-looking text as literal values.

```go
installed, err := publication.Publish(ctx, output, overwrite, saveWorkbook, sheets)
```

A saver writes to the caller-owned staging file without closing it. Publication validates every saved literal before installation. Inspect both `installed` and `err`; a cleanup error may follow installation. Create the output directory explicitly and keep the evidence snapshot when publication fails.

Go 1.27.3 is required. Run `scripts/validate.ps1 -Race -Scan` or `scripts/validate.sh --race --scan` using approved external storage. The runnable `examples/archive` consumer takes an explicit output file and demonstrates the generic ZIP sink. Read [architecture](docs/architecture.md), [contribution/comment rules](CONTRIBUTING.md) and [code management](docs/code-management.md).

License: UPL 1.0.


## Explicit numeric cells

`table.Sheet.NumericCells` maps body coordinates such as `B2` to numeric payloads. The matching nonempty `Rows` text remains a display/layout hint. Headers and undeclared cells always remain literal text. A backend can opt into `cell.NumberWriter.SetCellNumber`; a numeric sheet fails before mutation when its backend lacks that optional interface. Existing text-only snapshots/backends retain their behavior.

`cell.Number` normalizes finite decimal/exponent literals only when they survive Excel's 15-significant-digit precision and normal binary64 range without decimal rounding. Larger exact counts and unsupported values should remain text. `cell.EqualNumber` admits equivalent decimal spellings in saved numeric payloads. Saved validation requires numeric type/value exactly where declared and continues rejecting formulas and undeclared numeric coercion. Styles/units and domain field meaning remain caller policy.
