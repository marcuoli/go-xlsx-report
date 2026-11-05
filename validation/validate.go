package validation

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/marcuoli/go-xlsx-report/cell"
	"github.com/marcuoli/go-xlsx-report/table"
)

type textItem = stringItem

// Validate checks ZIP integrity, part identities, sheet order, exact row coverage
// and every saved literal value/type, including declared numeric cells. It rejects
// formulas and external relationships; undeclared cells retain exact text semantics.
// Fixed file/expanded/entry limits bound verification before atomic publication.
func Validate(ctx context.Context, path string, expected []table.Sheet) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, sheet := range expected {
		if err := table.Validate(sheet); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxWorkbookSize {
		return errors.New("XLSX is not a bounded regular file")
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer archive.Close()
	if len(archive.File) > 2048 {
		return errors.New("XLSX has too many parts")
	}
	if err := compressedBounds(path, info.Size(), archive.File); err != nil {
		return err
	}
	entries := map[string]*zip.File{}
	var expanded uint64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name
		if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || pathpkg.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || entries[name] != nil {
			return errors.New("invalid or duplicated XLSX part")
		}
		if entry.UncompressedSize64 > 128<<20 || expanded > 128<<20-entry.UncompressedSize64 {
			return errors.New("expanded XLSX exceeds capacity")
		}
		expanded += entry.UncompressedSize64
		stream, err := entry.Open()
		if err != nil {
			return err
		}
		readBytes, readErr := io.Copy(io.Discard, io.LimitReader(stream, 128<<20+1))
		closeErr := stream.Close()
		if readBytes != int64(entry.UncompressedSize64) || readBytes > 128<<20 {
			return errors.New("XLSX expanded size differs from its declared capacity")
		}
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		entries[name] = entry
		if strings.HasSuffix(strings.ToLower(name), ".xml") || strings.HasSuffix(strings.ToLower(name), ".rels") {
			if err := validateXMLPart(ctx, entry); err != nil {
				return err
			}
		}
		if strings.HasSuffix(name, ".rels") {
			var rels struct {
				Items []struct {
					TargetMode string `xml:"TargetMode,attr"`
				} `xml:"Relationship"`
			}
			if err := decodeArchiveEntry(entry, &rels); err != nil {
				return err
			}
			for _, rel := range rels.Items {
				if strings.EqualFold(rel.TargetMode, "External") {
					return errors.New("external XLSX relationships are prohibited")
				}
			}
		}
	}
	if len(expected) == 0 {
		return errors.New("expected worksheet set is empty")
	}
	var contentTypes struct {
		Overrides []struct {
			PartName    string `xml:"PartName,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if err := decodeRequiredArchiveEntry(entries, "[Content_Types].xml", &contentTypes); err != nil {
		return err
	}
	workbookType := false
	partTypes := map[string]string{}
	for _, part := range contentTypes.Overrides {
		if partTypes[part.PartName] != "" {
			return errors.New("XLSX content type is duplicated")
		}
		partTypes[part.PartName] = part.ContentType
		if part.PartName == "/xl/workbook.xml" && part.ContentType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml" {
			workbookType = true
		}
		if strings.Contains(strings.ToLower(part.ContentType), "macroenabled") || strings.Contains(strings.ToLower(part.ContentType), "vbaproject") {
			return errors.New("macro-enabled packages are unsupported")
		}
	}
	if !workbookType {
		return errors.New("workbook content type is missing")
	}
	var rootRelationships struct {
		Items []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := decodeRequiredArchiveEntry(entries, "_rels/.rels", &rootRelationships); err != nil {
		return err
	}
	rootFound := false
	rootCount := 0
	for _, relationship := range rootRelationships.Items {
		if relationship.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" {
			rootCount++
		}
		if relationship.Type == "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" && relationship.Target == "xl/workbook.xml" {
			rootFound = true
		}
	}
	if !rootFound || rootCount != 1 {
		return errors.New("workbook root relationship is missing")
	}
	parts, err := worksheetParts(entries)
	if err != nil {
		return err
	}
	if len(parts) != len(expected) {
		return errors.New("saved worksheet set differs")
	}
	var book struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := decodeRequiredArchiveEntry(entries, "xl/workbook.xml", &book); err != nil {
		return err
	}
	stringsTable := []string{}
	if shared := entries["xl/sharedStrings.xml"]; shared != nil {
		var values struct {
			Items []textItem `xml:"si"`
		}
		if err := decodeArchiveEntry(shared, &values); err != nil {
			return err
		}
		for _, item := range values.Items {
			value, err := item.value()
			if err != nil {
				return err
			}
			stringsTable = append(stringsTable, value)
		}
	}
	seen := map[string]bool{}
	for index, sheet := range expected {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := table.Validate(sheet); err != nil {
			return err
		}
		key := strings.ToLower(sheet.Name)
		if seen[key] || book.Sheets[index].Name != sheet.Name {
			return errors.New("saved worksheet order/identity differs")
		}
		seen[key] = true
		part, ok := parts[sheet.Name]
		if partTypes["/"+part] != "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml" {
			return errors.New("worksheet content type is missing or changed")
		}
		if !ok {
			return errors.New("saved worksheet is missing")
		}
		var worksheet worksheetDocument
		if err := decodeRequiredArchiveEntry(entries, part, &worksheet); err != nil {
			return err
		}
		rows := [][]string{sheet.Headers}
		rows = append(rows, sheet.Rows...)
		if len(worksheet.Rows) != len(rows) {
			return errors.New("saved worksheet row coverage differs")
		}
		for r, row := range worksheet.Rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			if row.Index != r+1 {
				return errors.New("saved worksheet rows are reordered")
			}
			actual := map[string]string{}
			for _, saved := range row.Cells {
				if saved.Formula != nil {
					return errors.New("saved literal cell contains a formula")
				}
				if _, duplicate := actual[saved.Axis]; duplicate {
					return errors.New("saved cell is duplicated")
				}
				if number, declared := sheet.NumericCells[saved.Axis]; declared {
					if saved.Kind != "" && saved.Kind != "n" || !saved.hasValue || saved.hasInline || !cell.EqualNumber(saved.Value, number) {
						return errors.New("saved numeric type or value differs")
					}
					actual[saved.Axis] = number
					continue
				}
				value := ""
				switch saved.Kind {
				case "s":
					if !saved.hasValue || saved.hasInline {
						return errors.New("shared string value child differs")
					}
					n, err := strconv.Atoi(saved.Value)
					if err != nil || n < 0 || n >= len(stringsTable) {
						return errors.New("invalid shared string reference")
					}
					value = stringsTable[n]
				case "inlineStr":
					if !saved.hasInline || saved.hasValue {
						return errors.New("inline string value child differs")
					}
					var err error
					value, err = saved.Inline.value()
					if err != nil {
						return err
					}
				case "":
					if saved.Value != "" || saved.hasValue || saved.hasInline {
						return errors.New("literal text changed to a numeric cell")
					}
				default:
					return errors.New("saved literal cell type differs")
				}
				actual[saved.Axis] = value
			}
			for c, want := range rows[r] {
				axis, _ := table.Axis(c+1, r+1)
				got, exists := actual[axis]
				if number, declared := sheet.NumericCells[axis]; declared {
					want = number
				}
				if got != want || !exists && want != "" {
					return fmt.Errorf("saved literal value differs at %s!%s", sheet.Name, axis)
				}
				delete(actual, axis)
			}
			if len(actual) != 0 {
				return errors.New("saved worksheet contains unexpected cells")
			}
		}
	}
	return nil
}
