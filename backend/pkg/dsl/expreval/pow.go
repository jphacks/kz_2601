package expreval

import (
	"math"
	"math/big"
	"sync"
)

// logPrec is the working precision (bits) of the pow correction.
const logPrec = 200

// pow is math.Pow rounded to the nearest float64. math.Pow is not correctly
// rounded and its last bit differs by GOARCH (amd64 uses assembly Exp/Log,
// arm64 fuses multiply-adds), while iOS (Foundation.pow) is correctly
// rounded. So the result is moved to the neighbour closest to e^(b·ln|a|),
// compared in logPrec-bit logs.
func pow(a, b float64) float64 {
	y := math.Pow(a, b)
	if y == 0 || math.IsInf(y, 0) || math.IsNaN(y) || math.Abs(y) < 0x1p-1022 ||
		a == 0 || math.IsInf(a, 0) || math.IsInf(b, 0) || b == 0 || b == 1 {
		return y
	}
	t := bigLog(new(big.Float).SetPrec(logPrec).SetFloat64(math.Abs(a)))
	t.Mul(t, new(big.Float).SetPrec(logPrec).SetFloat64(b))
	m := math.Abs(y)
	// below reports whether e^t lies below the midpoint of p and q.
	below := func(p, q float64) bool {
		mid := new(big.Float).SetPrec(logPrec).SetFloat64(p)
		mid.Add(mid, new(big.Float).SetPrec(logPrec).SetFloat64(q))
		mid.Quo(mid, big.NewFloat(2))
		return t.Cmp(bigLog(mid)) < 0
	}
	for range 4 {
		if up := math.Nextafter(m, math.Inf(1)); !math.IsInf(up, 0) && !below(m, up) {
			m = up
			continue
		}
		if down := math.Nextafter(m, 0); down >= 0x1p-1022 && below(down, m) {
			m = down
			continue
		}
		break
	}
	return math.Copysign(m, y)
}

var ln2 = sync.OnceValue(func() *big.Float {
	l := bigLogMant(new(big.Float).SetPrec(logPrec).SetFloat64(0.5))
	return l.Neg(l)
})

// bigLog is ln(x) for x > 0.
func bigLog(x *big.Float) *big.Float {
	m := new(big.Float).SetPrec(logPrec)
	e := x.MantExp(m) // x = m·2^e, m in [0.5, 1)
	r := bigLogMant(m)
	k := new(big.Float).SetPrec(logPrec).SetInt64(int64(e))
	return r.Add(r, k.Mul(k, ln2()))
}

// bigLogMant is ln(m) for m in [0.5, 1) by 2·atanh((m-1)/(m+1)).
func bigLogMant(m *big.Float) *big.Float {
	one := new(big.Float).SetPrec(logPrec).SetInt64(1)
	z := new(big.Float).SetPrec(logPrec).Sub(m, one)
	if z.Sign() == 0 {
		return z
	}
	z.Quo(z, new(big.Float).SetPrec(logPrec).Add(m, one))
	z2 := new(big.Float).SetPrec(logPrec).Mul(z, z)
	sum := new(big.Float).SetPrec(logPrec).Set(z)
	pw := new(big.Float).SetPrec(logPrec).Set(z)
	term := new(big.Float).SetPrec(logPrec)
	for n := int64(3); ; n += 2 {
		pw.Mul(pw, z2)
		term.Quo(pw, new(big.Float).SetPrec(logPrec).SetInt64(n))
		sum.Add(sum, term)
		if term.MantExp(nil) < sum.MantExp(nil)-logPrec-8 {
			break
		}
	}
	return sum.Mul(sum, big.NewFloat(2))
}
