// Package zipio streams ZIP output while preserving ordinary and ZIP64 headers.
package zipio

import (
	"archive/zip"
	"errors"
	"io"
	"os"
)

// NewWriter starts an archive at the destination's current offset. The caller
// owns the file, synchronization and publication; Close only finalizes the ZIP.
func NewWriter(destination *os.File) (*Writer, error) {
	if destination == nil {
		return nil, errors.New("ZIP output file is required")
	}
	offset, err := destination.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	return &Writer{Writer: zip.NewWriter(destination), destination: destination, startOffset: offset}, nil
}

// Writer retains only member headers and offsets, never workbook row payloads.
// Create and Close are sequential; use one writer for one output file.
// The embedded archive/zip methods retain interface compatibility. ZIP64 local
// header repair covers members opened through Create, as used by workbook backends.
type Writer struct {
	*zip.Writer
	destination *os.File
	startOffset int64
	entries     []member
}

type member struct {
	header *zip.FileHeader
	offset int64
}

func (entry member) needsZIP64(startOffset int64) bool {
	const maxZIP32 = 1<<32 - 1
	return entry.header.CompressedSize64 >= maxZIP32 ||
		entry.header.UncompressedSize64 >= maxZIP32 ||
		entry.offset-startOffset >= maxZIP32
}

// Create opens one deflated member and retains its finalized ZIP metadata.
func (writer *Writer) Create(name string) (io.Writer, error) {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	entry, err := writer.Writer.CreateHeader(header)
	if err != nil {
		return nil, err
	}
	// Remember the actual local-header position, before archive/zip may append
	// a ZIP64 extra field to the central header. No row or part data is retained.
	if err := writer.Writer.Flush(); err != nil {
		return nil, err
	}
	position, err := writer.destination.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	writer.entries = append(writer.entries, member{
		header: header,
		offset: position - int64(30+len(header.Name)+len(header.Extra)),
	})
	return entry, nil
}

// Close finalizes the archive and repairs only genuine ZIP64 local headers.
// Small members retain ZIP 2.0 headers for desktop spreadsheet compatibility.
func (writer *Writer) Close() error {
	if err := writer.Writer.Close(); err != nil {
		return err
	}
	for _, entry := range writer.entries {
		if !entry.needsZIP64(writer.startOffset) {
			continue
		}
		// Match Go's central-directory ZIP64 threshold, including the 32-bit
		// sentinel itself. Ordinary members retain their original ZIP 2.0 header.
		if _, err := writer.destination.WriteAt([]byte{45, 0}, entry.offset+4); err != nil {
			return err
		}
	}
	return nil
}
