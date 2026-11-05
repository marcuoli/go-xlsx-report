package cell_test

import (
	"fmt"
	"github.com/marcuoli/go-xlsx-report/cell"
	"testing"
)

func TestNumberPrecisionAndRange(t *testing.T) {
	for _, test := range []struct{ name, text, want string }{
		{"zero", "0", "0"}, {"negative zero", "-0.000", "0"}, {"decimal", ".747", "0.747"}, {"exponent", "1.25E+2", "125"},
		{"trailing zeros", "1000000000000000", "1e+15"}, {"fifteen digits", "123456789012345", "123456789012345"},
		{"sixteen digits", "9007199254740993", ""}, {"exact decimal overflow", "1234567890123456.789", ""}, {"extra precision", "1.000000000000001", ""},
		{"nan", "NaN", ""}, {"infinity", "Inf", ""}, {"formula", "=1+1", ""}, {"unit text", "5 rows", ""}, {"fraction", "1/3", ""},
		{"underflow", "1e-400", ""}, {"subnormal", "1e-308", ""}, {"normal small", "1e-307", "1e-307"}, {"overflow", "1e309", ""},
		{"unbounded exponent", "0e2147483647", ""}, {"whitespace", " 1", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := cell.Number(test.text)
			if got != test.want || ok != (test.want != "") {
				t.Fatalf("got %q/%v; want %q", got, ok, test.want)
			}
		})
	}
	if !cell.EqualNumber("125.00", "1.25e2") || cell.EqualNumber("1", "2") || cell.EqualNumber("NaN", "NaN") {
		t.Fatal("numeric equivalence changed")
	}
}

func ExampleNumber() {
	value, ok := cell.Number(".747")
	fmt.Println(value, ok)
	_, ok = cell.Number("9007199254740993")
	fmt.Println(ok)
	// Output:
	// 0.747 true
	// false
}
