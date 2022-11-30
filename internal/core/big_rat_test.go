// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestBigRat(t *testing.T) {
	// I want 1000 credits. 1 credit is worth $0.01. If the oracle is
	// $0.25/ACME, then 1000 credits = $10 = 40 ACME.
	const creditsWanted = 1000.00
	const acmeExpected = 40.00000000

	// ACME = credits ÷ oracle ÷ credits-per-dollar
	rat := NewBigRat(1, 1)
	rat = rat.Mul2(creditsWanted*protocol.CreditPrecision, protocol.CreditPrecision)
	rat = rat.Div2(0.25*protocol.AcmeOraclePrecision, protocol.AcmeOraclePrecision)
	rat = rat.Div2(protocol.CreditsPerDollar, 1)
	rat = rat.Simplify()

	require.Equal(t, acmeExpected, rat.Float64())
}
