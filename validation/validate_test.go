package validation_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcuoli/go-xlsx-report/internal/testbook"
	"github.com/marcuoli/go-xlsx-report/table"
	"github.com/marcuoli/go-xlsx-report/validation"
)

func TestSavedLiteralPrecisionAndCorruption(t *testing.T) {
	sheets := []table.Sheet{{Name: "Summary", Headers: []string{"Name", "Value"}, Rows: [][]string{{"integer", "9007199254740993"}, {"formula-looking", "=1+1"}, {"Unicode", "ação 東京 😀"}, {"decimal", "1234567890123456.789"}}}}
	for _, test := range []struct {
		name string
		edit func(map[string]string)
	}{
		{"intact", nil},
		{"foreign sheets namespace", func(parts map[string]string) {
			parts["xl/workbook.xml"] = strings.Replace(parts["xl/workbook.xml"], "<sheets>", `<sheets xmlns="urn:foreign">`, 1)
		}},
		{"foreign worksheet content type namespace", func(parts map[string]string) {
			parts["[Content_Types].xml"] = strings.Replace(parts["[Content_Types].xml"], `<Override PartName="/xl/worksheets/`, `<Override xmlns="urn:foreign" PartName="/xl/worksheets/`, 1)
		}},
		{"unqualified relationship id", func(parts map[string]string) {
			parts["xl/workbook.xml"] = strings.ReplaceAll(parts["xl/workbook.xml"], "r:id=", "id=")
		}},
		{"wrong worksheet root", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.ReplaceAll(parts["xl/worksheets/sheet1.xml"], "worksheet", "notWorksheet")
		}},
		{"wrong worksheet namespace", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.ReplaceAll(parts["xl/worksheets/sheet1.xml"], "http://schemas.openxmlformats.org/spreadsheetml/2006/main", "urn:wrong")
		}},
		{"missing worksheet type", func(parts map[string]string) {
			parts["[Content_Types].xml"] = strings.ReplaceAll(parts["[Content_Types].xml"], "spreadsheetml.worksheet+xml", "spreadsheetml.other+xml")
		}},
		{"malformed style XML", func(parts map[string]string) {
			parts["xl/styles.xml"] = `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><broken>`
		}},
		{"duplicated text value", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], `<t xml:space="preserve">9007199254740993</t>`, `<t>wrong</t><t>9007199254740993</t>`, 1)
		}},
		{"missing content types", func(parts map[string]string) { delete(parts, "[Content_Types].xml") }},
		{"missing root relation", func(parts map[string]string) { delete(parts, "_rels/.rels") }},
		{"value changed", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], "9007199254740993", "9007199254740992", 1)
		}},
		{"formula injected", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], `<c r="B3" t="inlineStr">`, `<c r="B3" t="inlineStr"><f>1+1</f>`, 1)
		}},
		{"row lost", func(parts map[string]string) {
			value := parts["xl/worksheets/sheet1.xml"]
			first := strings.Index(value, `<row r="5">`)
			last := strings.Index(value[first:], `</row>`) + first + len(`</row>`)
			parts["xl/worksheets/sheet1.xml"] = value[:first] + value[last:]
		}},
		{"external relation", func(parts map[string]string) {
			parts["xl/_rels/workbook.xml.rels"] = strings.Replace(parts["xl/_rels/workbook.xml.rels"], `Target="worksheets/sheet1.xml"`, `Target="https://example.invalid/remote" TargetMode="External"`, 1)
		}},
		{"missing workbook", func(parts map[string]string) { delete(parts, "xl/workbook.xml") }},
		{"trailing XML", func(parts map[string]string) { parts["xl/workbook.xml"] += `<second/>` }},
		{"numeric coercion", func(parts map[string]string) {
			parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], `<c r="B2" t="inlineStr"><is><t xml:space="preserve">9007199254740993</t></is></c>`, `<c r="B2"><v>9007199254740993</v></c>`, 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			parts := testbook.Parts(sheets)
			if test.edit != nil {
				test.edit(parts)
			}
			path := filepath.Join(t.TempDir(), "report.xlsx")
			testbook.Write(t, path, parts)
			err := validation.Validate(context.Background(), path, sheets)
			if test.edit == nil && err != nil {
				t.Fatal(err)
			}
			if test.edit != nil && err == nil {
				t.Fatal("corrupt workbook admitted")
			}
		})
	}
}

func TestSpreadsheetEscapesAreDecodedOnce(t *testing.T) {
	sheets := []table.Sheet{{Name: "Summary", Headers: []string{"Value"}, Rows: [][]string{{"_x000D_"}, {"😀"}}}}
	parts := testbook.Parts(sheets)
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], "_x000D_", "_x005F_x000D_", 1)
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], "😀", "_xD83D__xDE00_", 1)
	path := filepath.Join(t.TempDir(), "report.xlsx")
	testbook.Write(t, path, parts)
	if err := validation.Validate(context.Background(), path, sheets); err != nil {
		t.Fatal(err)
	}
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], "_xD83D__xDE00_", "_xD83D_", 1)
	testbook.Write(t, path, parts)
	if err := validation.Validate(context.Background(), path, sheets); err == nil {
		t.Fatal("unpaired surrogate accepted")
	}
}

func TestCancellationAndUnsafeFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validation.Validate(ctx, "missing", nil); err == nil {
		t.Fatal("cancel ignored")
	}
	path := filepath.Join(t.TempDir(), "invalid.xlsx")
	if err := os.WriteFile(path, []byte("not a zip"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validation.Validate(context.Background(), path, nil); err == nil {
		t.Fatal("invalid package accepted")
	}
}

func TestRichTextEscapesDoNotCrossNodeBoundaries(t *testing.T) {
	sheets := []table.Sheet{{Name: "Summary", Headers: []string{"Value"}, Rows: [][]string{{"_x000D_"}}}}
	parts := testbook.Parts(sheets)
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], `<is><t xml:space="preserve">_x000D_</t></is>`, `<is><r><t>_x00</t></r><r><t>0D_</t></r></is>`, 1)
	path := filepath.Join(t.TempDir(), "report.xlsx")
	testbook.Write(t, path, parts)
	if err := validation.Validate(context.Background(), path, sheets); err != nil {
		t.Fatal(err)
	}
	sheets[0].Rows[0][0] = "\r"
	if err := validation.Validate(context.Background(), path, sheets); err == nil {
		t.Fatal("node-boundary escape changed the displayed value")
	}
}

func TestEncodedEnvelopeAllowsValidDecodedControlText(t *testing.T) {
	value := strings.Repeat("\x01", 20000)
	sheets := []table.Sheet{{Name: "Summary", Headers: []string{"Value"}, Rows: [][]string{{value}}}}
	parts := testbook.Parts(sheets)
	// Build the valid wire representation directly; XML 1.0 cannot carry the
	// decoded control byte, while SpreadsheetML carries it as seven ASCII bytes.
	parts["xl/worksheets/sheet1.xml"] = `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Value</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>` + strings.Repeat("_x0001_", 20000) + `</t></is></c></row></sheetData></worksheet>`
	path := filepath.Join(t.TempDir(), "report.xlsx")
	testbook.Write(t, path, parts)
	if err := validation.Validate(context.Background(), path, sheets); err != nil {
		t.Fatal(err)
	}
}

func TestInlineStringCannotHideValueChild(t *testing.T) {
	sheets := []table.Sheet{{Name: "Summary", Headers: []string{"Value"}, Rows: [][]string{{""}}}}
	parts := testbook.Parts(sheets)
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], `<c r="A2" t="inlineStr"><is><t xml:space="preserve"></t></is></c>`, `<c r="A2" t="inlineStr"><v>ignored</v></c>`, 1)
	path := filepath.Join(t.TempDir(), "report.xlsx")
	testbook.Write(t, path, parts)
	if err := validation.Validate(context.Background(), path, sheets); err == nil {
		t.Fatal("inline string with incompatible value accepted")
	}
}
