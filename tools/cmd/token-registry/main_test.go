// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func tok(issued string, precision uint64) *Token {
	v, ok := new(big.Int).SetString(issued, 10)
	if !ok {
		panic(issued)
	}
	return &Token{issued: v, Precision: precision}
}

func TestDecimal(t *testing.T) {
	cases := []struct {
		issued    string
		precision uint64
		want      string
	}{
		{"34149218556237373", 8, "341492185.56237373"},
		{"100000000", 8, "1"},
		{"5", 8, "0.00000005"},
		{"0", 8, "0"},
		{"12345", 0, "12345"},
		{"1000000000000000000000000", 18, "1000000"},
	}
	for _, c := range cases {
		require.Equal(t, c.want, decimal(tok(c.issued, c.precision).issued, c.precision), c.issued)
	}
}

func TestCompareSupplyAcrossPrecisions(t *testing.T) {
	// 2 whole tokens at precision 0 beat 1.5 at precision 18, even though
	// the precision-18 base-unit figure is astronomically larger.
	require.Equal(t, 1, compareSupply(tok("2", 0), tok("1500000000000000000", 18)))
	// Equal whole-token supply at different precisions compares equal.
	require.Equal(t, 0, compareSupply(tok("100000000", 8), tok("1", 0)))
	// Beyond 2^53: must not lose precision.
	require.Equal(t, -1, compareSupply(tok("9007199254740993", 0), tok("9007199254740994", 0)))
}
