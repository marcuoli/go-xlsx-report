package zipio

import (
	"archive/zip"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkbookZIP64MetadataBoundaries(t *testing.T) {
	const limit = uint64(1<<32 - 1)
	for _, test := range []struct {
		name         string
		compressed   uint64
		uncompressed uint64
		version      uint16
	}{
		{"below compressed boundary", limit - 1, 7, 20},
		{"at compressed boundary", limit, 7, 45},
		{"above compressed boundary", limit + 1, 7, 45},
		{"below expanded boundary", 7, limit - 1, 20},
		{"at expanded boundary", 7, limit, 45},
		{"above expanded boundary", 7, limit + 1, 45},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "metadata.zip")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			writer := &Writer{Writer: zip.NewWriter(file), destination: file}
			candidate, err := writer.Create("candidate.xml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(candidate, "payload"); err != nil {
				t.Fatal(err)
			}
			ordinary, err := writer.Create("ordinary.xml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(ordinary, "retained"); err != nil {
				t.Fatal(err)
			}
			// The first member is already finalized. Supply synthetic counts to
			// exercise Go's real central-directory thresholds without writing
			// gigabytes. This is metadata coverage, not huge-payload/native proof.
			writer.entries[0].header.CompressedSize64 = test.compressed
			writer.entries[0].header.UncompressedSize64 = test.uncompressed
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			stat, err := file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(file, stat.Size())
			if err != nil {
				t.Fatal(err)
			}
			if len(archive.File) != 2 {
				t.Fatalf("members=%d", len(archive.File))
			}
			for index, want := range []uint16{test.version, 20} {
				if got := archive.File[index].ReaderVersion; got != want {
					t.Errorf("member %d central version=%d, want %d", index, got, want)
				}
				var raw [2]byte
				if _, err := file.ReadAt(raw[:], writer.entries[index].offset+4); err != nil {
					t.Fatal(err)
				}
				if got := binary.LittleEndian.Uint16(raw[:]); got != want {
					t.Errorf("member %d local version=%d, want %d", index, got, want)
				}
			}
			// A neighbouring ordinary member must retain both ZIP 2.0 and its
			// independently readable payload/CRC when the archive uses ZIP64.
			reader, err := archive.File[1].Open()
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(reader)
			closeErr := reader.Close()
			if err != nil || closeErr != nil || string(body) != "retained" {
				t.Fatalf("ordinary payload=%q read=%v close=%v", body, err, closeErr)
			}
		})
	}
}

func TestWorkbookZIP64OffsetBoundary(t *testing.T) {
	const limit = int64(1<<32 - 1)
	for _, test := range []struct {
		name   string
		start  int64
		offset int64
		want   bool
	}{
		{"below boundary", 0, limit - 1, false},
		{"at boundary", 0, limit, true},
		{"above boundary", 0, limit + 1, true},
		{"prefix is not a ZIP-relative offset", limit, limit + 10, false},
		{"prefixed boundary", 100, limit + 100, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := member{header: &zip.FileHeader{}, offset: test.offset}
			if got := entry.needsZIP64(test.start); got != test.want {
				t.Fatalf("ZIP64=%v, want %v", got, test.want)
			}
		})
	}
}

func TestWorkbookZIP64BackpatchFailureIsReturned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read-only-header.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := &Writer{Writer: zip.NewWriter(file), destination: file}
	for _, name := range []string{"candidate.xml", "ordinary.xml"} {
		output, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(output, "retained"); err != nil {
			t.Fatal(err)
		}
	}
	writer.entries[0].header.UncompressedSize64 = 1<<32 - 1
	readOnly, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	// The underlying ZIP writer can finish on its original writable handle;
	// the local-header update alone must fail on this read-only handle.
	writer.destination = readOnly
	if err := writer.Close(); err == nil {
		t.Fatal("ZIP64 local-header write failure was hidden")
	}
}
