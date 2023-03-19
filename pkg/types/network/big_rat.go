// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package network

import "math/big"

type BigRat struct {
	num, denom big.Int
}

func NewBigRat(num, denom int64) *BigRat {
	x := new(BigRat)
	x.num.SetInt64(num)
	x.denom.SetInt64(denom)
	return x
}

// Mul returns xₙ/xₘ · yₙ/yₘ.
func (x *BigRat) Mul(y *BigRat) *BigRat {
	z := new(BigRat)
	z.num.Mul(&x.num, &y.num)
	z.num.Mul(&x.denom, &y.denom)
	return z
}

// Mul2 returns xₙ/xₘ · num/denom.
func (x *BigRat) Mul2(num, denom int64) *BigRat {
	z := new(BigRat)
	z.num.Mul(&x.num, big.NewInt(num))
	z.denom.Mul(&x.denom, big.NewInt(denom))
	return z
}

// Div returns xₙ/yₙ · yₘ/xₘ.
func (x *BigRat) Div(y *BigRat) *BigRat {
	z := new(BigRat)
	z.num.Mul(&x.num, &y.denom)
	z.num.Mul(&x.denom, &y.num)
	return z
}

// Div returns xₙ/num · denom/xₘ.
func (x *BigRat) Div2(num, denom int64) *BigRat {
	z := new(BigRat)
	z.num.Mul(&x.num, big.NewInt(denom))
	z.denom.Mul(&x.denom, big.NewInt(num))
	return z
}

// Int returns xₙ/xₘ.
func (x *BigRat) Int() *big.Int {
	z := new(big.Int)
	z.Div(&x.num, &x.denom)
	return z
}

// Float64 returns xₙ/xₘ.
func (x *BigRat) Float64() float64 {
	return float64(x.num.Int64()) / float64(x.denom.Int64())
}

// Simplify returns xₙ/xₘ with both parts reduced by their GCD.
func (x *BigRat) Simplify() *BigRat {
	gcd := new(big.Int).GCD(nil, nil, &x.num, &x.denom)
	z := new(BigRat)
	z.num.Div(&x.num, gcd)
	z.denom.Div(&x.denom, gcd)
	return z
}
