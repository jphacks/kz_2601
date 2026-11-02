package expreval

import (
	"math"
	"testing"
)

// Correctly rounded values (checked with 50-digit decimals).
func TestPowCorrectlyRounded(t *testing.T) {
	for _, c := range []struct{ a, b, want float64 }{
		{1.5, 2.5, 2.7556759606310752},
		{2, 0.5, math.Sqrt2},
		{2, 10, 1024},
		{-2, 3, -8},
		{10, -2, 0.01},
		{0.5, 1074, 0x1p-1074},
	} {
		if got := pow(c.a, c.b); got != c.want {
			t.Errorf("pow(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
