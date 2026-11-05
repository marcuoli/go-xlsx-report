package publication_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/marcuoli/go-xlsx-report/internal/testbook"
	"github.com/marcuoli/go-xlsx-report/publication"
	"github.com/marcuoli/go-xlsx-report/table"
)

func TestFailedValidationPreservesInstalledWorkbook(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "report.xlsx")
	sheets := []table.Sheet{{Name: "Example", Headers: []string{"Value"}, Rows: [][]string{{"9007199254740993"}}}}
	parts := testbook.Parts(sheets)
	save := func(file *os.File) error { return testbook.WriteTo(file, parts) }
	installed, err := publication.Publish(context.Background(), path, false, save, sheets)
	if !installed || err != nil {
		t.Fatalf("initial publication: %v %v", installed, err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], "9007199254740993", "9007199254740992", 1)
	installed, err = publication.Publish(context.Background(), path, true, save, sheets)
	if installed || err == nil {
		t.Fatal("invalid staged values published")
	}
	retained, _ := os.ReadFile(path)
	if string(retained) != string(original) {
		t.Fatal("previous workbook lost")
	}
	leftovers, _ := filepath.Glob(filepath.Join(directory, ".xlsx-*"))
	if len(leftovers) != 0 {
		t.Fatal("staging files leaked")
	}
}

func TestFailedOrCancelledSaveNeverInstalls(t *testing.T) {
	sheets := []table.Sheet{{Name: "Example", Headers: []string{"Value"}}}
	for _, interrupted := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "report.xlsx")
		ctx, cancel := context.WithCancel(context.Background())
		failure := errors.New("write failed")
		save := func(file *os.File) error {
			if interrupted {
				if err := testbook.WriteTo(file, testbook.Parts(sheets)); err != nil {
					return err
				}
				cancel()
				return nil
			}
			return failure
		}
		installed, err := publication.Publish(ctx, path, false, save, sheets)
		cancel()
		if installed || err == nil {
			t.Fatal("failed/cancelled save installed")
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unexpected destination")
		}
	}
}

func TestNestedNumericContentCannotReplaceInstalledWorkbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "numbers.xlsx")
	sheets := []table.Sheet{{Name: "Values", Headers: []string{"Number"}, Rows: [][]string{{"1.25"}}, NumericCells: map[string]string{"A2": "1.25"}}}
	parts := testbook.Parts(sheets)
	save := func(file *os.File) error { return testbook.WriteTo(file, parts) }
	if installed, err := publication.Publish(context.Background(), path, false, save, sheets); !installed || err != nil {
		t.Fatal(installed, err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parts["xl/worksheets/sheet1.xml"] = strings.Replace(parts["xl/worksheets/sheet1.xml"], `<v>1.25</v>`, `<v>1.25<f>1+1</f></v>`, 1)
	if installed, err := publication.Publish(context.Background(), path, true, save, sheets); installed || err == nil {
		t.Fatal("nested numeric content installed")
	}
	retained, err := os.ReadFile(path)
	if err != nil || string(retained) != string(original) {
		t.Fatal("failed numeric validation changed installed workbook")
	}
}
