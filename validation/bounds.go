package validation

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
)

// compressedBounds rejects member extents outside their local record or directory.
// ZIP64 member sizes remain supported by archive/zip; a directory requiring more
// than 32-bit offsets cannot occur within this validator's 32 MiB file bound.
func compressedBounds(path string, size int64, entries []*zip.File) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	tailSize := min(size, int64(65535+22))
	tail := make([]byte, int(tailSize))
	if _, err := file.ReadAt(tail, size-tailSize); err != nil {
		return err
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) == 0x06054b50 && i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) == len(tail) {
			end = i
			break
		}
	}
	if end < 0 {
		return errors.New("ZIP directory footer is missing")
	}
	directorySize := int64(binary.LittleEndian.Uint32(tail[end+12:]))
	relative := int64(binary.LittleEndian.Uint32(tail[end+16:]))
	if directorySize == 0xffffffff || relative == 0xffffffff {
		return errors.New("ZIP directory exceeds bounded workbook offsets")
	}
	directory := size - tailSize + int64(end) - directorySize
	base := directory - relative
	if directory < 0 || base < 0 {
		return errors.New("ZIP directory bounds differ")
	}
	type extent struct{ header, data, end int64 }
	ranges := make([]extent, 0, len(entries))
	position := directory
	for _, entry := range entries {
		var header [46]byte
		if _, err := file.ReadAt(header[:], position); err != nil {
			return err
		}
		if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 {
			return errors.New("ZIP central record is malformed")
		}
		offset := int64(binary.LittleEndian.Uint32(header[42:]))
		if offset == 0xffffffff {
			return errors.New("ZIP local offset exceeds file bound")
		}
		local := base + offset
		data, err := entry.DataOffset()
		if err != nil {
			return err
		}
		if local < base || data < local+30 || data > directory || entry.CompressedSize64 > uint64(directory-data) {
			return errors.New("ZIP compressed member exceeds archive extent")
		}
		ranges = append(ranges, extent{local, data, data + int64(entry.CompressedSize64)})
		position += 46 + int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if position > directory+directorySize {
			return errors.New("ZIP central record exceeds directory")
		}
	}
	if position != directory+directorySize {
		return errors.New("ZIP directory member coverage differs")
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].header < ranges[j].header })
	for i, member := range ranges {
		if i > 0 && member.header < ranges[i-1].end {
			return errors.New("ZIP member records overlap")
		}
		if member.end > directory {
			return errors.New("ZIP member overlaps directory")
		}
	}
	return nil
}

func validateXMLPart(ctx context.Context, entry *zip.File) error {
	stream, err := entry.Open()
	if err != nil {
		return err
	}
	defer stream.Close()
	decoder := xml.NewDecoder(stream)
	depth, roots := 0, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			if depth != 0 || roots != 1 {
				return errors.New("XML document root is missing")
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 {
					return errors.New("XML document repeats its root")
				}
				if err := validatePartRoot(entry.Name, token.Name); err != nil {
					return err
				}
			}
			if err := validateMetadataElement(entry.Name, token); err != nil {
				return err
			}
			attrs := map[xml.Name]bool{}
			for _, attr := range token.Attr {
				if attrs[attr.Name] {
					return errors.New("XML element repeats an attribute")
				}
				attrs[attr.Name] = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(token)) != "" {
				return errors.New("XML has content outside its root")
			}
		case xml.Directive:
			return errors.New("XML directives are unsupported")
		}
	}
}
func validatePartRoot(part string, name xml.Name) error {
	local, namespace := "", ""
	switch {
	case part == "[Content_Types].xml":
		local, namespace = "Types", "http://schemas.openxmlformats.org/package/2006/content-types"
	case strings.HasSuffix(part, ".rels"):
		local, namespace = "Relationships", "http://schemas.openxmlformats.org/package/2006/relationships"
	case part == "xl/workbook.xml":
		local, namespace = "workbook", mainNamespace
	case part == "xl/styles.xml":
		local, namespace = "styleSheet", mainNamespace
	case part == "xl/sharedStrings.xml":
		local, namespace = "sst", mainNamespace
	case strings.HasPrefix(part, "xl/worksheets/"):
		local, namespace = "worksheet", mainNamespace
	}
	if local != "" && (name.Local != local || name.Space != namespace) {
		return errors.New("XLSX XML part identity differs")
	}
	return nil
}

func validateMetadataElement(part string, start xml.StartElement) error {
	const relationships = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	const packageRelationships = "http://schemas.openxmlformats.org/package/2006/relationships"
	const contentTypes = "http://schemas.openxmlformats.org/package/2006/content-types"
	if part == "[Content_Types].xml" {
		if start.Name.Space != contentTypes {
			return errors.New("content type namespace differs")
		}
		for _, attr := range start.Attr {
			if (attr.Name.Local == "PartName" || attr.Name.Local == "ContentType" || attr.Name.Local == "Extension") && attr.Name.Space != "" {
				return errors.New("content type attribute namespace differs")
			}
		}
	}
	if strings.HasSuffix(part, ".rels") && start.Name.Local == "Relationship" {
		if start.Name.Space != packageRelationships {
			return errors.New("relationship namespace differs")
		}
		for _, attr := range start.Attr {
			if (attr.Name.Local == "Id" || attr.Name.Local == "Type" || attr.Name.Local == "Target" || attr.Name.Local == "TargetMode") && attr.Name.Space != "" {
				return errors.New("relationship attribute namespace differs")
			}
		}
	}
	if part == "xl/workbook.xml" && (start.Name.Local == "sheets" || start.Name.Local == "sheet") {
		if start.Name.Space != mainNamespace {
			return errors.New("workbook sheet namespace differs")
		}
		if start.Name.Local == "sheet" {
			ids := 0
			for _, attr := range start.Attr {
				if attr.Name.Local == "id" {
					if attr.Name.Space != relationships {
						return errors.New("sheet relationship attribute namespace differs")
					}
					ids++
				}
				if (attr.Name.Local == "name" || attr.Name.Local == "sheetId") && attr.Name.Space != "" {
					return errors.New("sheet metadata attribute namespace differs")
				}
			}
			if ids != 1 {
				return errors.New("sheet relationship identity missing")
			}
		}
	}
	return nil
}
