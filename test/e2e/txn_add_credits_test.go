// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestAddCredits_BurnsAcme(t *testing.T) {
	var timestamp uint64
	const issued = 1000.00
	const balance = 100.00
	const oracle = InitialAcmeOracleValue
	const expectedCredits = 50
	const spend = expectedCredits / oracle * CreditPrecision

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(balance * AcmePrecision)})
	updateAccount(sim, AcmeUrl(), func(acme *TokenIssuer) { acme.Issued.SetUint64(1e3 * AcmePrecision) })

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("tokens")).
			Body(&AddCredits{
				Recipient: alice.JoinPath("book", "1"),
				Amount:    *big.NewInt(spend * AcmePrecision),
				Oracle:    oracle,
			}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)),
	)...)

	// Wait a few blocks for everything to settle
	sim.ExecuteBlocks(10)

	// Verify 0.5 credits were credited
	page := simulator.GetAccount[*KeyPage](sim, alice.JoinPath("book", "1"))
	assert.Equal(t,
		FormatAmount(expectedCredits*CreditPrecision, CreditPrecisionPower),
		FormatAmount(page.CreditBalance, CreditPrecisionPower))

	// Verify 10 ACME was debited
	account := simulator.GetAccount[*TokenAccount](sim, alice.JoinPath("tokens"))
	assert.Equal(t,
		FormatAmount((balance-spend)*AcmePrecision, AcmePrecisionPower),
		FormatBigAmount(&account.Balance, AcmePrecisionPower))

	// Verify 10 ACME was burnt
	acme := simulator.GetAccount[*TokenIssuer](sim, AcmeUrl())
	assert.Equal(t,
		FormatAmount((issued-spend)*AcmePrecision, AcmePrecisionPower),
		FormatBigAmount(&acme.Issued, AcmePrecisionPower))
}

func TestAddCredits_RefundsAcme(t *testing.T) {
	var timestamp uint64
	const issued = 1000.00
	const balance = 100.00
	const oracle = InitialAcmeOracleValue
	const expectedCredits = 50
	const spend = expectedCredits / oracle * CreditPrecision

	// The oracle is $5000/ACME and the minimum spend/fee is $0.01 so the
	// minimum debit is 0.000002 ACME
	const minSpend = 100 / oracle

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	sim.CreateAccount(&TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(balance * AcmePrecision)})
	updateAccount(sim, AcmeUrl(), func(acme *TokenIssuer) { acme.Issued.SetUint64(1e3 * AcmePrecision) })

	// Execute
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("tokens")).
			Body(&AddCredits{
				Recipient: AccountUrl("foo"),
				Amount:    *big.NewInt(spend * AcmePrecision),
				Oracle:    oracle,
			}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)),
	)...)

	// Wait a few blocks for everything to settle
	sim.ExecuteBlocks(10)

	// Verify $0.01 worth of ACME was debited, after the refund
	account := simulator.GetAccount[*TokenAccount](sim, alice.JoinPath("tokens"))
	assert.Equal(t,
		FormatAmount((balance-minSpend)*AcmePrecision, AcmePrecisionPower),
		FormatBigAmount(&account.Balance, AcmePrecisionPower))

	// Verify $0.01 worth of ACME was burnt
	acme := simulator.GetAccount[*TokenIssuer](sim, AcmeUrl())
	assert.Equal(t,
		FormatAmount((issued-minSpend)*AcmePrecision, AcmePrecisionPower),
		FormatBigAmount(&acme.Issued, AcmePrecisionPower))
}
