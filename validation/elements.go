package validation

import (
	"encoding/xml"
	"errors"
	"github.com/marcuoli/go-xlsx-report/cell"
	"strconv"
	"strings"
)

const mainNamespace = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"

func requireName(start xml.StartElement, local string) error {
	if start.Name.Local != local || start.Name.Space != mainNamespace {
		return errors.New("saved spreadsheet element identity differs")
	}
	return nil
}
func simpleText(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	text, err := leafText(decoder, start, "t", 7*32767)
	if err != nil {
		return "", err
	}
	return decodeSpreadsheetMLText(text)
}

// leafText rejects nested XML instead of DecodeElement's silent child skipping.
// This keeps formulas/foreign elements out of numeric payloads and text leaves.
func leafText(decoder *xml.Decoder, start xml.StartElement, local string, limit int) (string, error) {
	if err := requireName(start, local); err != nil {
		return "", err
	}
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch token := token.(type) {
		case xml.CharData:
			text.Write(token)
			if text.Len() > limit {
				return "", errors.New("saved text exceeds cell capacity")
			}
		case xml.EndElement:
			if token.Name != start.Name {
				return "", errors.New("malformed text element")
			}
			return text.String(), nil
		case xml.Comment:
		default:
			return "", errors.New("saved literal text contains nested content")
		}
	}
}

type stringItem struct{ text string }

func (item *stringItem) value() (string, error) { return item.text, nil }
func (item *stringItem) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if start.Name.Space != mainNamespace || (start.Name.Local != "si" && start.Name.Local != "is") {
		return errors.New("invalid saved string identity")
	}
	plain, runs := false, 0
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Space != mainNamespace {
				return errors.New("saved string namespace differs")
			}
			switch token.Name.Local {
			case "t":
				if plain || runs != 0 {
					return errors.New("saved string has duplicate/mixed text nodes")
				}
				plain = true
				value, err := simpleText(decoder, token)
				if err != nil {
					return err
				}
				text.WriteString(value)
			case "r":
				if plain {
					return errors.New("saved string mixes plain/rich text")
				}
				runs++
				value, err := readRun(decoder, token)
				if err != nil {
					return err
				}
				text.WriteString(value)
			case "rPh", "phoneticPr":
				if err := decoder.Skip(); err != nil {
					return err
				}
			default:
				return errors.New("saved string contains unsupported content")
			}
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return errors.New("saved string contains untagged text")
			}
		case xml.EndElement:
			if token.Name != start.Name || !plain && runs == 0 {
				return errors.New("saved string text is missing")
			}
			item.text = text.String()
			if err := cell.CheckText("saved_sheet", "saved_cell", item.text); err != nil {
				return err
			}
			return nil
		case xml.Comment:
		default:
			return errors.New("invalid saved string token")
		}
	}
}
func readRun(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	seen := false
	value := ""
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Space != mainNamespace {
				return "", errors.New("rich text namespace differs")
			}
			switch token.Name.Local {
			case "rPr":
				if err := decoder.Skip(); err != nil {
					return "", err
				}
			case "t":
				if seen {
					return "", errors.New("rich text has duplicate text nodes")
				}
				seen = true
				value, err = simpleText(decoder, token)
				if err != nil {
					return "", err
				}
			default:
				return "", errors.New("unsupported rich text element")
			}
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return "", errors.New("rich text has untagged content")
			}
		case xml.EndElement:
			if token.Name != start.Name || !seen {
				return "", errors.New("rich text value missing")
			}
			return value, nil
		case xml.Comment:
		default:
			return "", errors.New("invalid rich text token")
		}
	}
}

type savedCell struct {
	Axis, Kind, Value   string
	Inline              stringItem
	Formula             *string
	hasValue, hasInline bool
}

func (cell *savedCell) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if err := requireName(start, "c"); err != nil {
		return err
	}
	for _, attr := range start.Attr {
		if attr.Name.Space == "" {
			switch attr.Name.Local {
			case "r":
				cell.Axis = attr.Value
			case "t":
				cell.Kind = attr.Value
			}
		}
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Space != mainNamespace {
				return errors.New("saved cell namespace differs")
			}
			switch token.Name.Local {
			case "f":
				return errors.New("saved literal cell contains a formula")
			case "v":
				if cell.hasValue || cell.hasInline {
					return errors.New("saved cell repeats its value")
				}
				cell.hasValue = true
				value, err := leafText(decoder, token, "v", 512)
				if err != nil {
					return err
				}
				cell.Value = value
			case "is":
				if cell.hasValue || cell.hasInline {
					return errors.New("saved cell repeats its value")
				}
				cell.hasInline = true
				if err := decoder.DecodeElement(&cell.Inline, &token); err != nil {
					return err
				}
			default:
				return errors.New("saved cell has unsupported content")
			}
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return errors.New("saved cell has untagged content")
			}
		case xml.EndElement:
			if token.Name != start.Name {
				return errors.New("malformed cell")
			}
			return nil
		case xml.Comment:
		default:
			return errors.New("invalid cell token")
		}
	}
}

type savedRow struct {
	Index int
	Cells []savedCell
}

func (row *savedRow) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if err := requireName(start, "row"); err != nil {
		return err
	}
	for _, attr := range start.Attr {
		if attr.Name.Space == "" && attr.Name.Local == "r" {
			index, err := strconv.Atoi(attr.Value)
			if err != nil {
				return err
			}
			row.Index = index
		}
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if err := requireName(token, "c"); err != nil {
				return err
			}
			var cell savedCell
			if err := decoder.DecodeElement(&cell, &token); err != nil {
				return err
			}
			row.Cells = append(row.Cells, cell)
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return errors.New("row has untagged content")
			}
		case xml.EndElement:
			if token.Name != start.Name {
				return errors.New("malformed row")
			}
			return nil
		case xml.Comment:
		default:
			return errors.New("invalid row token")
		}
	}
}

type worksheetDocument struct{ Rows []savedRow }

func (sheet *worksheetDocument) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if err := requireName(start, "worksheet"); err != nil {
		return err
	}
	seen := false
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Space != mainNamespace {
				return errors.New("worksheet child namespace differs")
			}
			if token.Name.Local != "sheetData" {
				if err := decoder.Skip(); err != nil {
					return err
				}
				continue
			}
			if seen {
				return errors.New("worksheet repeats sheetData")
			}
			seen = true
			var data struct {
				Rows []savedRow `xml:"row"`
			}
			if err := decoder.DecodeElement(&data, &token); err != nil {
				return err
			}
			sheet.Rows = data.Rows
		case xml.CharData:
			if strings.TrimSpace(string(token)) != "" {
				return errors.New("worksheet contains untagged content")
			}
		case xml.EndElement:
			if token.Name != start.Name || !seen {
				return errors.New("worksheet data missing")
			}
			return nil
		case xml.Comment:
		default:
			return errors.New("invalid worksheet token")
		}
	}
}
