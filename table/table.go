// Package table renders validated literal tables through an injected workbook backend.
package table

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/marcuoli/go-xlsx-report/cell"
	"github.com/marcuoli/go-xlsx-report/layout"
)

// Backend supplies workbook operations. The adapter owns native style objects.
// SetCellStr must write literal strings, including values beginning with '='.
type Backend interface {
	cell.TextWriter
	NewSheet(string) (int, error)
	SetCellStyle(sheet, first, last string, style int) error
	SetRowHeight(sheet string, row int, height float64) error
	SetColWidth(sheet, first, last string, width float64) error
	Freeze(sheet string, rows, columns int) error
	Filter(sheet, reference string) error
}

// Sheet is a complete table snapshot. Rows and headers remain caller-owned and
// must not change while rendering or validating the saved package.
type Sheet struct {
	Name                                  string
	Headers                               []string
	Rows                                  [][]string
	Widths                                []float64
	RowStyles                             []int
	HeaderStyle, BodyStyle, FreezeColumns int
}

// Axis converts a one-based column/row to a bounded spreadsheet cell reference.
func Axis(column, row int) (string, error) {
	if column < 1 || column > 16384 || row < 1 || row > 1048576 {
		return "", errors.New("cell coordinates exceed worksheet capacity")
	}
	var letters [3]byte
	i := len(letters)
	for column > 0 {
		column--
		i--
		letters[i] = byte('A' + column%26)
		column /= 26
	}
	return fmt.Sprintf("%s%d", letters[i:], row), nil
}

// Validate checks all dimensions and literal capacities before backend mutation.
func Validate(sheet Sheet) error {
	if strings.TrimSpace(sheet.Name) == "" || sheet.Name != strings.TrimSpace(sheet.Name) || len([]rune(sheet.Name)) > 31 || strings.ContainsAny(sheet.Name, `[]:*?/\`) || strings.HasPrefix(sheet.Name, "'") || strings.HasSuffix(sheet.Name, "'") {
		return errors.New("invalid worksheet name")
	}
	for _, r := range sheet.Name {
		if unicode.IsControl(r) {
			return errors.New("worksheet name contains controls")
		}
	}
	if len(sheet.Headers) == 0 || len(sheet.Headers) > 16384 || len(sheet.Rows) > 1048575 || sheet.FreezeColumns < 0 || sheet.FreezeColumns >= len(sheet.Headers) {
		return errors.New("invalid worksheet dimensions")
	}
	if len(sheet.Widths) != 0 && len(sheet.Widths) != len(sheet.Headers) || len(sheet.RowStyles) != 0 && len(sheet.RowStyles) != len(sheet.Rows) {
		return errors.New("worksheet layout dimensions differ")
	}
	for _, width := range sheet.Widths {
		if math.IsNaN(width) || math.IsInf(width, 0) || width <= 0 || width > 255 {
			return errors.New("invalid column width")
		}
	}
	rows := [][]string{sheet.Headers}
	rows = append(rows, sheet.Rows...)
	for r, row := range rows {
		if len(row) != len(sheet.Headers) {
			return errors.New("row width differs from header")
		}
		for c, value := range row {
			axis, _ := Axis(c+1, r+1)
			if err := cell.CheckText(sheet.Name, axis, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// backendOperation observes cancellation before and after every backend mutation.
// The post-check prevents a cancellation during the final write from being hidden.
func backendOperation(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.Join(operation(), ctx.Err())
}

// Render writes literal values and bounded dimensions, preserving exact strings.
// A failure aborts the caller's staged publication; no partial table is accepted.
func Render(ctx context.Context, backend Backend, sheet Sheet) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := Validate(sheet); err != nil {
		return err
	}
	if err := backendOperation(ctx, func() error { _, err := backend.NewSheet(sheet.Name); return err }); err != nil {
		return err
	}
	rows := [][]string{sheet.Headers}
	rows = append(rows, sheet.Rows...)
	widths := make([]float64, len(sheet.Headers))
	for c, header := range sheet.Headers {
		widths[c] = min(64, max(12, float64(layout.LongestLine(header)+2)))
	}
	for r, row := range rows {
		for c, value := range row {
			axis, _ := Axis(c+1, r+1)
			if err := backendOperation(ctx, func() error { return cell.SetText(backend, sheet.Name, axis, value) }); err != nil {
				return err
			}
			widths[c] = min(64, max(widths[c], float64(layout.LongestLine(value)+2)))
		}
	}
	if len(sheet.Widths) != 0 {
		copy(widths, sheet.Widths)
	}
	for c, width := range widths {
		axis, _ := Axis(c+1, 1)
		column := strings.TrimSuffix(axis, "1")
		if err := backendOperation(ctx, func() error { return backend.SetColWidth(sheet.Name, column, column, width) }); err != nil {
			return err
		}
	}
	for r, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		style := sheet.BodyStyle
		if r == 0 {
			style = sheet.HeaderStyle
		} else if len(sheet.RowStyles) != 0 {
			style = sheet.RowStyles[r-1]
		}
		first, _ := Axis(1, r+1)
		last, _ := Axis(len(row), r+1)
		if err := backendOperation(ctx, func() error { return backend.SetCellStyle(sheet.Name, first, last, style) }); err != nil {
			return err
		}
		lines := 1
		for c, value := range row {
			lines = max(lines, layout.LineCount(value, max(1, int(widths[c])-2)))
		}
		height := min(409, float64(lines)*15+6)
		if r == 0 {
			height = max(36, height)
		}
		if err := backendOperation(ctx, func() error { return backend.SetRowHeight(sheet.Name, r+1, height) }); err != nil {
			return err
		}
	}
	last, _ := Axis(len(sheet.Headers), len(sheet.Rows)+1)
	if err := backendOperation(ctx, func() error { return backend.Filter(sheet.Name, "A1:"+last) }); err != nil {
		return err
	}
	return backendOperation(ctx, func() error { return backend.Freeze(sheet.Name, 1, sheet.FreezeColumns) })
}
