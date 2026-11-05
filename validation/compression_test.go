package validation_test

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/marcuoli/go-xlsx-report/internal/testbook"
	"github.com/marcuoli/go-xlsx-report/table"
	"github.com/marcuoli/go-xlsx-report/validation"
)

func TestCompressedExtentCannotEscapeArchive(t *testing.T) {
	sheets := []table.Sheet{{Name: "Summary", Headers: []string{"Value"}}}
	for _, oversized := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "report.xlsx")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		archive := zip.NewWriter(file)
		for name, text := range testbook.Parts(sheets) {
			var compressed bytes.Buffer
			deflater, err := flate.NewWriter(&compressed, flate.DefaultCompression)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := deflater.Write([]byte(text)); err != nil {
				t.Fatal(err)
			}
			if err := deflater.Close(); err != nil {
				t.Fatal(err)
			}
			header := &zip.FileHeader{Name: name, Method: zip.Deflate, CRC32: crc32.ChecksumIEEE([]byte(text)), CompressedSize64: uint64(compressed.Len()), UncompressedSize64: uint64(len(text))}
			if oversized && name == "xl/worksheets/sheet1.xml" {
				header.CompressedSize64 += 128 << 20
			}
			output, err := archive.CreateRaw(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := output.Write(compressed.Bytes()); err != nil {
				t.Fatal(err)
			}
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		err = validation.Validate(context.Background(), path, sheets)
		if !oversized && err != nil {
			t.Fatal(err)
		}
		if oversized && err == nil {
			t.Fatal("compressed size beyond archive accepted")
		}
	}
}
