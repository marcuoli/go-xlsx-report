// Package cell owns literal text capacity checks independent of a workbook backend.
package cell

import (
	"fmt"
	"strings"
	"unicode"
)

// MaximumTextUnits is the spreadsheet cell capacity in UTF-16 code units.
const MaximumTextUnits = 32767

// CapacityError records location and capacity facts without retaining cell values.
type CapacityError struct {
	Sheet, Cell                  string
	MeasuredUnits, CapacityUnits int
}

// Error reports the location and measured capacity without retaining values.
func (e *CapacityError) Error() string {
	return fmt.Sprintf("text capacity exceeded: sheet=%s cell=%s measured=%d capacity=%d unit=UTF-16", e.Sheet, e.Cell, e.MeasuredUnits, e.CapacityUnits)
}

// TextUnits counts UTF-16 code units rather than bytes or Unicode code points.
func TextUnits(value string) int {
	units := 0
	for _, r := range value {
		units++
		if r > 0xffff {
			units++
		}
	}
	return units
}

// SafeIdentity bounds public error locations without echoing collected values.
func SafeIdentity(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.Contains(value, "://") || strings.ContainsAny(value, "@\\=\"\x1b") {
		return fallback
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fallback
		}
	}
	return value
}

// CheckText preserves complete values and returns a typed error on overflow.
func CheckText(sheet, axis, value string) error {
	units := TextUnits(value)
	if units > MaximumTextUnits {
		return &CapacityError{SafeIdentity(sheet, "generated_sheet"), SafeIdentity(axis, "generated_cell"), units, MaximumTextUnits}
	}
	return nil
}

// TextWriter is the backend's literal-string operation, never formula inference.
type TextWriter interface {
	SetCellStr(sheet, axis, value string) error
}

// SetText validates capacity before dispatching the complete literal value.
func SetText(writer TextWriter, sheet, axis, value string) error {
	if err := CheckText(sheet, axis, value); err != nil {
		return err
	}
	return writer.SetCellStr(sheet, axis, value)
}
