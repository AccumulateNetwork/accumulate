package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestAddCredits_BurnsAcme(t *testing.T) {
	var timestamp uint64
	const issued = 1000.00
	const balance = 100.00
	const spend = 10.00
	const oracle = 500

	// The oracle is $0.05/ACME so spending 10 ACME buys 50 credits ($0.50)
	const expectedCredits = 50

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
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: alice.JoinPath("book", "1"),
				Amount:    *big.NewInt(spend * AcmePrecision),
				Oracle:    oracle,
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
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
	const spend = 10.00
	const oracle = 500

	// The oracle is $0.05/ACME and the minimum spend/fee is $0.01 so the
	// minimum debit is 0.2 ACME
	const minSpend = 0.2

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
		acctesting.NewTransaction().
			WithPrincipal(alice.JoinPath("tokens")).
			WithSigner(alice.JoinPath("book", "1"), 1).
			WithTimestampVar(&timestamp).
			WithBody(&AddCredits{
				Recipient: AccountUrl("foo"),
				Amount:    *big.NewInt(spend * AcmePrecision),
				Oracle:    oracle,
			}).
			Initiate(SignatureTypeED25519, aliceKey).
			Build(),
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
