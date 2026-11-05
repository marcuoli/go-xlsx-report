# go-xlsx-report

Reusable literal-table rendering, Unicode layout, saved XLSX validation, ZIP/ZIP64 output and atomic publication. Applications supply schemas, messages and concrete workbook adapters.

Use functional imports under `cell/`, `layout/`, `table/`, `validation/`, `publication/` and `zipio/`. The module has no dependency on a concrete workbook library and preserves exact numeric strings and formula-looking text as literal values.

```go
installed, err := publication.Publish(ctx, output, overwrite, saveWorkbook, sheets)
```

A saver writes to the caller-owned staging file without closing it. Publication validates every saved literal before installation. Inspect both `installed` and `err`; a cleanup error may follow installation. Create the output directory explicitly and keep the evidence snapshot when publication fails.

Go 1.27.3 is required. Run `scripts/validate.ps1 -Race -Scan` or `scripts/validate.sh --race --scan` using approved external storage. The runnable `examples/archive` consumer takes an explicit output file and demonstrates the generic ZIP sink. Read [architecture](docs/architecture.md), [contribution/comment rules](CONTRIBUTING.md) and [code management](docs/code-management.md).

License: UPL 1.0.
