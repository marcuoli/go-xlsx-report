// Package testbook builds small synthetic OOXML packages for corruption checks.
package testbook

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/marcuoli/go-xlsx-report/table"
)

func escaped(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

// Parts constructs only fixture data, never a production workbook writer.
func Parts(sheets []table.Sheet) map[string]string {
	parts := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="root" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
	}
	var workbook, rels strings.Builder
	workbook.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	rels.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, sheet := range sheets {
		parts["[Content_Types].xml"] = strings.Replace(parts["[Content_Types].xml"], "</Types>", fmt.Sprintf(`<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`, i+1), 1)
		fmt.Fprintf(&workbook, `<sheet name="%s" sheetId="%d" r:id="r%d"/>`, escaped(sheet.Name), i+1, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="r%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
		var body strings.Builder
		body.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
		rows := [][]string{sheet.Headers}
		rows = append(rows, sheet.Rows...)
		for r, row := range rows {
			fmt.Fprintf(&body, `<row r="%d">`, r+1)
			for c, value := range row {
				axis, _ := table.Axis(c+1, r+1)
				if number, ok := sheet.NumericCells[axis]; ok {
					fmt.Fprintf(&body, `<c r="%s" t="n"><v>%s</v></c>`, axis, escaped(number))
				} else {
					fmt.Fprintf(&body, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, axis, escaped(value))
				}
			}
			body.WriteString(`</row>`)
		}
		body.WriteString(`</sheetData></worksheet>`)
		parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)] = body.String()
	}
	workbook.WriteString(`</sheets></workbook>`)
	rels.WriteString(`</Relationships>`)
	parts["xl/workbook.xml"] = workbook.String()
	parts["xl/_rels/workbook.xml.rels"] = rels.String()
	return parts
}

// WriteTo writes complete fixture members without closing the caller's file.
func WriteTo(file *os.File, parts map[string]string) error {
	writer := zip.NewWriter(file)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		part, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := part.Write([]byte(parts[name])); err != nil {
			return err
		}
	}
	return writer.Close()
}

// Write stores fixture data in a new file and closes it for validation.
func Write(t testing.TB, path string, parts map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteTo(file, parts); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
