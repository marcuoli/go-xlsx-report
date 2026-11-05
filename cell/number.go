package cell

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// ErrInvalidNumber identifies a value outside the supported Excel number contract.
var ErrInvalidNumber = errors.New("invalid Excel numeric literal")

var decimalLiteral = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// Number normalizes a finite decimal literal that survives Excel's 15-significant-
// digit precision and normal binary64 range. It does not infer a cell's meaning.
// False leaves the caller free to retain exact text; no rounding is accepted.
// Input length and exponent bounds keep decimal equivalence checks bounded.
func Number(text string) (string, bool) {
	if len(text) == 0 || len(text) > 512 || !decimalLiteral.MatchString(text) {
		return "", false
	}
	mantissa := text
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(text[i+1:])
		if err != nil || exponent < -400 || exponent > 400 {
			return "", false
		}
		mantissa = text[:i]
	}
	digits := strings.NewReplacer("+", "", "-", "", ".", "").Replace(mantissa)
	if len(strings.Trim(digits, "0")) > 15 {
		return "", false
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) || value != 0 && math.Abs(value) < 2.2250738585072014e-308 {
		return "", false
	}
	original, ok := new(big.Rat).SetString(text)
	if !ok {
		return "", false
	}
	if value == 0 {
		if original.Sign() != 0 {
			return "", false
		}
		return "0", true
	}
	canonical := strconv.FormatFloat(value, 'g', 15, 64)
	restored, ok := new(big.Rat).SetString(canonical)
	if !ok || original.Cmp(restored) != 0 {
		return "", false
	}
	return canonical, true
}

// EqualNumber compares supported numeric literals by normalized decimal value.
// It rejects invalid, over-precision and out-of-range inputs on either side.
func EqualNumber(first, second string) bool {
	a, ok := Number(first)
	if !ok {
		return false
	}
	b, ok := Number(second)
	return ok && a == b
}

// NumberWriter writes a numeric payload without formula inference. Implementations
// must retain its decimal value; styles and number formats belong to the adapter.
// It is optional so existing text-only backends remain compatible.
type NumberWriter interface {
	SetCellNumber(sheet, axis, value string) error
}

// SetNumber checks and normalizes a literal before dispatching it. Invalid values
// return ErrInvalidNumber with a bounded location, never the supplied payload.
func SetNumber(writer NumberWriter, sheet, axis, value string) error {
	canonical, ok := Number(value)
	if !ok {
		return fmt.Errorf("%w: %s!%s", ErrInvalidNumber, SafeIdentity(sheet, "generated_sheet"), SafeIdentity(axis, "generated_cell"))
	}
	return writer.SetCellNumber(sheet, axis, canonical)
}
