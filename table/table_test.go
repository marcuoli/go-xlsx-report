package table_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marcuoli/go-xlsx-report/cell"
	"github.com/marcuoli/go-xlsx-report/layout"
	"github.com/marcuoli/go-xlsx-report/table"
)

type backend struct {
	Created int
	Values  map[string]string
	Failure error
	Cancel  context.CancelFunc
}

func (b *backend) NewSheet(string) (int, error) { b.Created++; return 0, nil }
func (b *backend) SetCellStr(_ string, axis, value string) error {
	if b.Failure != nil {
		return b.Failure
	}
	if b.Values == nil {
		b.Values = map[string]string{}
	}
	b.Values[axis] = value
	if b.Cancel != nil {
		b.Cancel()
	}
	return nil
}
func (*backend) SetCellStyle(string, string, string, int) error    { return nil }
func (*backend) SetRowHeight(string, int, float64) error           { return nil }
func (*backend) SetColWidth(string, string, string, float64) error { return nil }
func (*backend) Freeze(string, int, int) error                     { return nil }
func (*backend) Filter(string, string) error                       { return nil }

func TestRenderingKeepsLiteralAndExactValues(t *testing.T) {
	b := &backend{}
	sheet := table.Sheet{Name: "Example", Headers: []string{"Value"}, Rows: [][]string{{"=1+1"}, {"9007199254740993"}, {"ação 東京 😀"}}}
	if err := table.Render(context.Background(), b, sheet); err != nil {
		t.Fatal(err)
	}
	if b.Values["A2"] != "=1+1" || b.Values["A3"] != "9007199254740993" || b.Values["A4"] != "ação 東京 😀" {
		t.Fatal("literal value changed")
	}
	if layout.LongestLine("e\u0301") != 1 || layout.LongestLine("東京") != 4 {
		t.Fatal("Unicode glyph width changed")
	}
}

func TestCapacityFailsBeforeBackendMutation(t *testing.T) {
	b := &backend{}
	value := strings.Repeat("😀", 16384)
	err := table.Render(context.Background(), b, table.Sheet{Name: "Example", Headers: []string{"Value"}, Rows: [][]string{{value}}})
	var capacity *cell.CapacityError
	if !errors.As(err, &capacity) || capacity.MeasuredUnits != 32768 || b.Created != 0 || strings.Contains(err.Error(), value) {
		t.Fatal("overflow lost identity or mutated output")
	}
	if err := cell.CheckText("Example", "A1", strings.Repeat("😀", 16383)+"x"); err != nil {
		t.Fatal(err)
	}
}

func TestWriterErrorsAndCancellationAbort(t *testing.T) {
	failure := errors.New("destination failed")
	if err := table.Render(context.Background(), &backend{Failure: failure}, table.Sheet{Name: "Example", Headers: []string{"Value"}}); !errors.Is(err, failure) {
		t.Fatal("writer failure hidden")
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &backend{Cancel: cancel}
	err := table.Render(ctx, b, table.Sheet{Name: "Example", Headers: []string{"Value"}, Rows: [][]string{{"not written"}}})
	if !errors.Is(err, context.Canceled) || len(b.Values) != 1 {
		t.Fatal("cancelled renderer continued")
	}
}

func TestCancellationDuringOnlyCellDoesNotReportSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := &backend{Cancel: cancel}
	err := table.Render(ctx, b, table.Sheet{Name: "Example", Headers: []string{"Value"}})
	if !errors.Is(err, context.Canceled) || len(b.Values) != 1 {
		t.Fatalf("late cancellation: %v writes=%d", err, len(b.Values))
	}
}
