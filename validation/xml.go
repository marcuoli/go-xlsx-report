// Package validation checks complete saved XLSX packages and exact literal cells.
package validation

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	pathpkg "path"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maxWorkbookSize = 32 << 20
const workbookWorksheetRelationshipType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet"

func worksheetParts(entries map[string]*zip.File) (map[string]string, error) {
	var workbook struct {
		Sheets []struct {
			Name           string `xml:"name,attr"`
			RelationshipID string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := decodeRequiredArchiveEntry(entries, "xl/workbook.xml", &workbook); err != nil {
		return nil, err
	}

	type workbookRelationship struct {
		ID         string `xml:"Id,attr"`
		Type       string `xml:"Type,attr"`
		Target     string `xml:"Target,attr"`
		TargetMode string `xml:"TargetMode,attr"`
	}
	var relationships struct {
		Items []workbookRelationship `xml:"Relationship"`
	}
	if err := decodeRequiredArchiveEntry(entries, "xl/_rels/workbook.xml.rels", &relationships); err != nil {
		return nil, err
	}
	relationshipsByID := make(map[string]workbookRelationship, len(relationships.Items))
	for _, relationship := range relationships.Items {
		if relationship.ID == "" || relationship.Target == "" {
			return nil, errors.New("workbook relationship is invalid")
		}
		if _, duplicate := relationshipsByID[relationship.ID]; duplicate {
			return nil, errors.New("workbook relationship is duplicated")
		}
		relationshipsByID[relationship.ID] = relationship
	}

	parts := make(map[string]string, len(workbook.Sheets))
	seenSheets := make(map[string]struct{}, len(workbook.Sheets))
	seenParts := map[string]bool{}
	for _, sheet := range workbook.Sheets {
		if sheet.Name == "" || sheet.RelationshipID == "" {
			return nil, errors.New("workbook sheet relationship is invalid")
		}
		if _, duplicate := seenSheets[sheet.Name]; duplicate {
			return nil, errors.New("workbook sheet is duplicated")
		}
		seenSheets[sheet.Name] = struct{}{}
		relationship, found := relationshipsByID[sheet.RelationshipID]
		if !found {
			return nil, errors.New("workbook worksheet relationship is missing")
		}
		if relationship.Type != workbookWorksheetRelationshipType ||
			relationship.TargetMode != "" && !strings.EqualFold(relationship.TargetMode, "Internal") ||
			strings.Contains(relationship.Target, "\\") || strings.HasPrefix(relationship.Target, "/") {
			return nil, errors.New("workbook worksheet relationship is invalid")
		}
		target := pathpkg.Clean(pathpkg.Join("xl", relationship.Target))
		if !strings.HasPrefix(target, "xl/worksheets/") {
			return nil, errors.New("workbook worksheet relationship is invalid")
		}
		if seenParts[target] {
			return nil, errors.New("workbook worksheet part is duplicated")
		}
		seenParts[target] = true
		parts[sheet.Name] = target
	}
	return parts, nil
}

func decodeRequiredArchiveEntry(entries map[string]*zip.File, name string, value any) error {
	entry, found := entries[name]
	if !found {
		return fmt.Errorf("workbook package entry %q is missing", name)
	}
	return decodeArchiveEntry(entry, value)
}

// XML parts can grow with the generated result set. Decode directly from the
// archive, consume the complete stream to check the ZIP CRC, and reject any
// malformed trailing XML or second root rather than accepting a valid prefix.
func decodeArchiveEntry(file *zip.File, value any) (err error) {
	stream, err := file.Open()
	if err != nil {
		return errors.New("workbook package entry could not be read")
	}
	defer func() { err = errors.Join(err, stream.Close()) }()
	decoder := xml.NewDecoder(stream)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return errors.New("workbook package XML has trailing content")
			}
		case xml.Comment, xml.ProcInst:
		default:
			return errors.New("workbook package XML has trailing content")
		}
	}
}

func readArchiveEntry(file *zip.File) ([]byte, error) {
	stream, err := file.Open()
	if err != nil {
		return nil, errors.New("workbook package entry could not be read")
	}
	data, readErr := io.ReadAll(io.LimitReader(stream, maxWorkbookSize+1))
	closeErr := stream.Close()
	if readErr != nil || closeErr != nil || len(data) > maxWorkbookSize {
		return nil, errors.New("workbook package entry could not be read")
	}
	return data, nil
}
func decodeSpreadsheetMLText(value string) (string, error) {
	if !strings.Contains(value, "_x") {
		return value, nil
	}
	var result strings.Builder
	for index := 0; index < len(value); {
		unit, encoded := spreadsheetMLTextUnit(value[index:])
		if !encoded {
			result.WriteByte(value[index])
			index++
			continue
		}
		index += 7
		character := rune(unit)
		if utf16.IsSurrogate(character) {
			low, found := spreadsheetMLTextUnit(value[index:])
			if unit < 0xD800 || unit > 0xDBFF || !found || low < 0xDC00 || low > 0xDFFF {
				return "", errors.New("generated table contains an invalid SpreadsheetML surrogate")
			}
			character = utf16.DecodeRune(character, rune(low))
			index += 7
		}
		result.WriteRune(character)
	}
	return result.String(), nil
}

func spreadsheetMLTextUnit(value string) (uint16, bool) {
	if len(value) < 7 || value[0] != '_' || value[1] != 'x' || value[6] != '_' {
		return 0, false
	}
	unit, err := strconv.ParseUint(value[2:6], 16, 16)
	return uint16(unit), err == nil
}
